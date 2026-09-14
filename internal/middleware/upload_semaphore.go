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
	case <-ctx.Done():
		s.releaseRef(actorID, gate)
		return nil, false
	}
	select {
	case s.global <- struct{}{}:
	case <-ctx.Done():
		<-gate.slots
		s.releaseRef(actorID, gate)
		return nil, false
	}
	var once sync.Once
	return func() { once.Do(func() { <-gate.slots; <-s.global; s.releaseRef(actorID, gate) }) }, true
}
func (s *UploadSemaphore) releaseRef(actorID int64, gate *uploadActorGate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gate.refs--
	if gate.refs == 0 {
		delete(s.actors, actorID)
	}
}
