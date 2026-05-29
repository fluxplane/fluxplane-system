package hostsystem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

// Process executes direct host processes without a shell interpreter.
type Process struct {
	root   string
	env    system.Environment
	clock  system.Clock
	mu     sync.Mutex
	nextID atomic.Uint64
	procs  map[string]*managedProcess
}

func NewProcess(root string, env system.Environment, clock system.Clock) *Process {
	if clock == nil {
		clock = RealClock{}
	}
	if env == nil {
		env = Environment{}
	}
	return &Process{root: root, env: env, clock: clock, procs: map[string]*managedProcess{}}
}

func (p *Process) Run(ctx context.Context, req system.ProcessRequest) (system.ProcessResult, error) {
	handle, err := p.Start(ctx, req)
	if err != nil {
		return system.ProcessResult{}, err
	}
	return handle.Wait(ctx)
}

func (p *Process) Start(ctx context.Context, req system.ProcessRequest) (system.ProcessHandle, error) {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return nil, fmt.Errorf("command is empty")
	}
	if strings.ContainsAny(command, "\n\r;&|<>$`") {
		return nil, fmt.Errorf("shell syntax is not supported")
	}
	env, err := p.processEnv(ctx, req.Env)
	if err != nil {
		return nil, err
	}
	baseCtx := processContext(ctx, req.Detached)
	var runCtx context.Context
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(baseCtx, req.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(baseCtx)
	}
	workdir, err := p.workdir(req.Workdir)
	if err != nil {
		cancel()
		return nil, err
	}
	cmd := exec.CommandContext(runCtx, command, req.Args...)
	cmd.Dir = workdir
	cmd.Env = env
	configureCommandProcess(cmd)
	start := p.clock.Now()
	id := fmt.Sprintf("proc-%d", p.nextID.Add(1))
	mp := &managedProcess{
		manager: p, id: id, cmd: cmd, cancel: cancel,
		events: make(chan system.ProcessEvent, 128), done: make(chan struct{}), started: make(chan struct{}),
		stdout: cappedBuffer{max: positiveOr(req.MaxStdout, 64*1024)},
		stderr: cappedBuffer{max: positiveOr(req.MaxStderr, 64*1024)},
		info: system.ProcessInfo{
			ID: id, Label: strings.TrimSpace(req.Label), Tags: trimStrings(req.Tags), Metadata: cloneStringMap(req.Metadata),
			Command: command, Args: append([]string(nil), req.Args...), Workdir: workdir, StartedAt: start, Running: true,
		},
	}
	cmd.Stdout = processOutputWriter{process: mp, stream: "stdout"}
	cmd.Stderr = processOutputWriter{process: mp, stream: "stderr"}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	p.mu.Lock()
	p.procs[id] = mp
	p.mu.Unlock()
	mp.emit(system.ProcessEvent{ProcessID: id, Kind: system.ProcessEventStarted, Time: start})
	close(mp.started)
	go mp.wait(runCtx, start)
	return mp, nil
}

func (p *Process) Ensure(ctx context.Context, req system.ProcessRequest) (system.ProcessHandle, bool, error) {
	label := strings.TrimSpace(req.Label)
	if label != "" {
		p.mu.Lock()
		for _, proc := range p.procs {
			info := proc.Info()
			if info.Label == label && info.Running {
				p.mu.Unlock()
				return proc, false, nil
			}
		}
		p.mu.Unlock()
	}
	handle, err := p.Start(ctx, req)
	return handle, true, err
}

func (p *Process) List(context.Context) ([]system.ProcessInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]system.ProcessInfo, 0, len(p.procs))
	for _, proc := range p.procs {
		out = append(out, proc.Info())
	}
	return out, nil
}

func (p *Process) Status(_ context.Context, id string) (system.ProcessInfo, error) {
	proc, err := p.lookup(id)
	if err != nil {
		return system.ProcessInfo{}, err
	}
	return proc.Info(), nil
}

func (p *Process) Output(_ context.Context, id string) (system.ProcessOutput, error) {
	proc, err := p.lookup(id)
	if err != nil {
		return system.ProcessOutput{}, err
	}
	return proc.Output(), nil
}

func (p *Process) Wait(ctx context.Context, id string, timeout time.Duration) (system.ProcessResult, error) {
	proc, err := p.lookup(id)
	if err != nil {
		return system.ProcessResult{}, err
	}
	waitCtx := ctx
	if waitCtx == nil {
		waitCtx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(waitCtx, timeout)
		defer cancel()
	}
	return proc.Wait(waitCtx)
}

func (p *Process) Stop(_ context.Context, id string) error {
	proc, err := p.lookup(id)
	if err != nil {
		return err
	}
	proc.cancel()
	terminateCommandProcess(proc.cmd)
	return nil
}

func (p *Process) Kill(_ context.Context, id string) error {
	proc, err := p.lookup(id)
	if err != nil {
		return err
	}
	proc.cancel()
	killCommandProcess(proc.cmd)
	return nil
}

func (p *Process) lookup(id string) (*managedProcess, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	proc, ok := p.procs[id]
	if ok {
		return proc, nil
	}
	for _, candidate := range p.procs {
		info := candidate.Info()
		if info.Label == id {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("process %q not found", id)
}

func (p *Process) workdir(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return p.root, nil
	}
	var candidate string
	if filepath.IsAbs(raw) {
		candidate = filepath.Clean(raw)
	} else {
		if !validFSLikeName(raw) {
			return "", fmt.Errorf("invalid workdir %q", raw)
		}
		candidate = filepath.Join(p.root, filepath.FromSlash(raw))
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workdir is not a directory")
	}
	return candidate, nil
}

func (p *Process) processEnv(ctx context.Context, overrides []string) ([]string, error) {
	if provider, ok := p.env.(ProcessEnvironment); ok {
		return provider.ProcessEnv(ctx, overrides)
	}
	return processEnv(overrides)
}

type managedProcess struct {
	manager *Process
	id      string
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	infoMu  sync.Mutex
	info    system.ProcessInfo
	stdout  cappedBuffer
	stderr  cappedBuffer
	events  chan system.ProcessEvent
	done    chan struct{}
	started chan struct{}
	result  system.ProcessResult
	err     error
}

func (p *managedProcess) ID() string { return p.id }

func (p *managedProcess) Info() system.ProcessInfo {
	p.infoMu.Lock()
	defer p.infoMu.Unlock()
	info := p.info
	info.Args = append([]string(nil), p.info.Args...)
	info.Tags = append([]string(nil), p.info.Tags...)
	info.Metadata = cloneStringMap(p.info.Metadata)
	return info
}

func (p *managedProcess) Events() <-chan system.ProcessEvent { return p.events }

func (p *managedProcess) Wait(ctx context.Context) (system.ProcessResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-p.done:
		return p.result, p.err
	case <-ctx.Done():
		return system.ProcessResult{}, ctx.Err()
	}
}

func (p *managedProcess) Output() system.ProcessOutput {
	p.stdout.mu.Lock()
	stdout := p.stdout.String()
	stdoutTruncated := p.stdout.truncated
	p.stdout.mu.Unlock()
	p.stderr.mu.Lock()
	stderr := p.stderr.String()
	stderrTruncated := p.stderr.truncated
	p.stderr.mu.Unlock()
	return system.ProcessOutput{ProcessID: p.id, Stdout: stdout, Stderr: stderr, StdoutTruncated: stdoutTruncated, StderrTruncated: stderrTruncated}
}

func (p *managedProcess) wait(ctx context.Context, start time.Time) {
	err := p.cmd.Wait()
	ended := p.manager.clock.Now()
	duration := ended.Sub(start)
	timedOut := ctx.Err() != nil
	if timedOut {
		killCommandProcess(p.cmd)
	}
	exitCode := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			exitCode = exit.ExitCode()
		} else if timedOut {
			exitCode = -1
		}
	}
	out := p.Output()
	p.result = system.ProcessResult{
		Command: p.info.Command, Args: append([]string(nil), p.info.Args...), Workdir: p.info.Workdir,
		Stdout: out.Stdout, Stderr: out.Stderr, ExitCode: exitCode, TimedOut: timedOut,
		StdoutTruncated: out.StdoutTruncated, StderrTruncated: out.StderrTruncated, Duration: duration,
	}
	p.err = err
	if timedOut {
		p.err = ctx.Err()
	}
	p.infoMu.Lock()
	p.info.Running = false
	p.info.EndedAt = ended
	p.info.ExitCode = exitCode
	if p.err != nil && !errors.Is(p.err, context.Canceled) {
		p.info.Error = p.err.Error()
	}
	p.infoMu.Unlock()
	p.emit(system.ProcessEvent{ProcessID: p.id, Kind: system.ProcessEventExited, Time: ended, Data: fmt.Sprintf("%d", exitCode)})
	close(p.done)
	close(p.events)
}

type processOutputWriter struct {
	process *managedProcess
	stream  string
}

func (w processOutputWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if w.stream == "stderr" {
		_, _ = w.process.stderr.Write(data)
	} else {
		_, _ = w.process.stdout.Write(data)
	}
	<-w.process.started
	w.process.emit(system.ProcessEvent{ProcessID: w.process.id, Kind: system.ProcessEventOutput, Stream: w.stream, Data: string(data), Time: w.process.manager.clock.Now()})
	return len(data), nil
}

func (p *managedProcess) emit(event system.ProcessEvent) {
	select {
	case p.events <- event:
	default:
	}
}

type cappedBuffer struct {
	bytes.Buffer
	mu        sync.Mutex
	max       int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.max <= 0 {
		return len(p), nil
	}
	remaining := b.max - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func processContext(ctx context.Context, detached bool) context.Context {
	if detached || ctx == nil {
		return context.Background()
	}
	return ctx
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func validFSLikeName(name string) bool {
	name = strings.TrimSpace(filepath.ToSlash(name))
	return name == "." || (name != "" && !strings.HasPrefix(name, "../") && name != "..")
}

var _ system.ProcessManager = (*Process)(nil)
