package memsystem

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

// FileSystem is a mutable in-memory FileSystem.
type FileSystem struct {
	mu    sync.Mutex
	nodes map[string]*node
	now   time.Time
}

type node struct {
	dir     bool
	data    []byte
	mode    fs.FileMode
	modTime time.Time
}

// NewFileSystem returns an empty memory filesystem.
func NewFileSystem() *FileSystem {
	now := time.Unix(1700000000, 0).UTC()
	return &FileSystem{
		nodes: map[string]*node{"": {dir: true, mode: 0o755 | fs.ModeDir, modTime: now}},
		now:   now,
	}
}

func (f *FileSystem) Open(name string) (fs.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return nil, err
	}
	n, ok := f.nodes[rel]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memFile{name: path.Base(rel), node: cloneNode(n), reader: bytes.NewReader(n.data)}, nil
}

func (f *FileSystem) Stat(name string) (fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return nil, err
	}
	n, ok := f.nodes[rel]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return fileInfo{name: baseName(rel), node: cloneNode(n)}, nil
}

func (f *FileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return nil, err
	}
	n, ok := f.nodes[rel]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if !n.dir {
		return nil, fmt.Errorf("path is not a directory")
	}
	prefix := rel
	if prefix != "" {
		prefix += "/"
	}
	seen := map[string]fs.DirEntry{}
	for candidate, child := range f.nodes {
		if candidate == rel || !strings.HasPrefix(candidate, prefix) {
			continue
		}
		rest := strings.TrimPrefix(candidate, prefix)
		first, _, _ := strings.Cut(rest, "/")
		childPath := prefix + first
		childNode := child
		if existing, ok := f.nodes[childPath]; ok {
			childNode = existing
		}
		seen[first] = dirEntry{name: first, node: cloneNode(childNode)}
	}
	out := make([]fs.DirEntry, 0, len(seen))
	for _, entry := range seen {
		out = append(out, entry)
	}
	return out, nil
}

func (f *FileSystem) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return nil, err
	}
	n, ok := f.nodes[rel]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if n.dir {
		return nil, fmt.Errorf("path is a directory")
	}
	return append([]byte(nil), n.data...), nil
}

func (f *FileSystem) WriteFile(ctx context.Context, name string, data []byte, opts system.WriteFileOptions) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return err
	}
	if rel == "" {
		return fmt.Errorf("path is a directory")
	}
	if _, ok := f.nodes[rel]; ok && !opts.Overwrite {
		return fmt.Errorf("path already exists")
	}
	if err := f.ensureParents(rel); err != nil {
		return err
	}
	mode := opts.Perm
	if mode == 0 {
		mode = 0o644
	}
	f.nodes[rel] = &node{data: append([]byte(nil), data...), mode: mode, modTime: f.tick()}
	return nil
}

func (f *FileSystem) MkdirAll(ctx context.Context, name string, opts system.MkdirOptions) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return err
	}
	mode := opts.Perm
	if mode == 0 {
		mode = 0o755
	}
	for _, dir := range prefixes(rel) {
		f.nodes[dir] = &node{dir: true, mode: mode | fs.ModeDir, modTime: f.tick()}
	}
	return nil
}

func (f *FileSystem) Remove(ctx context.Context, name string) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rel, err := clean(name)
	if err != nil {
		return err
	}
	if rel == "" {
		return fmt.Errorf("cannot remove filesystem root")
	}
	if _, ok := f.nodes[rel]; !ok {
		return fs.ErrNotExist
	}
	prefix := rel + "/"
	for candidate := range f.nodes {
		if strings.HasPrefix(candidate, prefix) {
			return fmt.Errorf("directory is not empty")
		}
	}
	delete(f.nodes, rel)
	return nil
}

func (f *FileSystem) Rename(ctx context.Context, oldName, newName string, opts system.RenameOptions) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	oldRel, err := clean(oldName)
	if err != nil {
		return err
	}
	newRel, err := clean(newName)
	if err != nil {
		return err
	}
	n, ok := f.nodes[oldRel]
	if !ok {
		return fs.ErrNotExist
	}
	if _, ok := f.nodes[newRel]; ok && !opts.Overwrite {
		return fmt.Errorf("path already exists")
	}
	if err := f.ensureParents(newRel); err != nil {
		return err
	}
	f.nodes[newRel] = n
	delete(f.nodes, oldRel)
	return nil
}

func (f *FileSystem) ensureParents(rel string) error {
	dir := path.Dir(rel)
	if dir == "." {
		return nil
	}
	for _, prefix := range prefixes(dir) {
		if existing, ok := f.nodes[prefix]; ok && !existing.dir {
			return fmt.Errorf("parent is not a directory")
		}
		f.nodes[prefix] = &node{dir: true, mode: 0o755 | fs.ModeDir, modTime: f.tick()}
	}
	return nil
}

func (f *FileSystem) tick() time.Time {
	f.now = f.now.Add(time.Millisecond)
	return f.now
}

func clean(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." {
		return "", nil
	}
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("invalid filesystem name %q", name)
	}
	return path.Clean(name), nil
}

func prefixes(rel string) []string {
	if rel == "" || rel == "." {
		return nil
	}
	parts := strings.Split(rel, "/")
	out := make([]string, 0, len(parts))
	for i := range parts {
		out = append(out, strings.Join(parts[:i+1], "/"))
	}
	return out
}

func baseName(rel string) string {
	if rel == "" {
		return "."
	}
	return path.Base(rel)
}

func cloneNode(n *node) *node {
	if n == nil {
		return nil
	}
	out := *n
	out.data = append([]byte(nil), n.data...)
	return &out
}

type memFile struct {
	name   string
	node   *node
	reader *bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) { return fileInfo{name: f.name, node: f.node}, nil }
func (f *memFile) Read(p []byte) (int, error) {
	if f.node.dir {
		return 0, io.EOF
	}
	return f.reader.Read(p)
}
func (f *memFile) Close() error { return nil }

type fileInfo struct {
	name string
	node *node
}

func (i fileInfo) Name() string       { return i.name }
func (i fileInfo) Size() int64        { return int64(len(i.node.data)) }
func (i fileInfo) Mode() fs.FileMode  { return i.node.mode }
func (i fileInfo) ModTime() time.Time { return i.node.modTime }
func (i fileInfo) IsDir() bool        { return i.node.dir }
func (i fileInfo) Sys() any           { return nil }

type dirEntry struct {
	name string
	node *node
}

func (e dirEntry) Name() string               { return e.name }
func (e dirEntry) IsDir() bool                { return e.node.dir }
func (e dirEntry) Type() fs.FileMode          { return e.node.mode.Type() }
func (e dirEntry) Info() (fs.FileInfo, error) { return fileInfo{name: e.name, node: e.node}, nil }

var _ system.FileSystem = (*FileSystem)(nil)
