package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"

	"github.com/google/uuid"
)

func (s *Service) IdempotentMovement(ctx context.Context, actorID int64, key string) (json.RawMessage, error) {
	if _, err := uuid.Parse(key); err != nil {
		return nil, ErrInvalidRequest
	}
	return s.Repo.GetMovementIdempotency(ctx, actorID, key)
}

func Fingerprint(v any) []byte { b, _ := json.Marshal(v); sum := sha256.Sum256(b); return sum[:] }

func KeyedFingerprint(key string, v any) []byte {
	b, _ := json.Marshal(v)
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(b)
	return mac.Sum(nil)
}
