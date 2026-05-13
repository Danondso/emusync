package discovery_test

import (
	"context"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/discovery"
)

func TestLookupCanceledParentReturnsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := discovery.Lookup(ctx, time.Second)
	if len(got) != 0 {
		t.Fatalf("expected no results, got %+v", got)
	}
}
