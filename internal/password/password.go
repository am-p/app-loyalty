// Package password shares a bounded bcrypt work gate across every auth flow.
package password

import (
	"context"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"sync/atomic"
)

var ErrBusy = errors.New("password work capacity exhausted")

type Gate struct {
	slots    chan struct{}
	rejected atomic.Uint64
	active   atomic.Int64
}

func NewGate(limit int) *Gate {
	if limit < 1 {
		limit = 4
	}
	return &Gate{slots: make(chan struct{}, limit)}
}

var global = NewGate(4)

// Configure is called once at startup, before accepting requests.
func Configure(limit int) { global = NewGate(limit) }
func (g *Gate) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case g.slots <- struct{}{}:
		g.active.Add(1)
		return func() { <-g.slots; g.active.Add(-1) }, nil
	default:
		g.rejected.Add(1)
		return nil, ErrBusy
	}
}
func Stats() (int64, uint64) { return global.active.Load(), global.rejected.Load() }
func Generate(ctx context.Context, plain []byte, cost int) ([]byte, error) {
	release, e := global.Acquire(ctx)
	if e != nil {
		return nil, e
	}
	defer release()
	return bcrypt.GenerateFromPassword(plain, cost)
}
func Compare(ctx context.Context, hash, plain []byte) error {
	release, e := global.Acquire(ctx)
	if e != nil {
		return e
	}
	defer release()
	return bcrypt.CompareHashAndPassword(hash, plain)
}
