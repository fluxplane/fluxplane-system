//go:build !windows

package hostsystem

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/fluxplane/fluxplane-system"
)

func configureCommandProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateCommandProcess(cmd *exec.Cmd) error {
	return signalSyscallCommandProcess(cmd, syscall.SIGTERM)
}

func killCommandProcess(cmd *exec.Cmd) error {
	return signalSyscallCommandProcess(cmd, syscall.SIGKILL)
}

func pauseCommandProcess(cmd *exec.Cmd) error {
	return signalSyscallCommandProcess(cmd, syscall.SIGSTOP)
}

func resumeCommandProcess(cmd *exec.Cmd) error {
	return signalSyscallCommandProcess(cmd, syscall.SIGCONT)
}

func signalCommandProcess(cmd *exec.Cmd, signal system.ProcessSignal) error {
	switch signal {
	case system.ProcessSignalTerminate:
		return terminateCommandProcess(cmd)
	case system.ProcessSignalKill:
		return killCommandProcess(cmd)
	case system.ProcessSignalInterrupt:
		return signalSyscallCommandProcess(cmd, syscall.SIGINT)
	case system.ProcessSignalPause:
		return pauseCommandProcess(cmd)
	case system.ProcessSignalResume:
		return resumeCommandProcess(cmd)
	case system.ProcessSignalReload:
		return signalSyscallCommandProcess(cmd, syscall.SIGHUP)
	default:
		return fmt.Errorf("unsupported process signal %q", signal)
	}
}

func signalSyscallCommandProcess(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, signal)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}
