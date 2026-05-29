package system

import (
	"context"
	"io/fs"
	"time"
)

// System groups primitive host capabilities.
type System interface {
	FileSystem() FileSystem
	Network() Network
	Process() ProcessManager
	Environment() Environment
	Clock() Clock
}

// FileSystem is an io/fs-compatible filesystem with explicit write methods.
type FileSystem interface {
	fs.FS
	fs.StatFS
	fs.ReadDirFS
	fs.ReadFileFS
	Writer
	DirMaker
	Remover
	Renamer
}

// Writer writes complete files.
type Writer interface {
	WriteFile(context.Context, string, []byte, WriteFileOptions) error
}

// DirMaker creates directories and parents.
type DirMaker interface {
	MkdirAll(context.Context, string, MkdirOptions) error
}

// Remover removes one file or directory.
type Remover interface {
	Remove(context.Context, string) error
}

// Renamer renames one filesystem entry.
type Renamer interface {
	Rename(context.Context, string, string, RenameOptions) error
}

// WriteFileOptions controls file writes.
type WriteFileOptions struct {
	Perm      fs.FileMode
	Overwrite bool
}

// MkdirOptions controls directory creation.
type MkdirOptions struct {
	Perm fs.FileMode
}

// RenameOptions controls rename behavior.
type RenameOptions struct {
	Overwrite bool
}

// Environment is a host environment lookup boundary.
type Environment interface {
	Lookup(context.Context, string) (string, bool, error)
}

// ExecutableResolver resolves executables without exposing PATH directly.
type ExecutableResolver interface {
	ResolveExecutable(context.Context, string) (string, bool, error)
}

// Clock provides deterministic time access.
type Clock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}
