package mountfs_test

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	system "github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/memsystem"
	"github.com/fluxplane/fluxplane-system/mountfs"
)

func ExampleNew_namedMounts() {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	_ = base.WriteFile(ctx, "folder-a/a.txt", []byte("a"), system.WriteFileOptions{Overwrite: true})
	_ = base.WriteFile(ctx, "folder-b/b.txt", []byte("b"), system.WriteFileOptions{Overwrite: true})
	_ = base.WriteFile(ctx, "outside.txt", []byte("outside"), system.WriteFileOptions{Overwrite: true})

	mounted, _ := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "a", Path: "folder-a", Access: mountfs.ReadWrite},
		{Name: "b", Path: "folder-b", Access: mountfs.ReadOnly},
	}})

	entries, _ := mounted.ReadDir(".")
	fmt.Println(names(entries))
	data, _ := mounted.ReadFile("@b/b.txt")
	fmt.Println(string(data))
	_, err := mounted.ReadFile("outside.txt")
	fmt.Println(err != nil)

	// Output:
	// @a, @b
	// b
	// true
}

func ExampleNew_primaryRootWithNamedMount() {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	_ = base.WriteFile(ctx, "workspace/readme.md", []byte("hello"), system.WriteFileOptions{Overwrite: true})
	_ = base.WriteFile(ctx, "docs/guide.md", []byte("docs"), system.WriteFileOptions{Overwrite: true})

	mounted, _ := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "", Path: "workspace", Access: mountfs.ReadWrite},
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})

	entries, _ := mounted.ReadDir(".")
	fmt.Println(names(entries))

	// Output:
	// @docs, readme.md
}

func ExampleNew_rejectsEscapeRoot() {
	base := memsystem.NewFileSystem()
	_, err := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "sibling", Path: "../folder-b", Access: mountfs.ReadOnly},
	}})
	fmt.Println(err != nil)

	// Output:
	// true
}

func ExampleFileSystem_WriteFile_readOnlyRoot() {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	_ = base.WriteFile(ctx, "docs/guide.md", []byte("docs"), system.WriteFileOptions{Overwrite: true})

	mounted, _ := mountfs.New(base, mountfs.Spec{Roots: []mountfs.Root{
		{Name: "docs", Path: "docs", Access: mountfs.ReadOnly},
	}})

	err := mounted.WriteFile(ctx, "@docs/guide.md", []byte("changed"), system.WriteFileOptions{Overwrite: true})
	fmt.Println(strings.Contains(err.Error(), "read-only"))

	// Output:
	// true
}

func names(entries []fs.DirEntry) string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return strings.Join(names, ", ")
}
