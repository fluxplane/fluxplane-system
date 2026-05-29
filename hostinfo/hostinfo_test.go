package hostinfo

import (
	"context"
	"testing"
	"time"

	"github.com/fluxplane/fluxplane-system/memsystem"
)

func TestCollectUsesSystemClock(t *testing.T) {
	sys := memsystem.New()
	sys.Clock().(*memsystem.Clock).Set(time.Unix(42, 0).UTC())
	info, err := Collect(context.Background(), sys, Request{Categories: []Category{CategoryTime}})
	if err != nil {
		t.Fatal(err)
	}
	if info.GeneratedAt.Unix() != 42 {
		t.Fatalf("GeneratedAt = %s", info.GeneratedAt)
	}
}
