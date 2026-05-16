package discovery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/discovery"
)

func TestLookupCanceledParentReturnsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := discovery.Lookup(ctx, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no results, got %+v", got)
	}
}
