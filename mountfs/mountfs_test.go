package mountfs_test

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	system "github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/memsystem"
	"github.com/fluxplane/fluxplane-system/mountfs"
)

func TestPrimaryRootMapsReadsAndWrites(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "workspace/readme.md", []byte("hello"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "", Path: "workspace", Access: mountfs.ReadWrite},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := fsys.ReadFile("readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected data %q", string(data))
	}
	if err := fsys.WriteFile(ctx, "notes/todo.txt", []byte("ship"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	data, err = base.ReadFile("workspace/notes/todo.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ship" {
		t.Fatalf("unexpected mapped write %q", string(data))
	}
}

func TestNamedRootReadOnly(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "docs/readme.md", []byte("docs"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := fsys.ReadFile("@docs/readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "docs" {
		t.Fatalf("unexpected data %q", string(data))
	}
	err = fsys.WriteFile(ctx, "@docs/readme.md", []byte("changed"), system.WriteFileOptions{Overwrite: true})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
}

func TestRenameRequiresBothRootsWritable(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "scratch/item.txt", []byte("item"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "scratch", Path: "scratch", Access: mountfs.ReadWrite, Scratch: true},
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	err = fsys.Rename(ctx, "@scratch/item.txt", "@docs/item.txt", system.RenameOptions{})
	if err == nil || !strings.Contains(err.Error(), "destination root") {
		t.Fatalf("expected destination root error, got %v", err)
	}
}

func TestResolveRejectsEscapesAndUnknownRoots(t *testing.T) {
	base := memsystem.NewFileSystem()
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "", Path: "workspace", Access: mountfs.ReadWrite},
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret", "a/../secret", "@docs/../secret", "/abs", "@missing/file"} {
		if _, err := fsys.Resolve(name); err == nil {
			t.Fatalf("expected %q to fail", name)
		}
	}
	resolved, err := fsys.Resolve("@docs/readme.md")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Name != "@docs/readme.md" || resolved.Path != "docs/readme.md" {
		t.Fatalf("unexpected resolve: %+v", resolved)
	}
}

func TestRootValidation(t *testing.T) {
	base := memsystem.NewFileSystem()
	tests := []mountfs.Root{
		{Name: "bad/root", Path: "."},
		{Name: "@bad", Path: "."},
		{Name: "bad space", Path: "."},
		{Name: "docs", Path: "../docs"},
		{Name: "docs", Path: "/docs"},
		{Name: "docs", Path: "a/../docs"},
	}
	for _, root := range tests {
		if _, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{root}}); err == nil {
			t.Fatalf("expected root %+v to fail", root)
		}
	}
}

func TestWalkNamedRoot(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "docs/a.txt", []byte("a"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(fsys, "@docs")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "a.txt" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestVirtualRootListsConfiguredMounts(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "folder-a/a.txt", []byte("a"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if err := base.WriteFile(ctx, "folder-b/b.txt", []byte("b"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if err := base.WriteFile(ctx, "outside.txt", []byte("outside"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "a", Path: "folder-a", Access: mountfs.ReadWrite},
		{Name: "b", Path: "folder-b", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fsys.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	names := entryNames(entries)
	if strings.Join(names, ",") != "@a,@b" {
		t.Fatalf("unexpected virtual root entries: %v", names)
	}
	if _, err := fsys.ReadFile("outside.txt"); err == nil {
		t.Fatal("expected bare outside path to fail without a primary root")
	}
	var walked []string
	if err := fs.WalkDir(fsys, ".", func(name string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		walked = append(walked, name)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(walked, ",") != ".,@a,@a/a.txt,@b,@b/b.txt" {
		t.Fatalf("unexpected walk: %v", walked)
	}
}

func TestVirtualRootCombinesPrimaryAndNamedRoots(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "workspace/readme.md", []byte("hello"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if err := base.WriteFile(ctx, "docs/guide.md", []byte("docs"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "", Path: "workspace", Access: mountfs.ReadWrite},
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fsys.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	names := entryNames(entries)
	if strings.Join(names, ",") != "@docs,readme.md" {
		t.Fatalf("unexpected virtual root entries: %v", names)
	}
	if err := fsys.Remove(ctx, "."); err == nil {
		t.Fatal("expected removing virtual root to fail")
	}
	if err := fsys.Rename(ctx, ".", "moved", system.RenameOptions{}); err == nil {
		t.Fatal("expected renaming virtual root to fail")
	}
}

func TestImplementsSystemFileSystem(t *testing.T) {
	base := memsystem.NewFileSystem()
	fsys, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "", Path: ".", Access: mountfs.ReadOnly},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var _ system.FileSystem = fsys
}

func entryNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
