// Package hostsystem provides local host implementations of system contracts.
package hostsystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

// Config configures a local host-backed system.
type Config struct {
	// Root is the base directory for relative FileSystem and Process workdir
	// names. FileSystem operations are confined to it; Process execution is
	// still normal host execution and not a sandbox.
	Root string

	Clock   system.Clock
	Network NetworkConfig
}

// Host is the local host-backed System implementation.
type Host struct {
	files   *FileSystem
	network *Network
	process *Process
	env     Environment
	clock   system.Clock
}

// New constructs a local host-backed System.
func New(cfg Config) (*Host, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = wd
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	clock := cfg.Clock
	if clock == nil {
		clock = RealClock{}
	}
	files := NewFileSystem(abs)
	env := Environment{}
	return &Host{
		files:   files,
		network: NewNetwork(cfg.Network),
		process: NewProcess(abs, env, clock),
		env:     env,
		clock:   clock,
	}, nil
}

func (h *Host) FileSystem() system.FileSystem { return h.files }
func (h *Host) Network() system.Network       { return h.network }
func (h *Host) Process() system.ProcessManager {
	if h == nil {
		return nil
	}
	return h.process
}
func (h *Host) Environment() system.Environment { return h.env }
func (h *Host) Clock() system.Clock             { return h.clock }

// RealClock implements Clock with the process wall clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func defaultPerm(value, fallback os.FileMode) os.FileMode {
	if value == 0 {
		return fallback
	}
	return value
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func trimStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, 0, len(input))
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func errUnsupported(name string) error {
	return fmt.Errorf("hostsystem: %s is unsupported", name)
}
