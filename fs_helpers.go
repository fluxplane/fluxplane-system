package system

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/fluxplane/fluxplane-system/internal/pathpattern"
)

// WalkOptions bounds filesystem tree traversal.
type WalkOptions struct {
	Depth         int
	ShowHidden    bool
	MaxEntries    int
	FilesOnly     bool
	SkipDirs      []string
	FilterPattern string
}

// WalkEntry describes one path discovered by Walk.
type WalkEntry struct {
	Path    string      `json:"path"`
	Name    string      `json:"name"`
	Kind    string      `json:"kind"`
	Size    int64       `json:"size,omitempty"`
	Mode    string      `json:"mode,omitempty"`
	ModTime time.Time   `json:"mod_time,omitempty"`
	Level   int         `json:"level,omitempty"`
	Info    fs.FileInfo `json:"-"`
}

// GlobOptions bounds glob matching.
type GlobOptions struct {
	Base       string
	MaxResults int
	MaxScanned int
	SkipDirs   []string
}

// ReadFileLimit reads at most maxBytes from name.
func ReadFileLimit(ctx context.Context, fsys fs.FS, name string, maxBytes int64) ([]byte, bool, error) {
	if err := contextErr(ctx); err != nil {
		return nil, false, err
	}
	cleanName, err := cleanFSName(name)
	if err != nil {
		return nil, false, err
	}
	file, err := fsys.Open(cleanName)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	if maxBytes <= 0 {
		data, err := io.ReadAll(file)
		return data, false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}
	return data, truncated, contextErr(ctx)
}

// ReadFileLines reads a bounded 1-indexed line window.
func ReadFileLines(ctx context.Context, fsys fs.FS, name string, start, end int, maxBytes int64) ([]byte, int, bool, error) {
	if err := contextErr(ctx); err != nil {
		return nil, 0, false, err
	}
	cleanName, err := cleanFSName(name)
	if err != nil {
		return nil, 0, false, err
	}
	file, err := fsys.Open(cleanName)
	if err != nil {
		return nil, 0, false, err
	}
	defer func() { _ = file.Close() }()
	if start <= 0 {
		start = 1
	}
	if end > 0 && end < start {
		end = start
	}
	var out bytes.Buffer
	var written int64
	reader := bufio.NewReader(file)
	lineNo := 1
	for {
		if err := contextErr(ctx); err != nil {
			return nil, 0, false, err
		}
		line, err := reader.ReadString('\n')
		if lineNo >= start && (end <= 0 || lineNo <= end) {
			if maxBytes > 0 {
				remaining := maxBytes - written
				if remaining <= 0 {
					return out.Bytes(), start, true, nil
				}
				if int64(len(line)) > remaining {
					out.WriteString(line[:int(remaining)])
					return out.Bytes(), start, true, nil
				}
			}
			out.WriteString(line)
			written += int64(len(line))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, false, err
		}
		if end > 0 && lineNo >= end {
			break
		}
		lineNo++
	}
	return out.Bytes(), start, false, nil
}

// Walk returns a bounded tree traversal rooted at root.
func Walk(ctx context.Context, fsys fs.FS, root string, opts WalkOptions) ([]WalkEntry, bool, error) {
	root, err := cleanFSName(root)
	if err != nil {
		return nil, false, err
	}
	depth := opts.Depth
	if depth <= 0 {
		depth = 3
	}
	if depth > 50 {
		depth = 50
	}
	limit := opts.MaxEntries
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	skipDirs := skipDirSet(opts.SkipDirs)
	var entries []WalkEntry
	truncated := false
	err = fs.WalkDir(fsys, root, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := contextErr(ctx); err != nil {
			return err
		}
		if current == root {
			return nil
		}
		relToRoot := current
		if root != "." {
			relToRoot = strings.TrimPrefix(current, root+"/")
		}
		level := strings.Count(relToRoot, "/") + 1
		if level > depth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !opts.ShowHidden && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return fs.SkipDir
		}
		if opts.FilesOnly && d.IsDir() {
			return nil
		}
		if opts.FilterPattern != "" && !matchFilterPattern(opts.FilterPattern, relToRoot, d.IsDir()) {
			return nil
		}
		if len(entries) >= limit {
			truncated = true
			return fs.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		kind := "file"
		if d.IsDir() {
			kind = "dir"
		} else if d.Type()&fs.ModeSymlink != 0 {
			kind = "symlink"
		}
		entries = append(entries, WalkEntry{
			Path: current, Name: d.Name(), Kind: kind, Size: info.Size(),
			Mode: info.Mode().String(), ModTime: info.ModTime(), Level: level, Info: info,
		})
		return nil
	})
	return entries, truncated, err
}

// Glob returns filesystem paths matching a slash-style relative pattern.
func Glob(ctx context.Context, fsys fs.FS, pattern string, opts GlobOptions) ([]string, bool, error) {
	compiled, err := pathpattern.Compile(pattern)
	if err != nil {
		return nil, false, err
	}
	base, err := cleanFSName(opts.Base)
	if err != nil {
		return nil, false, err
	}
	limit := opts.MaxResults
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	scanLimit := opts.MaxScanned
	if scanLimit <= 0 || scanLimit > 100000 {
		scanLimit = 10000
	}
	entries, truncated, err := Walk(ctx, fsys, base, WalkOptions{Depth: 50, ShowHidden: true, MaxEntries: scanLimit, SkipDirs: opts.SkipDirs})
	if err != nil {
		return nil, false, err
	}
	matches := make([]string, 0)
	resultsTruncated := false
	for _, entry := range entries {
		matchRel := entry.Path
		if base != "." && strings.HasPrefix(matchRel, base+"/") {
			matchRel = strings.TrimPrefix(matchRel, base+"/")
		}
		if compiled.Match(matchRel) || compiled.Match(entry.Path) {
			if len(matches) < limit {
				matches = append(matches, entry.Path)
			} else {
				resultsTruncated = true
			}
		}
	}
	return matches, truncated || resultsTruncated, nil
}

func cleanFSName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return ".", nil
	}
	name = path.Clean(name)
	if name == "." {
		return ".", nil
	}
	if path.IsAbs(name) || !fs.ValidPath(name) {
		return "", fmt.Errorf("invalid filesystem name %q", name)
	}
	return name, nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func skipDirSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func matchFilterPattern(pattern, relPath string, isDir bool) bool {
	matched, err := path.Match(pattern, relPath)
	if err != nil {
		return false
	}
	if matched {
		return true
	}
	base := path.Base(relPath)
	if base != relPath {
		if m, _ := path.Match(pattern, base); m {
			return true
		}
	}
	if isDir {
		if m, _ := path.Match(pattern, relPath+"/*"); m {
			return m
		}
	}
	return false
}
