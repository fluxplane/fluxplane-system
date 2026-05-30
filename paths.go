package system

import (
	"context"
	"io/fs"
	"strings"
)

// ResolvedPath is a canonical path resolved through a bounded path capability.
type ResolvedPath struct {
	Input string `json:"input,omitempty"`
	Abs   string `json:"abs"`
	Rel   string `json:"rel"`
}

// PathResolver resolves caller-provided paths to canonical bounded paths.
type PathResolver interface {
	ResolveExisting(context.Context, string) (ResolvedPath, error)
	ResolveCreate(context.Context, string) (ResolvedPath, error)
}

// BoundedFileReader reads files while returning their resolved bounded path.
type BoundedFileReader interface {
	ReadFile(context.Context, string, int64) ([]byte, bool, ResolvedPath, error)
}

// BoundedFileLineReader reads bounded line ranges while returning their resolved bounded path.
type BoundedFileLineReader interface {
	ReadFileLines(context.Context, string, int, int, int64) ([]byte, int, bool, ResolvedPath, error)
}

// BoundedFileWriter writes files while returning their resolved bounded path.
type BoundedFileWriter interface {
	WriteFile(context.Context, string, []byte, fs.FileMode, bool) (ResolvedPath, error)
}

// BoundedFileCopier copies one file while returning source and destination paths.
type BoundedFileCopier interface {
	CopyFile(context.Context, string, string, bool) (ResolvedPath, ResolvedPath, int64, error)
}

// BoundedFileMover moves one file while returning source and destination paths.
type BoundedFileMover interface {
	MoveFile(context.Context, string, string, bool) (ResolvedPath, ResolvedPath, int64, error)
}

// BoundedDirMaker creates directories while returning their resolved bounded path.
type BoundedDirMaker interface {
	MkdirAll(context.Context, string, fs.FileMode) (ResolvedPath, error)
}

// BoundedRemover removes paths while returning their resolved bounded path.
type BoundedRemover interface {
	Remove(context.Context, string) (ResolvedPath, error)
}

// BoundedStatFS stats paths while returning their resolved bounded path.
type BoundedStatFS interface {
	Stat(context.Context, string) (fs.FileInfo, ResolvedPath, error)
}

// BoundedReadDirFS lists directories while returning their resolved bounded path.
type BoundedReadDirFS interface {
	ReadDir(context.Context, string) ([]fs.DirEntry, ResolvedPath, error)
}

// ScratchDir is an isolated temporary directory owned by a bounded path capability.
type ScratchDir interface {
	Root() string
	WriteFile(context.Context, string, []byte, fs.FileMode) (ResolvedPath, error)
	RemoveAll(context.Context) error
}

// ScratchProvider creates isolated temporary directories for runtime-owned work.
type ScratchProvider interface {
	CreateScratch(context.Context, string) (ScratchDir, error)
}

// PathName returns the relative filesystem name for resolved.
func PathName(resolved ResolvedPath) string {
	if strings.TrimSpace(resolved.Rel) == "" {
		return "."
	}
	return resolved.Rel
}
