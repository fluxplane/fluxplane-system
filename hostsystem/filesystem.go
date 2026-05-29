package hostsystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/fluxplane/fluxplane-system"
)

// FileSystem is an OS-backed filesystem rooted at Root for relative fs.FS names.
type FileSystem struct {
	root string
}

// NewFileSystem returns an OS-backed filesystem rooted at root.
func NewFileSystem(root string) *FileSystem {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return &FileSystem{root: root}
}

// Root returns the base directory used for relative names.
func (f *FileSystem) Root() string {
	if f == nil {
		return ""
	}
	return f.root
}

func (f *FileSystem) Open(name string) (fs.File, error) {
	path, err := f.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (f *FileSystem) Stat(name string) (fs.FileInfo, error) {
	path, err := f.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(path)
}

func (f *FileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	path, err := f.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(path)
}

func (f *FileSystem) ReadFile(name string) ([]byte, error) {
	path, err := f.resolveExisting(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (f *FileSystem) WriteFile(ctx context.Context, name string, data []byte, opts system.WriteFileOptions) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	path, err := f.resolveCreate(name)
	if err != nil {
		return err
	}
	if !opts.Overwrite {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("path already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !opts.Overwrite {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, defaultPerm(opts.Perm, 0o644))
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func (f *FileSystem) MkdirAll(ctx context.Context, name string, opts system.MkdirOptions) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	path, err := f.resolveCreate(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(path, defaultPerm(opts.Perm, 0o755))
}

func (f *FileSystem) Remove(ctx context.Context, name string) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	path, err := f.resolveExisting(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (f *FileSystem) Rename(ctx context.Context, oldName, newName string, opts system.RenameOptions) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	oldPath, err := f.resolveExisting(oldName)
	if err != nil {
		return err
	}
	newPath, err := f.resolveCreate(newName)
	if err != nil {
		return err
	}
	if !opts.Overwrite {
		if _, err := os.Lstat(newPath); err == nil {
			return fmt.Errorf("path already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(oldPath, newPath)
}

func (f *FileSystem) path(name string) (string, error) {
	if f == nil {
		return "", fmt.Errorf("filesystem is nil")
	}
	name = strings.TrimSpace(filepath.ToSlash(name))
	if name == "" {
		name = "."
	}
	if path.IsAbs(name) || filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("absolute filesystem names are not supported")
	}
	if name != "." && !fs.ValidPath(name) {
		return "", fmt.Errorf("invalid filesystem name %q", name)
	}
	return filepath.Join(f.root, filepath.FromSlash(name)), nil
}

func (f *FileSystem) resolveExisting(name string) (string, error) {
	candidate, err := f.path(name)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if err := f.pathWithin(real); err != nil {
		return "", err
	}
	return real, nil
}

func (f *FileSystem) resolveCreate(name string) (string, error) {
	candidate, err := f.path(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(candidate); err == nil {
		real, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		if err := f.pathWithin(real); err != nil {
			return "", err
		}
		return real, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	missing := []string{filepath.Base(candidate)}
	parent := filepath.Dir(candidate)
	for {
		if _, err := os.Lstat(parent); err == nil {
			realParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				realParent = filepath.Join(realParent, missing[i])
			}
			if err := f.pathWithin(realParent); err != nil {
				return "", err
			}
			return realParent, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", fmt.Errorf("path escapes filesystem root")
		}
		missing = append(missing, filepath.Base(parent))
		parent = next
	}
}

func (f *FileSystem) pathWithin(candidate string) error {
	root := f.root
	if realRoot, err := filepath.EvalSymlinks(root); err == nil {
		root = realRoot
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if rel == "." || rel == "" {
		return nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes filesystem root")
	}
	return nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

var _ system.FileSystem = (*FileSystem)(nil)
