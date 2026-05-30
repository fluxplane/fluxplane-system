package mountfs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"

	system "github.com/fluxplane/fluxplane-system"
)

// Access describes the operations allowed through a mounted root.
type Access string

const (
	// ReadOnly allows reads and denies writes, mkdir, remove, and rename.
	ReadOnly Access = "read_only"
	// ReadWrite allows both reads and writes.
	ReadWrite Access = "read_write"
)

// Spec describes the roots exposed by a mounted filesystem.
type Spec struct {
	Roots []Root
}

// Root maps one visible root to a path in the underlying filesystem.
//
// The empty name is the primary root. Non-empty names are addressed with the
// reserved @name/path syntax. Root paths are slash-style io/fs names relative
// to the wrapped filesystem.
type Root struct {
	Name    string
	Path    string
	Access  Access
	Scratch bool
}

// RootInfo describes a configured root after validation and normalization.
type RootInfo struct {
	Name    string
	Path    string
	Access  Access
	Scratch bool
}

// Resolved describes how a visible name maps to the wrapped filesystem.
type Resolved struct {
	Root RootInfo
	Name string
	Path string
}

// FileSystem wraps another system.FileSystem with mounted roots.
type FileSystem struct {
	base  system.FileSystem
	roots map[string]RootInfo
	order []RootInfo
}

// New returns a mounted filesystem over base.
func New(base system.FileSystem, spec Spec) (*FileSystem, error) {
	if base == nil {
		return nil, fmt.Errorf("base filesystem is nil")
	}
	if len(spec.Roots) == 0 {
		return nil, fmt.Errorf("at least one root is required")
	}
	out := &FileSystem{
		base:  base,
		roots: make(map[string]RootInfo, len(spec.Roots)),
		order: make([]RootInfo, 0, len(spec.Roots)),
	}
	for _, root := range spec.Roots {
		info, err := normalizeRoot(root)
		if err != nil {
			return nil, err
		}
		if _, ok := out.roots[info.Name]; ok {
			if info.Name == "" {
				return nil, fmt.Errorf("duplicate primary root")
			}
			return nil, fmt.Errorf("duplicate root %q", info.Name)
		}
		out.roots[info.Name] = info
		out.order = append(out.order, info)
	}
	return out, nil
}

// Base returns the wrapped filesystem.
func (f *FileSystem) Base() system.FileSystem {
	if f == nil {
		return nil
	}
	return f.base
}

// Roots returns configured roots in declaration order.
func (f *FileSystem) Roots() []RootInfo {
	if f == nil {
		return nil
	}
	out := make([]RootInfo, len(f.order))
	copy(out, f.order)
	return out
}

// Resolve maps a visible name to the wrapped filesystem namespace.
func (f *FileSystem) Resolve(name string) (Resolved, error) {
	if f == nil {
		return Resolved{}, fmt.Errorf("filesystem is nil")
	}
	if isVirtualRoot(name) {
		return Resolved{Name: ".", Path: "."}, nil
	}
	root, rel, err := f.resolve(name)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Root: root,
		Name: cleanVisibleName(root.Name, rel),
		Path: join(root.Path, rel),
	}, nil
}

func (f *FileSystem) Open(name string) (fs.File, error) {
	if isVirtualRoot(name) {
		entries, err := f.virtualRootEntries()
		if err != nil {
			return nil, err
		}
		return &virtualDirFile{entries: entries}, nil
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return nil, err
	}
	return f.base.Open(resolved.Path)
}

func (f *FileSystem) Stat(name string) (fs.FileInfo, error) {
	if isVirtualRoot(name) {
		return virtualFileInfo{name: ".", mode: fs.ModeDir | 0o555}, nil
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return nil, err
	}
	return f.base.Stat(resolved.Path)
}

func (f *FileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if isVirtualRoot(name) {
		return f.virtualRootEntries()
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return nil, err
	}
	return f.base.ReadDir(resolved.Path)
}

func (f *FileSystem) ReadFile(name string) ([]byte, error) {
	if isVirtualRoot(name) {
		return nil, fmt.Errorf("path is a directory")
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return nil, err
	}
	return f.base.ReadFile(resolved.Path)
}

func (f *FileSystem) WriteFile(ctx context.Context, name string, data []byte, opts system.WriteFileOptions) error {
	if isVirtualRoot(name) {
		return fmt.Errorf("path is a directory")
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return err
	}
	if !canWrite(resolved.Root.Access) {
		return fmt.Errorf("root %q is read-only", rootLabel(resolved.Root.Name))
	}
	return f.base.WriteFile(ctx, resolved.Path, data, opts)
}

func (f *FileSystem) WriteTempFile(ctx context.Context, dir, pattern string, data []byte, opts system.WriteTempFileOptions) (string, error) {
	if isVirtualRoot(dir) {
		return "", fmt.Errorf("cannot write temp file at mount filesystem root")
	}
	resolved, err := f.Resolve(dir)
	if err != nil {
		return "", err
	}
	if !canWrite(resolved.Root.Access) {
		return "", fmt.Errorf("root %q is read-only", rootLabel(resolved.Root.Name))
	}
	created, err := f.base.WriteTempFile(ctx, resolved.Path, pattern, data, opts)
	if err != nil {
		return "", err
	}
	rel, err := relativeUnder(resolved.Root.Path, created)
	if err != nil {
		return "", err
	}
	return cleanVisibleName(resolved.Root.Name, rel), nil
}

func (f *FileSystem) MkdirAll(ctx context.Context, name string, opts system.MkdirOptions) error {
	if isVirtualRoot(name) {
		return nil
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return err
	}
	if !canWrite(resolved.Root.Access) {
		return fmt.Errorf("root %q is read-only", rootLabel(resolved.Root.Name))
	}
	return f.base.MkdirAll(ctx, resolved.Path, opts)
}

func (f *FileSystem) Remove(ctx context.Context, name string) error {
	if isVirtualRoot(name) {
		return fmt.Errorf("cannot remove mount filesystem root")
	}
	resolved, err := f.Resolve(name)
	if err != nil {
		return err
	}
	if !canWrite(resolved.Root.Access) {
		return fmt.Errorf("root %q is read-only", rootLabel(resolved.Root.Name))
	}
	return f.base.Remove(ctx, resolved.Path)
}

func (f *FileSystem) Rename(ctx context.Context, oldName, newName string, opts system.RenameOptions) error {
	if isVirtualRoot(oldName) || isVirtualRoot(newName) {
		return fmt.Errorf("cannot rename mount filesystem root")
	}
	oldResolved, err := f.Resolve(oldName)
	if err != nil {
		return err
	}
	newResolved, err := f.Resolve(newName)
	if err != nil {
		return err
	}
	if !canWrite(oldResolved.Root.Access) {
		return fmt.Errorf("source root %q is read-only", rootLabel(oldResolved.Root.Name))
	}
	if !canWrite(newResolved.Root.Access) {
		return fmt.Errorf("destination root %q is read-only", rootLabel(newResolved.Root.Name))
	}
	return f.base.Rename(ctx, oldResolved.Path, newResolved.Path, opts)
}

func (f *FileSystem) resolve(name string) (RootInfo, string, error) {
	name = cleanInput(name)
	if name == "" || name == "." {
		root, ok := f.roots[""]
		if !ok {
			return RootInfo{}, "", fmt.Errorf("primary root is not configured")
		}
		return root, "", nil
	}
	if strings.HasPrefix(name, "@") {
		head, tail, ok := strings.Cut(name, "/")
		rootName := strings.TrimPrefix(head, "@")
		if rootName == "" {
			return RootInfo{}, "", fmt.Errorf("missing root name")
		}
		root, exists := f.roots[rootName]
		if !exists {
			return RootInfo{}, "", fmt.Errorf("unknown root %q", rootName)
		}
		if !ok {
			return root, "", nil
		}
		rel, err := cleanRelative(tail)
		if err != nil {
			return RootInfo{}, "", err
		}
		return root, rel, nil
	}
	root, ok := f.roots[""]
	if !ok {
		return RootInfo{}, "", fmt.Errorf("primary root is not configured")
	}
	rel, err := cleanRelative(name)
	if err != nil {
		return RootInfo{}, "", err
	}
	return root, rel, nil
}

func (f *FileSystem) virtualRootEntries() ([]fs.DirEntry, error) {
	entries := map[string]fs.DirEntry{}
	if root, ok := f.roots[""]; ok {
		baseEntries, err := f.base.ReadDir(root.Path)
		if err != nil {
			return nil, err
		}
		for _, entry := range baseEntries {
			entries[entry.Name()] = entry
		}
	}
	for _, root := range f.order {
		if root.Name == "" {
			continue
		}
		entries["@"+root.Name] = virtualDirEntry{name: "@" + root.Name}
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		out = append(out, entries[name])
	}
	return out, nil
}

func normalizeRoot(root Root) (RootInfo, error) {
	name := strings.TrimSpace(root.Name)
	if name != "" && !validRootName(name) {
		return RootInfo{}, fmt.Errorf("invalid root name %q", root.Name)
	}
	rootPath, err := cleanRelative(root.Path)
	if err != nil {
		return RootInfo{}, fmt.Errorf("invalid root path %q: %w", root.Path, err)
	}
	access, err := normalizeAccess(root.Access)
	if err != nil {
		return RootInfo{}, err
	}
	if rootPath == "" {
		rootPath = "."
	}
	return RootInfo{Name: name, Path: rootPath, Access: access, Scratch: root.Scratch}, nil
}

func normalizeAccess(access Access) (Access, error) {
	switch access {
	case "":
		return ReadOnly, nil
	case ReadOnly, ReadWrite:
		return access, nil
	default:
		return "", fmt.Errorf("invalid root access %q", access)
	}
}

func canWrite(access Access) bool {
	return access == ReadWrite
}

func cleanRelative(name string) (string, error) {
	name = cleanInput(name)
	if name == "" || name == "." {
		return "", nil
	}
	if path.IsAbs(name) || !fs.ValidPath(name) {
		return "", fmt.Errorf("invalid filesystem name %q", name)
	}
	return path.Clean(name), nil
}

func cleanInput(name string) string {
	return strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
}

func isVirtualRoot(name string) bool {
	name = cleanInput(name)
	return name == "" || name == "."
}

func validRootName(name string) bool {
	if name == "." || name == ".." || strings.ContainsAny(name, "@/\\") {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func cleanVisibleName(rootName, rel string) string {
	if rootName == "" {
		if rel == "" {
			return "."
		}
		return rel
	}
	if rel == "" {
		return "@" + rootName
	}
	return "@" + rootName + "/" + rel
}

func join(rootPath, rel string) string {
	if rel == "" {
		return rootPath
	}
	if rootPath == "." {
		return rel
	}
	return path.Join(rootPath, rel)
}

func relativeUnder(rootPath, name string) (string, error) {
	rootPath = path.Clean(rootPath)
	name = path.Clean(name)
	if rootPath == "." {
		return name, nil
	}
	if name == rootPath {
		return "", fmt.Errorf("path is a directory")
	}
	prefix := strings.TrimSuffix(rootPath, "/") + "/"
	if !strings.HasPrefix(name, prefix) {
		return "", fmt.Errorf("path escapes mounted root")
	}
	return strings.TrimPrefix(name, prefix), nil
}

func rootLabel(name string) string {
	if name == "" {
		return "."
	}
	return "@" + name
}

type virtualDirFile struct {
	entries []fs.DirEntry
	offset  int
}

func (f *virtualDirFile) Stat() (fs.FileInfo, error) {
	return virtualFileInfo{name: ".", mode: fs.ModeDir | 0o555}, nil
}

func (f *virtualDirFile) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (f *virtualDirFile) Close() error {
	return nil
}

func (f *virtualDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if f.offset >= len(f.entries) && n > 0 {
		return nil, io.EOF
	}
	if n <= 0 {
		out := append([]fs.DirEntry(nil), f.entries[f.offset:]...)
		f.offset = len(f.entries)
		return out, nil
	}
	end := f.offset + n
	if end > len(f.entries) {
		end = len(f.entries)
	}
	out := append([]fs.DirEntry(nil), f.entries[f.offset:end]...)
	f.offset = end
	return out, nil
}

type virtualDirEntry struct {
	name string
}

func (e virtualDirEntry) Name() string {
	return e.name
}

func (e virtualDirEntry) IsDir() bool {
	return true
}

func (e virtualDirEntry) Type() fs.FileMode {
	return fs.ModeDir
}

func (e virtualDirEntry) Info() (fs.FileInfo, error) {
	return virtualFileInfo{name: e.name, mode: fs.ModeDir | 0o555}, nil
}

type virtualFileInfo struct {
	name string
	mode fs.FileMode
}

func (i virtualFileInfo) Name() string {
	return i.name
}

func (i virtualFileInfo) Size() int64 {
	return 0
}

func (i virtualFileInfo) Mode() fs.FileMode {
	return i.mode
}

func (i virtualFileInfo) ModTime() time.Time {
	return time.Time{}
}

func (i virtualFileInfo) IsDir() bool {
	return i.mode.IsDir()
}

func (i virtualFileInfo) Sys() any {
	return nil
}

var _ fs.ReadDirFile = (*virtualDirFile)(nil)

var _ system.FileSystem = (*FileSystem)(nil)
