package middleware

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestUploadSemaphoreBoundsActorAndGlobalConcurrency(t *testing.T) {
	gate := NewUploadSemaphore(2, 1)
	releaseFirst, ok := gate.Acquire(context.Background(), 7)
	if !ok {
		t.Fatal("first acquire")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, ok = gate.Acquire(ctx, 7); ok {
		t.Fatal("same actor exceeded limit")
	}
	releaseOther, ok := gate.Acquire(context.Background(), 8)
	if !ok {
		t.Fatal("other actor should use global capacity")
	}
	releaseOther()
	releaseFirst()
	if len(gate.actors) != 0 || len(gate.global) != 0 {
		t.Fatalf("semaphore leaked actors=%d global=%d", len(gate.actors), len(gate.global))
	}
}

func TestUploadSemaphoreActorWaiterDoesNotConsumeGlobalCapacity(t *testing.T) {
	gate := NewUploadSemaphore(2, 1)
	releaseFirst, ok := gate.Acquire(context.Background(), 7)
	if !ok {
		t.Fatal("first acquire")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var waiter sync.WaitGroup
	waiter.Add(1)
	waiting := make(chan struct{})
	go func() {
		defer waiter.Done()
		close(waiting)
		release, acquired := gate.Acquire(ctx, 7)
		if acquired {
			release()
		}
	}()
	<-waiting

	otherCtx, otherCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer otherCancel()
	releaseOther, ok := gate.Acquire(otherCtx, 8)
	if !ok {
		t.Fatal("same-actor waiter consumed the remaining global slot")
	}
	releaseOther()
	releaseFirst()
	waiter.Wait()
	if len(gate.actors) != 0 || len(gate.global) != 0 {
		t.Fatalf("semaphore leaked actors=%d global=%d", len(gate.actors), len(gate.global))
	}
}
