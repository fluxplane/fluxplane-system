package systemkit

import (
	"context"
	"errors"
	"testing"

	"github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/hostsystem"
	"github.com/fluxplane/fluxplane-system/memsystem"
	"github.com/fluxplane/fluxplane-system/mountfs"
)

func TestBuilderWithoutNetworkReturnsUnsupportedNetwork(t *testing.T) {
	facade, err := NewSystem().WithoutNetwork().Build()
	if err != nil {
		t.Fatal(err)
	}
	_, err = facade.Network().DialContext(context.Background(), "tcp", "example.invalid:80")
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("DialContext error = %v, want ErrUnsupported", err)
	}
}

func TestBuilderUnsupportedDefaults(t *testing.T) {
	facade, err := NewSystem().Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.FileSystem().Open("missing"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("FileSystem.Open error = %v, want ErrUnsupported", err)
	}
	if _, _, err := facade.Environment().Lookup(context.Background(), "PATH"); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Environment.Lookup error = %v, want ErrUnsupported", err)
	}
	if _, err := facade.Process().List(context.Background()); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Process.List error = %v, want ErrUnsupported", err)
	}
}

func TestBuilderRejectsNilCapability(t *testing.T) {
	_, err := NewSystem().WithFileSystem(nil).Build()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuilderWithMountedFileSystemWrapsCurrentFileSystem(t *testing.T) {
	ctx := context.Background()
	base := memsystem.NewFileSystem()
	if err := base.WriteFile(ctx, "folder-a/a.txt", []byte("a"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if err := base.WriteFile(ctx, "outside.txt", []byte("outside"), system.WriteFileOptions{Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	facade, err := NewSystem().
		WithFileSystem(base).
		WithMountedFileSystem(mountfs.Spec{Roots: []mountfs.Root{
			{Name: "a", Path: "folder-a", Access: mountfs.ReadOnly},
		}}).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := facade.FileSystem().ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "@a" {
		t.Fatalf("unexpected mount root entries: %+v", entries)
	}
	if _, err := facade.FileSystem().ReadFile("outside.txt"); err == nil {
		t.Fatal("expected unmounted file to be hidden")
	}
}

func TestBuilderWithMountedFileSystemRejectsInvalidSpec(t *testing.T) {
	_, err := NewSystem().
		WithFileSystem(memsystem.NewFileSystem()).
		WithMountedFileSystem(mountfs.Spec{Roots: []mountfs.Root{
			{Name: "bad/root", Path: "."},
		}}).
		Build()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewHostBuildsSystemFacade(t *testing.T) {
	facade, err := NewHost(hostsystem.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var _ system.System = facade
	if facade.FileSystem() == nil || facade.Network() == nil || facade.Environment() == nil || facade.Process() == nil || facade.Clock() == nil {
		t.Fatal("expected all host capabilities")
	}
}
