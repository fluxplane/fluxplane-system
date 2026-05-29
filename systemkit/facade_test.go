package systemkit

import (
	"context"
	"testing"

	"github.com/fluxplane/fluxplane-system"
	"github.com/fluxplane/fluxplane-system/memsystem"
)

func TestFacadeImplementsSystem(t *testing.T) {
	var _ system.System = New(memsystem.New())
}

func TestFacadeFileHelpers(t *testing.T) {
	base := memsystem.New()
	facade := New(base)
	if err := base.FileSystem().WriteFile(context.Background(), "a.txt", []byte("abcdef"), system.WriteFileOptions{}); err != nil {
		t.Fatal(err)
	}
	data, truncated, err := facade.ReadFileLimit(context.Background(), "a.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "abc" || !truncated {
		t.Fatalf("ReadFileLimit = %q truncated=%v", data, truncated)
	}
}
