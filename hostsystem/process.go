package hostsystem

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	root      string
	env       system.Environment
	clock     system.Clock
	mu        sync.Mutex
	nextID    atomic.Uint64
	nextSubID atomic.Uint64
	procs     map[string]*managedProcess
	groups    map[string]map[string]struct{}
	subs      map[uint64]*processSubscription
}

func NewProcess(root string, env system.Environment, clock system.Clock) *Process {
	if clock == nil {
		clock = RealClock{}
	}
	if env == nil {
		env = Environment{}
	}
	return &Process{root: root, env: env, clock: clock, procs: map[string]*managedProcess{}, groups: map[string]map[string]struct{}{}, subs: map[uint64]*processSubscription{}}
}

func (p *Process) Run(ctx context.Context, req system.ProcessRequest) (system.ProcessResult, error) {
	handle, err := p.start(ctx, req, false)
	if err != nil {
		return system.ProcessResult{}, err
	}
	return handle.Wait(ctx)
}

func (p *Process) Start(ctx context.Context, req system.ProcessRequest) (system.ProcessHandle, error) {
	return p.start(ctx, req, true)
}

func (p *Process) start(ctx context.Context, req system.ProcessRequest, interactive bool) (system.ProcessHandle, error) {
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
	if ctx != nil && ctx.Err() != nil && !req.Detached {
		return nil, ctx.Err()
	}
	baseCtx := ctx
	if baseCtx == nil || req.Detached {
		baseCtx = context.Background()
	}
	controlCtx, controlCancel := context.WithCancel(context.Background())
	var timeoutCtx context.Context
	var timeoutCancel context.CancelFunc
	if req.Timeout > 0 {
		timeoutCtx, timeoutCancel = context.WithTimeout(context.Background(), req.Timeout)
	} else {
		timeoutCtx, timeoutCancel = context.WithCancel(context.Background())
	}
	workdir, err := p.workdir(req.Workdir)
	if err != nil {
		timeoutCancel()
		controlCancel()
		return nil, err
	}
	cmd := exec.Command(command, req.Args...)
	cmd.Dir = workdir
	cmd.Env = env
	configureCommandProcess(cmd)
	start := p.clock.Now()
	id := fmt.Sprintf("proc-%d", p.nextID.Add(1))
	group := strings.TrimSpace(req.Group)
	var stdin io.WriteCloser
	if interactive {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			timeoutCancel()
			controlCancel()
			return nil, err
		}
	}
	mp := &managedProcess{
		manager: p, id: id, cmd: cmd, stdin: stdin, request: cloneProcessRequest(req), cancel: controlCancel,
		controlCtx: controlCtx, baseCtx: baseCtx, timeoutCtx: timeoutCtx, timeoutCancel: timeoutCancel,
		done: make(chan struct{}), started: make(chan struct{}),
		// Process output is streamed as events; managed processes do not retain output buffers.
		info: system.ProcessInfo{
			ID: id, Label: strings.TrimSpace(req.Label), Group: group,
			Tags: trimStrings(req.Tags), Metadata: cloneStringMap(req.Metadata),
			Command: command, Args: append([]string(nil), req.Args...), Workdir: workdir, StartedAt: start, Running: true, Detached: req.Detached,
		},
	}
	cmd.Stdout = processOutputWriter{process: mp, stream: "stdout"}
	cmd.Stderr = processOutputWriter{process: mp, stream: "stderr"}
	if ctx != nil && ctx.Err() != nil && !req.Detached {
		timeoutCancel()
		controlCancel()
		return nil, ctx.Err()
	}
	if err := cmd.Start(); err != nil {
		timeoutCancel()
		controlCancel()
		return nil, err
	}
	p.mu.Lock()
	p.procs[id] = mp
	if group != "" {
		if p.groups[group] == nil {
			p.groups[group] = map[string]struct{}{}
		}
		p.groups[group][id] = struct{}{}
	}
	p.mu.Unlock()
	mp.emit(system.ProcessEvent{ProcessID: id, Kind: system.ProcessEventStarted, Time: start})
	close(mp.started)
	go mp.monitor()
	go mp.wait(start)
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

func (p *Process) Group(name string) system.ProcessGroup {
	return processGroup{manager: p, name: strings.TrimSpace(name)}
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

func (p *Process) subscribe(ctx context.Context, selector processSelector) <-chan system.ProcessEvent {
	if ctx == nil {
		ctx = context.Background()
	}
	events := make(chan system.ProcessEvent, 128)
	subCtx, cancel := context.WithCancel(ctx)
	sub := &processSubscription{id: p.nextSubID.Add(1), manager: p, selector: normalizeProcessSelector(selector), events: events, cancel: cancel}
	p.mu.Lock()
	p.subs[sub.id] = sub
	p.mu.Unlock()
	go func() {
		<-subCtx.Done()
		sub.close()
	}()
	return events
}

func (p *Process) groupProcesses(name string) []*managedProcess {
	p.mu.Lock()
	defer p.mu.Unlock()
	name = strings.TrimSpace(name)
	members := p.groups[name]
	out := make([]*managedProcess, 0, len(members))
	for id := range members {
		proc, ok := p.procs[id]
		if ok {
			out = append(out, proc)
		}
	}
	return out
}

func (p *Process) listGroup(name string) []system.ProcessInfo {
	procs := p.groupProcesses(name)
	out := make([]system.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		out = append(out, proc.Info())
	}
	return out
}

func (p *Process) controlGroup(name string, fn func(*managedProcess) error) error {
	var errs []error
	for _, proc := range p.groupProcesses(name) {
		if err := fn(proc); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Process) broadcast(event system.ProcessEvent) {
	info, ok := p.processInfo(event.ProcessID)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, sub := range p.subs {
		if !matchesProcessSelector(sub.selector, info, event) {
			continue
		}
		select {
		case sub.events <- event:
		default:
		}
	}
}

func (p *Process) processInfo(id string) (system.ProcessInfo, bool) {
	p.mu.Lock()
	proc, ok := p.procs[id]
	p.mu.Unlock()
	if !ok {
		return system.ProcessInfo{}, false
	}
	return proc.Info(), true
}

func (p *Process) removeSubscription(id uint64) {
	p.mu.Lock()
	delete(p.subs, id)
	p.mu.Unlock()
}

func (p *Process) removeFromGroup(name, id string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	members := p.groups[name]
	if members == nil {
		return
	}
	delete(members, id)
	if len(members) == 0 {
		delete(p.groups, name)
	}
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

type processGroup struct {
	manager *Process
	name    string
}

func (g processGroup) Name() string { return g.name }

func (g processGroup) List(context.Context) ([]system.ProcessInfo, error) {
	if g.manager == nil {
		return nil, fmt.Errorf("process group %q has no manager", g.name)
	}
	return g.manager.listGroup(g.name), nil
}

func (g processGroup) Stop(ctx context.Context) error {
	return g.Signal(ctx, system.ProcessSignalTerminate)
}

func (g processGroup) Kill(ctx context.Context) error {
	return g.Signal(ctx, system.ProcessSignalKill)
}

func (g processGroup) Signal(ctx context.Context, signal system.ProcessSignal) error {
	return g.manager.controlGroup(g.name, func(proc *managedProcess) error { return proc.Signal(ctx, signal) })
}

func (g processGroup) Interrupt(ctx context.Context) error {
	return g.Signal(ctx, system.ProcessSignalInterrupt)
}

func (g processGroup) Reload(ctx context.Context) error {
	return g.Signal(ctx, system.ProcessSignalReload)
}

func (g processGroup) Pause(ctx context.Context) error {
	return g.Signal(ctx, system.ProcessSignalPause)
}

func (g processGroup) Resume(ctx context.Context) error {
	return g.Signal(ctx, system.ProcessSignalResume)
}

func (g processGroup) Subscribe(ctx context.Context) <-chan system.ProcessEvent {
	return g.manager.subscribe(ctx, processSelector{Groups: []string{g.name}})
}

func (g processGroup) Wait(ctx context.Context) (system.ProcessResult, error) {
	var errs []error
	for _, proc := range g.manager.groupProcesses(g.name) {
		_, err := proc.Wait(ctx)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return system.ProcessResult{}, errors.Join(errs...)
}

func (g processGroup) Write(ctx context.Context, data []byte) (int, error) {
	total := 0
	var errs []error
	for _, proc := range g.manager.groupProcesses(g.name) {
		n, err := proc.Write(ctx, data)
		total += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	return total, errors.Join(errs...)
}

func (g processGroup) CloseInput(ctx context.Context) error {
	return g.manager.controlGroup(g.name, func(proc *managedProcess) error { return proc.CloseInput(ctx) })
}

func (g processGroup) Restart(context.Context) (system.ProcessHandle, error) {
	return nil, errors.ErrUnsupported
}

func (g processGroup) Detach(ctx context.Context) error {
	return g.manager.controlGroup(g.name, func(proc *managedProcess) error { return proc.Detach(ctx) })
}

type managedProcess struct {
	manager       *Process
	id            string
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	request       system.ProcessRequest
	cancel        context.CancelFunc
	controlCtx    context.Context
	baseCtx       context.Context
	timeoutCtx    context.Context
	timeoutCancel context.CancelFunc
	timedOut      atomic.Bool
	inputMu       sync.Mutex
	writeMu       sync.Mutex
	infoMu        sync.Mutex
	info          system.ProcessInfo
	done          chan struct{}
	started       chan struct{}
	result        system.ProcessResult
	err           error
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

func (p *managedProcess) Stop(ctx context.Context) error {
	return p.Signal(ctx, system.ProcessSignalTerminate)
}

func (p *managedProcess) Kill(ctx context.Context) error {
	return p.Signal(ctx, system.ProcessSignalKill)
}

func (p *managedProcess) Signal(ctx context.Context, signal system.ProcessSignal) error {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return p.signal(signal)
}

func (p *managedProcess) Interrupt(ctx context.Context) error {
	return p.Signal(ctx, system.ProcessSignalInterrupt)
}

func (p *managedProcess) Reload(ctx context.Context) error {
	return p.Signal(ctx, system.ProcessSignalReload)
}

func (p *managedProcess) Pause(ctx context.Context) error {
	return p.Signal(ctx, system.ProcessSignalPause)
}

func (p *managedProcess) Resume(ctx context.Context) error {
	return p.Signal(ctx, system.ProcessSignalResume)
}

func (p *managedProcess) Write(ctx context.Context, data []byte) (int, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}
	n, err := p.writeInput(ctx, data)
	if err == nil {
		p.emit(system.ProcessEvent{ProcessID: p.id, Kind: system.ProcessEventInput, Data: fmt.Sprintf("%d", n), Time: p.manager.clock.Now()})
	}
	return n, err
}

func (p *managedProcess) CloseInput(ctx context.Context) error {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	err := p.closeInput()
	if err == nil {
		p.emit(system.ProcessEvent{ProcessID: p.id, Kind: system.ProcessEventInputClosed, Time: p.manager.clock.Now()})
	}
	return err
}

func (p *managedProcess) Restart(ctx context.Context) (system.ProcessHandle, error) {
	info := p.Info()
	if info.Running {
		waitCtx := ctx
		if waitCtx == nil {
			var cancel context.CancelFunc
			waitCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
		}
		if err := p.Stop(context.Background()); err != nil {
			return nil, err
		}
		if _, err := p.Wait(waitCtx); err != nil {
			_ = p.Kill(context.Background())
			if _, killWaitErr := p.Wait(context.Background()); killWaitErr != nil {
				return nil, errors.Join(err, killWaitErr)
			}
			if waitCtx.Err() != nil {
				return nil, waitCtx.Err()
			}
		}
	}
	restarted, err := p.manager.Start(ctx, p.Request())
	if err == nil {
		p.emit(system.ProcessEvent{ProcessID: p.id, Kind: system.ProcessEventRestarted, Data: restarted.ID(), Time: p.manager.clock.Now()})
	}
	return restarted, err
}

func (p *managedProcess) Detach(context.Context) error {
	p.setDetached(true)
	p.emit(system.ProcessEvent{ProcessID: p.id, Kind: system.ProcessEventDetached, Time: p.manager.clock.Now()})
	return nil
}

func (p *managedProcess) Subscribe(ctx context.Context) <-chan system.ProcessEvent {
	return p.manager.subscribe(ctx, processSelector{IDs: []string{p.id}})
}

func (p *managedProcess) setPaused(paused bool) {
	p.infoMu.Lock()
	defer p.infoMu.Unlock()
	if !p.info.Running {
		p.info.Paused = false
		return
	}
	p.info.Paused = paused
}

func (p *managedProcess) Request() system.ProcessRequest {
	req := cloneProcessRequest(p.request)
	info := p.Info()
	req.Label = info.Label
	req.Group = info.Group
	req.Tags = append([]string(nil), info.Tags...)
	req.Metadata = cloneStringMap(info.Metadata)
	return req
}

func (p *managedProcess) signal(signal system.ProcessSignal) error {
	if err := signalCommandProcess(p.cmd, signal); err != nil {
		return err
	}
	if signal == system.ProcessSignalTerminate || signal == system.ProcessSignalKill {
		p.cancel()
	}
	eventKind := system.ProcessEventSignaled
	switch signal {
	case system.ProcessSignalTerminate:
		eventKind = system.ProcessEventStopped
	case system.ProcessSignalKill:
		eventKind = system.ProcessEventKilled
	case system.ProcessSignalInterrupt:
		eventKind = system.ProcessEventInterrupted
	case system.ProcessSignalReload:
		eventKind = system.ProcessEventReloaded
	case system.ProcessSignalPause:
		eventKind = system.ProcessEventPaused
		p.setPaused(true)
	case system.ProcessSignalResume:
		eventKind = system.ProcessEventResumed
		p.setPaused(false)
	}
	p.emit(system.ProcessEvent{ProcessID: p.id, Kind: eventKind, Data: string(signal), Time: p.manager.clock.Now()})
	return nil
}

func (p *managedProcess) setDetached(detached bool) {
	p.infoMu.Lock()
	defer p.infoMu.Unlock()
	p.info.Detached = detached
}

func (p *managedProcess) detached() bool {
	p.infoMu.Lock()
	defer p.infoMu.Unlock()
	return p.info.Detached
}

func (p *managedProcess) writeInput(ctx context.Context, data []byte) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	type writeResult struct {
		n   int
		err error
	}
	done := make(chan writeResult, 1)
	go func() {
		p.writeMu.Lock()
		defer p.writeMu.Unlock()
		p.inputMu.Lock()
		stdin := p.stdin
		p.inputMu.Unlock()
		if stdin == nil {
			done <- writeResult{err: fmt.Errorf("process %q input is closed", p.id)}
			return
		}
		n, err := stdin.Write(data)
		done <- writeResult{n: n, err: err}
	}()
	select {
	case result := <-done:
		return result.n, result.err
	case <-ctx.Done():
		_ = p.closeInput()
		return 0, ctx.Err()
	}
}

func (p *managedProcess) closeInput() error {
	p.inputMu.Lock()
	defer p.inputMu.Unlock()
	if p.stdin == nil {
		return nil
	}
	err := p.stdin.Close()
	p.stdin = nil
	return err
}

func (p *managedProcess) monitor() {
	baseDone := p.baseCtx.Done()
	for {
		select {
		case <-p.done:
			return
		case <-p.controlCtx.Done():
			_ = terminateCommandProcess(p.cmd)
			return
		case <-p.timeoutCtx.Done():
			if errors.Is(p.timeoutCtx.Err(), context.DeadlineExceeded) {
				p.timedOut.Store(true)
				_ = killCommandProcess(p.cmd)
			}
			return
		case <-baseDone:
			if !p.detached() {
				_ = terminateCommandProcess(p.cmd)
				return
			}
			baseDone = nil
		}
	}
}

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

func (p *managedProcess) wait(start time.Time) {
	err := p.cmd.Wait()
	ended := p.manager.clock.Now()
	duration := ended.Sub(start)
	timedOut := p.timedOut.Load()
	exitCode := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			exitCode = exit.ExitCode()
		} else if timedOut {
			exitCode = -1
		}
	}
	info := p.Info()
	p.result = system.ProcessResult{
		Command: info.Command, Args: append([]string(nil), info.Args...), Workdir: info.Workdir,
		ExitCode: exitCode, TimedOut: timedOut, Duration: duration,
	}
	p.err = err
	if timedOut {
		p.err = context.DeadlineExceeded
	}
	p.infoMu.Lock()
	p.info.Running = false
	p.info.Paused = false
	p.info.EndedAt = ended
	p.info.ExitCode = exitCode
	group := p.info.Group
	if p.err != nil && !errors.Is(p.err, context.Canceled) {
		p.info.Error = p.err.Error()
	}
	p.infoMu.Unlock()
	p.manager.removeFromGroup(group, p.id)
	p.timeoutCancel()
	_ = p.closeInput()

	p.emit(system.ProcessEvent{ProcessID: p.id, Kind: system.ProcessEventExited, Time: ended, Data: fmt.Sprintf("%d", exitCode)})
	close(p.done)

}

type processOutputWriter struct {
	process *managedProcess
	stream  string
}

func (w processOutputWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if w.process.cmd.Process != nil {
		<-w.process.started
		w.process.emit(system.ProcessEvent{ProcessID: w.process.id, Kind: system.ProcessEventOutput, Stream: w.stream, Data: string(data), Time: w.process.manager.clock.Now()})
	}
	return len(data), nil
}

func (p *managedProcess) emit(event system.ProcessEvent) {
	p.manager.broadcast(event)
}

func validFSLikeName(name string) bool {
	name = strings.TrimSpace(filepath.ToSlash(name))
	return name == "." || (name != "" && !strings.HasPrefix(name, "../") && name != "..")
}

var _ system.ProcessManager = (*Process)(nil)

type processSubscription struct {
	id       uint64
	manager  *Process
	selector processSelector
	events   chan system.ProcessEvent
	cancel   context.CancelFunc
	once     sync.Once
}

func (s *processSubscription) close() {
	s.once.Do(func() {
		s.manager.removeSubscription(s.id)
		close(s.events)
		s.cancel()
	})
}

type processSelector struct {
	IDs     []string
	Groups  []string
	Labels  []string
	Streams []string
}

func normalizeProcessSelector(selector processSelector) processSelector {
	return processSelector{
		IDs:     trimStrings(selector.IDs),
		Groups:  trimStrings(selector.Groups),
		Labels:  trimStrings(selector.Labels),
		Streams: trimStrings(selector.Streams),
	}
}

func matchesProcessSelector(selector processSelector, info system.ProcessInfo, event system.ProcessEvent) bool {
	return matchesStringSelector(selector.IDs, info.ID) &&
		matchesStringSelector(selector.Groups, info.Group) &&
		matchesStringSelector(selector.Labels, info.Label) &&
		matchesStringSelector(selector.Streams, event.Stream)
}

func matchesStringSelector(values []string, value string) bool {
	if len(values) == 0 {
		return true
	}
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func cloneProcessRequest(req system.ProcessRequest) system.ProcessRequest {
	req.Args = append([]string(nil), req.Args...)
	req.Env = append([]string(nil), req.Env...)
	req.Tags = append([]string(nil), req.Tags...)
	req.Metadata = cloneStringMap(req.Metadata)
	return req
}
