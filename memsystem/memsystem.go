// Package memsystem provides deterministic in-memory system implementations.
package memsystem

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

// System is an in-memory implementation of system.System.
type System struct {
	files *FileSystem
	env   *Environment
	clock *Clock
}

// New returns an in-memory system.
func New() *System {
	return &System{
		files: NewFileSystem(),
		env:   NewEnvironment(nil),
		clock: NewClock(time.Unix(1700000000, 0).UTC()),
	}
}

func (s *System) FileSystem() system.FileSystem   { return s.files }
func (s *System) Network() system.Network         { return unsupportedNetwork{} }
func (s *System) Process() system.ProcessManager  { return nil }
func (s *System) Environment() system.Environment { return s.env }
func (s *System) Clock() system.Clock             { return s.clock }

// Clock is a manually advanced deterministic clock.
type Clock struct {
	mu  sync.Mutex
	now time.Time
}

func NewClock(now time.Time) *Clock {
	if now.IsZero() {
		now = time.Unix(1700000000, 0).UTC()
	}
	return &Clock{now: now}
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func (c *Clock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

func (c *Clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if d > 0 {
		c.Advance(d)
	}
	return contextErr(ctx)
}

// Environment is a map-backed environment.
type Environment struct {
	mu     sync.Mutex
	values map[string]string
}

func NewEnvironment(values map[string]string) *Environment {
	out := map[string]string{}
	for key, value := range values {
		out[key] = value
	}
	return &Environment{values: out}
}

func (e *Environment) Lookup(_ context.Context, key string) (string, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	value, ok := e.values[key]
	return value, ok, nil
}

func (e *Environment) Set(key, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.values[key] = value
}

type unsupportedNetwork struct{}

func (unsupportedNetwork) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.ErrUnsupported
}

func (unsupportedNetwork) Resolver() system.Resolver {
	return unsupportedResolver{}
}

type unsupportedResolver struct{}

func (unsupportedResolver) LookupHost(context.Context, string) ([]string, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedResolver) LookupCNAME(context.Context, string) (string, error) {
	return "", errors.ErrUnsupported
}
func (unsupportedResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedResolver) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errors.ErrUnsupported
}
func (unsupportedResolver) LookupTXT(context.Context, string) ([]string, error) {
	return nil, errors.ErrUnsupported
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
