package system

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
)

const defaultProcessCaptureLimit int64 = 1 << 20

var processCaptureGroupID atomic.Uint64

// CapturedProcessResult is a ProcessResult plus bounded stdout/stderr collected
// from ProcessEventOutput events.
type CapturedProcessResult struct {
	Result          ProcessResult `json:"result"`
	Stdout          string        `json:"stdout,omitempty"`
	Stderr          string        `json:"stderr,omitempty"`
	StdoutTruncated bool          `json:"stdout_truncated,omitempty"`
	StderrTruncated bool          `json:"stderr_truncated,omitempty"`
}

// RunProcessCapture starts req through manager, closes stdin, waits for the
// process to finish, and returns its final result plus bounded streamed output.
//
// The process manager intentionally does not retain output snapshots. This
// helper is for callers and tests that need foreground-command semantics with a
// final stdout/stderr value. When req.Group is empty the helper assigns a
// private temporary group so it can subscribe before process start and avoid
// missing short-lived output. When req.Group is non-empty, events are collected
// from that group and filtered to the started process id.
func RunProcessCapture(ctx context.Context, manager ProcessManager, req ProcessRequest, maxBytes int64) (CapturedProcessResult, error) {
	if manager == nil {
		return CapturedProcessResult{}, fmt.Errorf("process manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if maxBytes <= 0 {
		maxBytes = defaultProcessCaptureLimit
	}
	if strings.TrimSpace(req.Group) == "" {
		req.Group = fmt.Sprintf("process-capture-%d", processCaptureGroupID.Add(1))
	}

	eventCtx, cancelEvents := context.WithCancel(ctx)
	defer cancelEvents()
	events := manager.Group(req.Group).Subscribe(eventCtx)

	handle, err := manager.Start(ctx, req)
	if err != nil {
		return CapturedProcessResult{}, err
	}
	_ = handle.CloseInput(context.Background())

	captured := collectProcessEvents(events, handle.ID(), maxBytes)
	result, waitErr := handle.Wait(ctx)
	cancelEvents()
	capture := <-captured
	capture.Result = result
	capture.Result.Stdout = capture.Stdout
	capture.Result.Stderr = capture.Stderr
	capture.Result.StdoutTruncated = capture.StdoutTruncated
	capture.Result.StderrTruncated = capture.StderrTruncated
	return capture, waitErr
}

func collectProcessEvents(events <-chan ProcessEvent, processID string, maxBytes int64) <-chan CapturedProcessResult {
	out := make(chan CapturedProcessResult, 1)
	go func() {
		defer close(out)
		var capture CapturedProcessResult
		var stdout, stderr strings.Builder
		var stdoutBytes, stderrBytes int64
		for event := range events {
			if event.ProcessID != processID || event.Kind != ProcessEventOutput {
				continue
			}
			switch event.Stream {
			case "stdout":
				stdoutBytes, capture.StdoutTruncated = appendCaptured(&stdout, stdoutBytes, event.Data, maxBytes, capture.StdoutTruncated)
			case "stderr":
				stderrBytes, capture.StderrTruncated = appendCaptured(&stderr, stderrBytes, event.Data, maxBytes, capture.StderrTruncated)
			}
		}
		capture.Stdout = stdout.String()
		capture.Stderr = stderr.String()
		out <- capture
	}()
	return out
}

func appendCaptured(builder *strings.Builder, size int64, data string, maxBytes int64, truncated bool) (int64, bool) {
	if truncated || data == "" {
		return size, truncated
	}
	remaining := maxBytes - size
	if remaining <= 0 {
		return size, true
	}
	if int64(len(data)) > remaining {
		_, _ = builder.WriteString(data[:int(remaining)])
		return maxBytes, true
	}
	_, _ = builder.WriteString(data)
	return size + int64(len(data)), false
}
