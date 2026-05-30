package system

import (
	"context"
	"time"

	"github.com/fluxplane/fluxplane-event"
)

const (
	ProcessEventStarted     = "started"
	ProcessEventOutput      = "output"
	ProcessEventExited      = "exited"
	ProcessEventStopped     = "stopped"
	ProcessEventKilled      = "killed"
	ProcessEventSignaled    = "signaled"
	ProcessEventInterrupted = "interrupted"
	ProcessEventReloaded    = "reloaded"
	ProcessEventPaused      = "paused"
	ProcessEventResumed     = "resumed"
	ProcessEventInput       = "input"
	ProcessEventInputClosed = "input_closed"
	ProcessEventRestarted   = "restarted"
	ProcessEventDetached    = "detached"

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
	Group(string) ProcessGroup
	List(context.Context) ([]ProcessInfo, error)
}

// ProcessGroup controls the managed processes assigned to one group.
type ProcessGroup interface {
	ProcessControl
	Name() string
	List(context.Context) ([]ProcessInfo, error)
}

// ProcessSignal names a portable process control signal.
type ProcessSignal string

const (
	ProcessSignalTerminate ProcessSignal = "terminate"
	ProcessSignalKill      ProcessSignal = "kill"
	ProcessSignalInterrupt ProcessSignal = "interrupt"
	ProcessSignalPause     ProcessSignal = "pause"
	ProcessSignalResume    ProcessSignal = "resume"
	ProcessSignalReload    ProcessSignal = "reload"
)

// ProcessControl controls or observes a process or process group.
type ProcessControl interface {
	Subscribe(context.Context) <-chan ProcessEvent
	Wait(context.Context) (ProcessResult, error)
	Stop(context.Context) error
	Kill(context.Context) error
	Signal(context.Context, ProcessSignal) error
	Interrupt(context.Context) error
	Reload(context.Context) error
	Pause(context.Context) error
	Resume(context.Context) error
	Write(context.Context, []byte) (int, error)
	CloseInput(context.Context) error
	Restart(context.Context) (ProcessHandle, error)
	Detach(context.Context) error
}

// ProcessHandle identifies a running or completed managed process.
type ProcessHandle interface {
	ProcessControl
	ID() string
	Info() ProcessInfo
}

// ProcessRequest describes one bounded process execution.
type ProcessRequest struct {
	Command  string
	Args     []string
	Workdir  string
	Env      []string
	Timeout  time.Duration
	Detached bool
	Label    string
	Group    string
	Tags     []string
	Metadata map[string]string
}

// ProcessInfo describes a managed process.
type ProcessInfo struct {
	ID        string            `json:"id"`
	Label     string            `json:"label,omitempty"`
	Group     string            `json:"group,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Workdir   string            `json:"workdir,omitempty"`
	StartedAt time.Time         `json:"started_at,omitempty"`
	EndedAt   time.Time         `json:"ended_at,omitempty"`
	Running   bool              `json:"running,omitempty"`
	Paused    bool              `json:"paused,omitempty"`
	ExitCode  int               `json:"exit_code,omitempty"`
	Error     string            `json:"error,omitempty"`
	Detached  bool              `json:"detached,omitempty"`
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
		return event.Name("process." + e.Kind)
	}
}

// ProcessResult is the captured process outcome.
type ProcessResult struct {
	Command  string        `json:"command"`
	Args     []string      `json:"args,omitempty"`
	Workdir  string        `json:"workdir,omitempty"`
	ExitCode int           `json:"exit_code"`
	TimedOut bool          `json:"timed_out,omitempty"`
	Duration time.Duration `json:"-"`
}
