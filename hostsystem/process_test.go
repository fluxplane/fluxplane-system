package hostsystem

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	system "github.com/fluxplane/fluxplane-system"
)

func TestProcessRunCapturesAndTruncatesOutput(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	result, err := proc.Run(context.Background(), system.ProcessRequest{
		Command:   testCommand(t),
		Args:      []string{"__process_child__", "abcdef"},
		MaxStdout: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "abc" || !result.StdoutTruncated {
		t.Fatalf("result = %#v", result)
	}
}

func TestProcessRunTimeout(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	_, err := proc.Run(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "--sleep"},
		Timeout: 20 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__process_child__" {
		if len(os.Args) > 2 && os.Args[2] == "--sleep" {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
		if len(os.Args) > 2 {
			_, _ = os.Stdout.WriteString(os.Args[2])
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func testCommand(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		return exe
	}
	path, err := exec.LookPath(exe)
	if err != nil {
		return exe
	}
	return path
}
