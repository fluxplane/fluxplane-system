//go:build windows

package hostsystem

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/fluxplane/fluxplane-system"
)

func configureCommandProcess(*exec.Cmd) {}

func terminateCommandProcess(cmd *exec.Cmd) error {
	return killCommandProcess(cmd)
}

func killCommandProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

func pauseCommandProcess(*exec.Cmd) error {
	return errors.ErrUnsupported
}

func resumeCommandProcess(*exec.Cmd) error {
	return errors.ErrUnsupported
}

func signalCommandProcess(cmd *exec.Cmd, signal system.ProcessSignal) error {
	switch signal {
	case system.ProcessSignalTerminate, system.ProcessSignalKill, system.ProcessSignalInterrupt:
		return killCommandProcess(cmd)
	case system.ProcessSignalPause, system.ProcessSignalResume, system.ProcessSignalReload:
		return errors.ErrUnsupported
	default:
		return fmt.Errorf("unsupported process signal %q", signal)
	}
}
