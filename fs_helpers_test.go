package system_test

import (
	"context"
	"testing"

	system "github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/memsystem"
)

func TestReadFileLimitTruncates(t *testing.T) {
	fsys := memsystem.NewFileSystem()
	if err := fsys.WriteFile(context.Background(), "README.md", []byte("abcdef"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	data, truncated, err := system.ReadFileLimit(context.Background(), fsys, "README.md", 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abc" || !truncated {
		t.Fatalf("ReadFileLimit = %q truncated=%v, want abc true", data, truncated)
	}
}

func TestReadFileLines(t *testing.T) {
	fsys := memsystem.NewFileSystem()
	if err := fsys.WriteFile(context.Background(), "notes.txt", []byte("one\ntwo\nthree\n"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	data, start, truncated, err := system.ReadFileLines(context.Background(), fsys, "notes.txt", 2, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if start != 2 || truncated || string(data) != "two\nthree\n" {
		t.Fatalf("ReadFileLines = start=%d truncated=%v data=%q", start, truncated, data)
	}
}

func TestWalkAndGlob(t *testing.T) {
	fsys := memsystem.NewFileSystem()
	for _, name := range []string{"a/main.go", "a/readme.md", "b/test.go"} {
		if err := fsys.WriteFile(context.Background(), name, []byte("x"), system.WriteFileOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	entries, truncated, err := system.Walk(context.Background(), fsys, ".", system.WalkOptions{Depth: 3, MaxEntries: 10, FilesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(entries) != 3 {
		t.Fatalf("Walk entries=%d truncated=%v, want 3 false", len(entries), truncated)
	}
	matches, truncated, err := system.Glob(context.Background(), fsys, "**/*.go", system.GlobOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(matches) != 2 {
		t.Fatalf("Glob matches=%v truncated=%v, want 2 false", matches, truncated)
	}
}

func TestHelpersRejectInvalidFSNames(t *testing.T) {
	fsys := memsystem.NewFileSystem()
	for _, name := range []string{"../escape", "/absolute", "bad\x00name"} {
		if _, _, err := system.ReadFileLimit(context.Background(), fsys, name, 10); err == nil {
			t.Fatalf("ReadFileLimit(%q) succeeded, want error", name)
		}
	}
}

func TestHelpersNormalizeBackslashes(t *testing.T) {
	fsys := memsystem.NewFileSystem()
	if err := fsys.WriteFile(context.Background(), "dir/file.txt", []byte("ok"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _, err := system.ReadFileLimit(context.Background(), fsys, `dir\file.txt`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("data = %q", data)
	}
}

func TestWalkAndGlobTruncate(t *testing.T) {
	fsys := memsystem.NewFileSystem()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := fsys.WriteFile(context.Background(), name, []byte("x"), system.WriteFileOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	_, truncated, err := system.Walk(context.Background(), fsys, ".", system.WalkOptions{MaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("Walk truncated=false, want true")
	}
	matches, truncated, err := system.Glob(context.Background(), fsys, "*.go", system.GlobOptions{MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || !truncated {
		t.Fatalf("Glob matches=%v truncated=%v, want one true", matches, truncated)
	}
}
