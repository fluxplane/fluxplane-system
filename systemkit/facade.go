// Package systemkit provides ergonomic helpers over primitive system contracts.
package systemkit

import (
	"context"

	"github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/hostsystem"
)

// Facade wraps a primitive System and adds convenience methods.
type Facade struct {
	base system.System
}

// New wraps base in a Facade.
func New(base system.System) Facade {
	return Facade{base: base}
}

// NewHost constructs a host-backed system facade.
func NewHost(cfg hostsystem.Config) (Facade, error) {
	return NewSystem().
		WithHostFileSystem(cfg.Root).
		WithHostNetwork().
		WithHostEnvironment().
		WithClock(firstClock(cfg.Clock)).
		WithHostProcess(cfg.Root).
		Build()
}

// System returns the wrapped primitive system.
func (f Facade) System() system.System { return f.base }

func (f Facade) FileSystem() system.FileSystem {
	if f.base == nil {
		return nil
	}
	return f.base.FileSystem()
}

func (f Facade) Network() system.Network {
	if f.base == nil {
		return nil
	}
	return f.base.Network()
}

func (f Facade) Process() system.ProcessManager {
	if f.base == nil {
		return nil
	}
	return f.base.Process()
}

func (f Facade) Environment() system.Environment {
	if f.base == nil {
		return nil
	}
	return f.base.Environment()
}

func (f Facade) Clock() system.Clock {
	if f.base == nil {
		return nil
	}
	return f.base.Clock()
}

// ReadFileLimit reads at most maxBytes from the wrapped filesystem.
func (f Facade) ReadFileLimit(ctx context.Context, name string, maxBytes int64) ([]byte, bool, error) {
	return system.ReadFileLimit(ctx, f.FileSystem(), name, maxBytes)
}

// ReadFileLines reads a bounded 1-indexed line window from the wrapped filesystem.
func (f Facade) ReadFileLines(ctx context.Context, name string, start, end int, maxBytes int64) ([]byte, int, bool, error) {
	return system.ReadFileLines(ctx, f.FileSystem(), name, start, end, maxBytes)
}

// Walk returns a bounded tree traversal over the wrapped filesystem.
func (f Facade) Walk(ctx context.Context, root string, opts system.WalkOptions) ([]system.WalkEntry, bool, error) {
	return system.Walk(ctx, f.FileSystem(), root, opts)
}

// Glob returns paths matching pattern over the wrapped filesystem.
func (f Facade) Glob(ctx context.Context, pattern string, opts system.GlobOptions) ([]string, bool, error) {
	return system.Glob(ctx, f.FileSystem(), pattern, opts)
}

var _ system.System = Facade{}

func firstClock(clock system.Clock) system.Clock {
	if clock != nil {
		return clock
	}
	return hostsystem.RealClock{}
}
