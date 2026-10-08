package middleware

import (
	"context"
	"sync"
)

type uploadActorGate struct {
	slots chan struct{}
	refs  int
}
type UploadSemaphore struct {
	global     chan struct{}
	actorLimit int
	mu         sync.Mutex
	actors     map[int64]*uploadActorGate
}

func NewUploadSemaphore(globalLimit, actorLimit int) *UploadSemaphore {
	return &UploadSemaphore{global: make(chan struct{}, globalLimit), actorLimit: actorLimit, actors: make(map[int64]*uploadActorGate)}
}

func (s *UploadSemaphore) Acquire(ctx context.Context, actorID int64) (func(), bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	// Reject immediately when saturated; waiting actors cannot grow an unbounded map.
	select {
	case s.global <- struct{}{}:
	default:
		return nil, false
	}
	s.mu.Lock()
	gate := s.actors[actorID]
	if gate == nil {
		gate = &uploadActorGate{slots: make(chan struct{}, s.actorLimit)}
		s.actors[actorID] = gate
	}
	gate.refs++
	s.mu.Unlock()
	select {
	case gate.slots <- struct{}{}:
	default:
		<-s.global
		s.releaseRef(actorID, gate)
		return nil, false
	}
	var once sync.Once
	return func() { once.Do(func() { <-gate.slots; <-s.global; s.releaseRef(actorID, gate) }) }, true
}
func (s *UploadSemaphore) Stats() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.global), len(s.actors)
}

func (s *UploadSemaphore) releaseRef(actorID int64, gate *uploadActorGate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gate.refs--
	if gate.refs == 0 {
		delete(s.actors, actorID)
	}
}
