package systemkit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"time"

	"github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/hostsystem"
	"github.com/fluxplane/fluxplane-system/mountfs"
)

// Builder assembles a System from independently supplied capabilities.
type Builder struct {
	sys assembledSystem
	err error
}

// NewSystem returns a builder with unsupported capabilities and a real clock.
func NewSystem() Builder {
	return Builder{sys: assembledSystem{
		fileSystem:  unsupportedFileSystem{},
		network:     unsupportedNetwork{},
		process:     unsupportedProcess{},
		environment: unsupportedEnvironment{},
		clock:       hostsystem.RealClock{},
	}}
}

func (b Builder) WithFileSystem(fileSystem system.FileSystem) Builder {
	if fileSystem == nil {
		return b.withError("filesystem is nil")
	}
	b.sys.fileSystem = fileSystem
	return b
}

func (b Builder) WithHostFileSystem(root string) Builder {
	return b.WithFileSystem(hostsystem.NewFileSystem(root))
}

func (b Builder) WithMountedFileSystem(spec mountfs.Spec) Builder {
	mounted, err := mountfs.New(b.sys.fileSystem, spec)
	if err != nil {
		return b.withError(err.Error())
	}
	b.sys.fileSystem = mounted
	return b
}

func (b Builder) WithoutFileSystem() Builder {
	b.sys.fileSystem = unsupportedFileSystem{}
	return b
}

func (b Builder) WithNetwork(network system.Network) Builder {
	if network == nil {
		return b.withError("network is nil")
	}
	b.sys.network = network
	return b
}

func (b Builder) WithHostNetwork() Builder {
	return b.WithNetwork(hostsystem.NewNetwork())
}

func (b Builder) WithoutNetwork() Builder {
	b.sys.network = unsupportedNetwork{}
	return b
}

func (b Builder) WithProcess(process system.ProcessManager) Builder {
	if process == nil {
		return b.withError("process is nil")
	}
	b.sys.process = process
	return b
}

func (b Builder) WithHostProcess(root string) Builder {
	return b.WithProcess(hostsystem.NewProcess(root, hostsystem.Environment{}, b.sys.clock))
}

func (b Builder) WithoutProcess() Builder {
	b.sys.process = unsupportedProcess{}
	return b
}

func (b Builder) WithEnvironment(environment system.Environment) Builder {
	if environment == nil {
		return b.withError("environment is nil")
	}
	b.sys.environment = environment
	return b
}

func (b Builder) WithHostEnvironment() Builder {
	return b.WithEnvironment(hostsystem.Environment{})
}

func (b Builder) WithoutEnvironment() Builder {
	b.sys.environment = unsupportedEnvironment{}
	return b
}

func (b Builder) WithClock(clock system.Clock) Builder {
	if clock == nil {
		return b.withError("clock is nil")
	}
	b.sys.clock = clock
	return b
}

func (b Builder) WithRealClock() Builder {
	return b.WithClock(hostsystem.RealClock{})
}

// Build returns an ergonomic facade over the assembled primitive system.
func (b Builder) Build() (Facade, error) {
	if b.err != nil {
		return Facade{}, b.err
	}
	return New(b.sys), nil
}

func (b Builder) withError(message string) Builder {
	if b.err == nil {
		b.err = fmt.Errorf("systemkit: %s", message)
	}
	return b
}

type assembledSystem struct {
	fileSystem  system.FileSystem
	network     system.Network
	process     system.ProcessManager
	environment system.Environment
	clock       system.Clock
}

func (s assembledSystem) FileSystem() system.FileSystem   { return s.fileSystem }
func (s assembledSystem) Network() system.Network         { return s.network }
func (s assembledSystem) Process() system.ProcessManager  { return s.process }
func (s assembledSystem) Environment() system.Environment { return s.environment }
func (s assembledSystem) Clock() system.Clock             { return s.clock }

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

type unsupportedEnvironment struct{}

func (unsupportedEnvironment) Lookup(context.Context, string) (string, bool, error) {
	return "", false, errors.ErrUnsupported
}

type unsupportedProcess struct{}

func (unsupportedProcess) Run(context.Context, system.ProcessRequest) (system.ProcessResult, error) {
	return system.ProcessResult{}, errors.ErrUnsupported
}
func (unsupportedProcess) Start(context.Context, system.ProcessRequest) (system.ProcessHandle, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedProcess) Ensure(context.Context, system.ProcessRequest) (system.ProcessHandle, bool, error) {
	return nil, false, errors.ErrUnsupported
}
func (unsupportedProcess) List(context.Context) ([]system.ProcessInfo, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedProcess) Status(context.Context, string) (system.ProcessInfo, error) {
	return system.ProcessInfo{}, errors.ErrUnsupported
}
func (unsupportedProcess) Output(context.Context, string) (system.ProcessOutput, error) {
	return system.ProcessOutput{}, errors.ErrUnsupported
}
func (unsupportedProcess) Wait(context.Context, string, time.Duration) (system.ProcessResult, error) {
	return system.ProcessResult{}, errors.ErrUnsupported
}
func (unsupportedProcess) Stop(context.Context, string) error {
	return errors.ErrUnsupported
}
func (unsupportedProcess) Kill(context.Context, string) error {
	return errors.ErrUnsupported
}

type unsupportedFileSystem struct{}

func (unsupportedFileSystem) Open(string) (fs.File, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedFileSystem) Stat(string) (fs.FileInfo, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedFileSystem) ReadFile(string) ([]byte, error) {
	return nil, errors.ErrUnsupported
}
func (unsupportedFileSystem) WriteFile(context.Context, string, []byte, system.WriteFileOptions) error {
	return errors.ErrUnsupported
}
func (unsupportedFileSystem) WriteTempFile(context.Context, string, string, []byte, system.WriteTempFileOptions) (string, error) {
	return "", errors.ErrUnsupported
}
func (unsupportedFileSystem) MkdirAll(context.Context, string, system.MkdirOptions) error {
	return errors.ErrUnsupported
}
func (unsupportedFileSystem) Remove(context.Context, string) error {
	return errors.ErrUnsupported
}
func (unsupportedFileSystem) Rename(context.Context, string, string, system.RenameOptions) error {
	return errors.ErrUnsupported
}
