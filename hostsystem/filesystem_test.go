package hostsystem

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	system "github.com/fluxplane/fluxplane-system"
)

func TestFileSystemReadWrite(t *testing.T) {
	fsys := NewFileSystem(t.TempDir())
	if err := fsys.WriteFile(context.Background(), "a/b.txt", []byte("ok"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(fsys, "a/b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("data = %q", data)
	}
	if err := fsys.WriteFile(context.Background(), "a/b.txt", []byte("again"), system.WriteFileOptions{}); err == nil {
		t.Fatal("expected overwrite=false to fail")
	}
	if err := fsys.WriteFile(context.Background(), "a/b.txt", []byte("again"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
}

func TestFileSystemRenameOverwrite(t *testing.T) {
	fsys := NewFileSystem(t.TempDir())
	if err := fsys.WriteFile(context.Background(), "from.txt", []byte("from"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile(context.Background(), "to.txt", []byte("to"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Rename(context.Background(), "from.txt", "to.txt", system.RenameOptions{}); err == nil {
		t.Fatal("expected overwrite=false rename to fail")
	}
	if err := fsys.Rename(context.Background(), "from.txt", "to.txt", system.RenameOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(fsys, "to.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "from" {
		t.Fatalf("renamed data = %q", data)
	}
}

func TestFileSystemRejectsInvalidNames(t *testing.T) {
	fsys := NewFileSystem(t.TempDir())
	if _, err := fsys.Open("../escape"); err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestFileSystemRejectsSymlinkReadEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	fsys := NewFileSystem(root)
	_, err := fsys.ReadFile("link/secret.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("ReadFile error = %v, want escape rejection", err)
	}
}

func TestFileSystemRejectsSymlinkCreateEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	fsys := NewFileSystem(root)
	err := fsys.WriteFile(context.Background(), "link/out.txt", []byte("x"), system.WriteFileOptions{Overwrite: true})
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("WriteFile error = %v, want escape rejection", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "out.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file stat error = %v, want not exist", err)
	}
}
