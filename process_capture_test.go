package system_test

import (
	"context"
	"os"
	"testing"
	"time"

	system "github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/hostsystem"
)

func TestRunProcessCaptureCollectsOutput(t *testing.T) {
	proc := hostsystem.NewProcess(t.TempDir(), hostsystem.Environment{}, hostsystem.RealClock{})
	capture, err := system.RunProcessCapture(context.Background(), proc, system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_capture_child__", "hello", "warning"},
		Timeout: 5 * time.Second,
	}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Result.ExitCode != 0 {
		t.Fatalf("result = %#v", capture.Result)
	}
	if capture.Stdout != "hello" || capture.Stderr != "warning" {
		t.Fatalf("capture stdout/stderr = %q/%q, want hello/warning", capture.Stdout, capture.Stderr)
	}
}

func TestRunProcessCaptureTruncatesStreams(t *testing.T) {
	proc := hostsystem.NewProcess(t.TempDir(), hostsystem.Environment{}, hostsystem.RealClock{})
	capture, err := system.RunProcessCapture(context.Background(), proc, system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_capture_child__", "abcdef", ""},
		Timeout: 5 * time.Second,
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Stdout != "abc" || !capture.StdoutTruncated {
		t.Fatalf("stdout/truncated = %q/%v, want abc/true", capture.Stdout, capture.StdoutTruncated)
	}
}

func testCommand(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__process_capture_child__" {
		if len(os.Args) > 2 {
			_, _ = os.Stdout.WriteString(os.Args[2])
		}
		if len(os.Args) > 3 {
			_, _ = os.Stderr.WriteString(os.Args[3])
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
