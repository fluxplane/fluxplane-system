package system

import (
	"context"
	"time"

	"github.com/fluxplane/fluxplane-event"
)

const (
	ProcessEventStarted = "started"
	ProcessEventOutput  = "output"
	ProcessEventExited  = "exited"

	EventProcessStarted event.Name = "process.started"
	EventProcessOutput  event.Name = "process.output"
	EventProcessExited  event.Name = "process.exited"
)

// ProcessRunner runs a process through a system boundary.
type ProcessRunner interface {
	Run(context.Context, ProcessRequest) (ProcessResult, error)
}

// ProcessManager owns foreground and managed background process execution.
type ProcessManager interface {
	ProcessRunner
	Start(context.Context, ProcessRequest) (ProcessHandle, error)
	Ensure(context.Context, ProcessRequest) (ProcessHandle, bool, error)
	List(context.Context) ([]ProcessInfo, error)
	Status(context.Context, string) (ProcessInfo, error)
	Output(context.Context, string) (ProcessOutput, error)
	Wait(context.Context, string, time.Duration) (ProcessResult, error)
	Stop(context.Context, string) error
	Kill(context.Context, string) error
}

// ProcessHandle identifies a running or completed managed process.
type ProcessHandle interface {
	ID() string
	Info() ProcessInfo
	Events() <-chan ProcessEvent
	Wait(context.Context) (ProcessResult, error)
}

// ProcessRequest describes one bounded process execution.
type ProcessRequest struct {
	Command   string
	Args      []string
	Workdir   string
	Env       []string
	Timeout   time.Duration
	Detached  bool
	MaxStdout int
	MaxStderr int
	Label     string
	Tags      []string
	Metadata  map[string]string
}

// ProcessInfo describes a managed process.
type ProcessInfo struct {
	ID        string            `json:"id"`
	Label     string            `json:"label,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Workdir   string            `json:"workdir,omitempty"`
	StartedAt time.Time         `json:"started_at,omitempty"`
	EndedAt   time.Time         `json:"ended_at,omitempty"`
	Running   bool              `json:"running,omitempty"`
	ExitCode  int               `json:"exit_code,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// ProcessEvent is emitted for streaming process output and lifecycle changes.
type ProcessEvent struct {
	ProcessID string    `json:"process_id"`
	Kind      string    `json:"kind"`
	Stream    string    `json:"stream,omitempty"`
	Data      string    `json:"data,omitempty"`
	Time      time.Time `json:"time,omitempty"`
}

// EventName returns the event payload name.
func (e ProcessEvent) EventName() event.Name {
	switch e.Kind {
	case ProcessEventStarted:
		return EventProcessStarted
	case ProcessEventExited:
		return EventProcessExited
	default:
		return EventProcessOutput
	}
}

// ProcessResult is the captured process outcome.
type ProcessResult struct {
	Command         string        `json:"command"`
	Args            []string      `json:"args,omitempty"`
	Workdir         string        `json:"workdir,omitempty"`
	Stdout          string        `json:"stdout,omitempty"`
	Stderr          string        `json:"stderr,omitempty"`
	ExitCode        int           `json:"exit_code"`
	TimedOut        bool          `json:"timed_out,omitempty"`
	StdoutTruncated bool          `json:"stdout_truncated,omitempty"`
	StderrTruncated bool          `json:"stderr_truncated,omitempty"`
	Duration        time.Duration `json:"-"`
}

// ProcessOutput is a bounded output snapshot for a managed process.
type ProcessOutput struct {
	ProcessID       string `json:"process_id"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated,omitempty"`
	StderrTruncated bool   `json:"stderr_truncated,omitempty"`
}
