package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyRegularFile copies one regular file from src to dst and returns bytes written.
func CopyRegularFile(src, dst string, overwrite bool) (int64, error) {
	info, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("source path is a directory")
	}
	if filepath.Clean(src) == filepath.Clean(dst) {
		return info.Size(), nil
	}
	if !overwrite {
		if _, err := os.Lstat(dst); err == nil {
			return 0, fmt.Errorf("path already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return 0, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, err
	}
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer func() { _ = in.Close() }()
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags |= os.O_EXCL
	}
	out, err := os.OpenFile(dst, flags, info.Mode().Perm())
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return written, copyErr
	}
	if closeErr != nil {
		return written, closeErr
	}
	return written, nil
}

// ScratchPath is a path resolved under a HostScratchDir.
type ScratchPath struct {
	Input string
	Abs   string
	Rel   string
}

// HostScratchDir is an isolated temporary directory with bounded path writes.
type HostScratchDir struct {
	root string
}

// NewHostScratchDir creates an isolated temporary directory under base.
func NewHostScratchDir(base, prefix string) (*HostScratchDir, error) {
	if strings.TrimSpace(prefix) == "" {
		prefix = "fluxplane-*"
	}
	dir, err := os.MkdirTemp(base, prefix)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &HostScratchDir{root: real}, nil
}

// Root returns the scratch directory root.
func (s *HostScratchDir) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

// WriteFile writes a file below the scratch root, creating parent directories.
func (s *HostScratchDir) WriteFile(ctx context.Context, raw string, data []byte, mode os.FileMode) (ScratchPath, error) {
	if err := contextErr(ctx); err != nil {
		return ScratchPath{}, err
	}
	resolved, err := s.ResolveCreate(raw)
	if err != nil {
		return ScratchPath{}, err
	}
	if err := os.MkdirAll(filepath.Dir(resolved.Abs), 0755); err != nil {
		return ScratchPath{}, err
	}
	return resolved, os.WriteFile(resolved.Abs, data, mode)
}

// RemoveAll removes the scratch directory tree.
func (s *HostScratchDir) RemoveAll(context.Context) error {
	if s == nil || s.root == "" {
		return nil
	}
	return os.RemoveAll(s.root)
}

// ResolveCreate resolves a path whose final component may not exist under the scratch root.
func (s *HostScratchDir) ResolveCreate(raw string) (ScratchPath, error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return ScratchPath{}, fmt.Errorf("scratch root is empty")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ScratchPath{}, fmt.Errorf("scratch path is empty")
	}
	clean := filepath.Clean(raw)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return ScratchPath{}, fmt.Errorf("scratch path escapes root")
	}
	abs := filepath.Join(s.root, clean)
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return ScratchPath{}, err
	}
	real := filepath.Join(parent, filepath.Base(abs))
	if err := PathWithin(s.root, real); err != nil {
		return ScratchPath{}, fmt.Errorf("scratch path escapes root")
	}
	rel, err := filepath.Rel(s.root, real)
	if err != nil {
		return ScratchPath{}, err
	}
	return ScratchPath{Input: raw, Abs: real, Rel: filepath.ToSlash(rel)}, nil
}
