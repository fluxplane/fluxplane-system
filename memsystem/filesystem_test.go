package memsystem

import (
	"context"
	"io/fs"
	"testing"
	"time"

	system "github.com/fluxplane/fluxplane-system"
)

func TestFileSystemReadWriteRenameRemove(t *testing.T) {
	fsys := NewFileSystem()
	if err := fsys.WriteFile(context.Background(), "dir/file.txt", []byte("hello"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(fsys, "dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q", data)
	}
	if err := fsys.Rename(context.Background(), "dir/file.txt", "dir/renamed.txt", system.RenameOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Remove(context.Background(), "dir/renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(fsys, "dir/renamed.txt"); err == nil {
		t.Fatal("expected renamed file to be removed")
	}
}

func TestFileSystemDirectoryAndOverwriteBehavior(t *testing.T) {
	fsys := NewFileSystem()
	if err := fsys.MkdirAll(context.Background(), "dir/sub", system.MkdirOptions{}); err != nil {
		t.Fatal(err)
	}
	if entries, err := fs.ReadDir(fsys, "dir"); err != nil {
		t.Fatal(err)
	} else if len(entries) != 1 || entries[0].Name() != "sub" || !entries[0].IsDir() {
		t.Fatalf("entries = %#v", entries)
	}
	if err := fsys.WriteFile(context.Background(), "dir/sub/file.txt", []byte("one"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(context.Background(), "dir/sub/file.txt", []byte("two"), system.WriteFileOptions{}); err == nil {
		t.Fatal("expected overwrite=false to fail")
	}
	if err := fsys.WriteFile(context.Background(), "dir/sub/file.txt", []byte("two"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(fsys, "dir/sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "two" {
		t.Fatalf("data = %q", data)
	}
}

func TestFileSystemWriteTempFile(t *testing.T) {
	fsys := NewFileSystem()
	first, err := fsys.WriteTempFile(context.Background(), "artifacts", "shot-*.png", []byte("one"), system.WriteTempFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fsys.WriteTempFile(context.Background(), "artifacts", "shot-*.png", []byte("two"), system.WriteTempFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("temp names were not unique: %q", first)
	}
	if first != "artifacts/shot-000001.png" || second != "artifacts/shot-000002.png" {
		t.Fatalf("temp names = %q, %q", first, second)
	}
	data, err := fs.ReadFile(fsys, second)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "two" {
		t.Fatalf("temp data = %q", data)
	}
}

func TestClockSleepAdvances(t *testing.T) {
	clock := NewClock(testTime())
	start := clock.Now()
	if err := clock.Sleep(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if !clock.Now().After(start) {
		t.Fatalf("clock did not advance")
	}
}

func testTime() time.Time {
	return time.Unix(100, 0).UTC()
}
