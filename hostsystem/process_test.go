package hostsystem

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	system "github.com/fluxplane/fluxplane-system"
)

func TestProcessRunStreamsWithoutBufferingOutput(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	result, err := proc.Run(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "abcdef"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
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

func TestProcessRunClosesInput(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	result, err := proc.Run(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "--stdin"},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestProcessGroupListsMembersAndControlsPausedState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process pause/resume is not supported on windows")
	}
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	handle, err := proc.Start(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "--sleep"},
		Group:   "workers",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Kill(context.Background()) }()

	group := proc.Group("workers")
	members, err := group.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].ID != handle.ID() || members[0].Group != "workers" {
		t.Fatalf("members = %#v", members)
	}

	if err := group.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	info := handle.Info()
	if !info.Paused {
		t.Fatalf("paused info = %#v", info)
	}

	if err := group.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	info = handle.Info()
	if info.Paused {
		t.Fatalf("resumed info = %#v", info)
	}
}

func TestProcessGroupRemovesMembersOnExit(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	handle, err := proc.Start(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "done"},
		Group:   "short-lived",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	members, err := proc.Group("short-lived").List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 0 {
		t.Fatalf("members after exit = %#v, want none", members)
	}

	info := handle.Info()
	if info.Group != "short-lived" || info.Running {
		t.Fatalf("process history = %#v", info)
	}
}

func TestProcessWriteCloseInputAndRestart(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	handle, err := proc.Start(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "--stdin"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := handle.Subscribe(ctx)
	if _, err := handle.Write(context.Background(), []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := handle.CloseInput(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitForProcessEvent(t, events, system.ProcessEventOutput, "stdout", "hello") {
		t.Fatal("missing stdout event")
	}
	result, err := handle.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}

	restarted, err := handle.Restart(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restartCtx, restartCancel := context.WithCancel(context.Background())
	defer restartCancel()
	restartEvents := restarted.Subscribe(restartCtx)
	if _, err := restarted.Write(context.Background(), []byte("again")); err != nil {
		t.Fatal(err)
	}
	if err := restarted.CloseInput(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitForProcessEvent(t, restartEvents, system.ProcessEventOutput, "stdout", "again") {
		t.Fatal("missing restarted stdout event")
	}
	result, err = restarted.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("restarted result = %#v", result)
	}
}

func TestProcessGroupSubscribeReceivesOutputEvents(t *testing.T) {
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := proc.Group("readers").Subscribe(ctx)
	handle, err := proc.Start(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "event"},
		Group:   "readers",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = handle.Wait(context.Background()) }()

	if !waitForProcessEvent(t, events, system.ProcessEventOutput, "stdout", "event") {
		t.Fatal("timed out waiting for output event")
	}
}

func TestProcessSubscribeFansOutControlEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process pause/resume is not supported on windows")
	}
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	handle, err := proc.Start(context.Background(), system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "--sleep"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Kill(context.Background()) }()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	events1 := handle.Subscribe(ctx1)
	events2 := handle.Subscribe(ctx2)
	if err := handle.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !waitForProcessEvent(t, events1, system.ProcessEventPaused, "", string(system.ProcessSignalPause)) {
		t.Fatal("subscriber 1 missed pause event")
	}
	if !waitForProcessEvent(t, events2, system.ProcessEventPaused, "", string(system.ProcessSignalPause)) {
		t.Fatal("subscriber 2 missed pause event")
	}
}

func waitForProcessEvent(t *testing.T, events <-chan system.ProcessEvent, kind, stream, data string) bool {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return false
			}
			if event.Kind == kind && (stream == "" || event.Stream == stream) && (data == "" || event.Data == data) {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

func TestProcessDetachSurvivesParentContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	proc := NewProcess(t.TempDir(), Environment{}, RealClock{})
	handle, err := proc.Start(ctx, system.ProcessRequest{
		Command: testCommand(t),
		Args:    []string{"__process_child__", "--sleep"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Kill(context.Background()) }()
	if err := handle.Detach(context.Background()); err != nil {
		t.Fatal(err)
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
	info := handle.Info()
	if !info.Running || !info.Detached {
		t.Fatalf("info = %#v", info)
	}
}

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__process_child__" {
		if len(os.Args) > 2 && os.Args[2] == "--sleep" {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
		if len(os.Args) > 2 && os.Args[2] == "--stdin" {
			_, _ = io.Copy(os.Stdout, os.Stdin)
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
