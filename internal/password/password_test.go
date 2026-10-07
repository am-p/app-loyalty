package password

import (
	"context"
	"errors"
	"testing"
)

func TestGateBoundsAndCancellation(t *testing.T) {
	g := NewGate(1)
	release, e := g.Acquire(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = g.Acquire(context.Background()); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = g.Acquire(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	release, e = g.Acquire(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	release()
	if g.active.Load() != 0 || g.rejected.Load() != 1 {
		t.Fatal("bad counters")
	}
}
