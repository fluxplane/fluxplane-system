// Package systemtest provides small fakes and helpers for tests.
package systemtest

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"sync"

	"github.com/fluxplane/fluxplane-system"
)

// UnsupportedNetwork rejects primitive network access.
type UnsupportedNetwork struct{}

func (UnsupportedNetwork) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.ErrUnsupported
}

func (UnsupportedNetwork) Resolver() system.Resolver {
	return UnsupportedResolver{}
}

// UnsupportedResolver rejects DNS lookups.
type UnsupportedResolver struct{}

func (UnsupportedResolver) LookupHost(context.Context, string) ([]string, error) {
	return nil, errors.ErrUnsupported
}
func (UnsupportedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, errors.ErrUnsupported
}
func (UnsupportedResolver) LookupCNAME(context.Context, string) (string, error) {
	return "", errors.ErrUnsupported
}
func (UnsupportedResolver) LookupMX(context.Context, string) ([]*net.MX, error) {
	return nil, errors.ErrUnsupported
}
func (UnsupportedResolver) LookupSRV(context.Context, string, string, string) (string, []*net.SRV, error) {
	return "", nil, errors.ErrUnsupported
}
func (UnsupportedResolver) LookupTXT(context.Context, string) ([]string, error) {
	return nil, errors.ErrUnsupported
}

// RecordingNetwork records dial attempts.
type RecordingNetwork struct {
	mu     sync.Mutex
	Dials  []Dial
	Dialer func(context.Context, string, string) (net.Conn, error)
}

func (n *RecordingNetwork) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	n.mu.Lock()
	n.Dials = append(n.Dials, Dial{Network: network, Address: address})
	dialer := n.Dialer
	defer n.mu.Unlock()
	if dialer != nil {
		return dialer(ctx, network, address)
	}
	return nil, errors.ErrUnsupported
}

// Dial records one network dial attempt.
type Dial struct {
	Network string
	Address string
}

func (n *RecordingNetwork) Resolver() system.Resolver {
	return UnsupportedResolver{}
}

// FailingFileSystem is a nil-safe filesystem placeholder for tests that should
// fail before touching files.
type FailingFileSystem struct {
	Err error
}

func (f FailingFileSystem) err() error {
	if f.Err != nil {
		return f.Err
	}
	return errors.ErrUnsupported
}

func (f FailingFileSystem) Open(string) (fs.File, error)          { return nil, f.err() }
func (f FailingFileSystem) Stat(string) (fs.FileInfo, error)      { return nil, f.err() }
func (f FailingFileSystem) ReadDir(string) ([]fs.DirEntry, error) { return nil, f.err() }
func (f FailingFileSystem) ReadFile(string) ([]byte, error)       { return nil, f.err() }
func (f FailingFileSystem) WriteFile(context.Context, string, []byte, system.WriteFileOptions) error {
	return f.err()
}
func (f FailingFileSystem) MkdirAll(context.Context, string, system.MkdirOptions) error {
	return f.err()
}
func (f FailingFileSystem) Remove(context.Context, string) error { return f.err() }
func (f FailingFileSystem) Rename(context.Context, string, string, system.RenameOptions) error {
	return f.err()
}

// StaticEnvironment is a map-backed test environment.
type StaticEnvironment map[string]string

func (e StaticEnvironment) Lookup(_ context.Context, key string) (string, bool, error) {
	value, ok := e[key]
	return value, ok, nil
}

var _ system.FileSystem = FailingFileSystem{}
