package service

import (
	"context"
	"math/rand/v2"
	"time"

	"clientesFrecuentes/internal/repository"
)

func retry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !repository.IsRetryable(err) {
			return err
		}
		delay := time.Duration(10+attempt*15) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}

// Registrations sharing initials contend on one counter row. Serializable
// transactions must retry with a fresh snapshot; jitter prevents lockstep retries.
// Keep this budget local to signup, without changing ledger retry behavior.
func retryRegistration(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 16; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !repository.IsRetryable(err) {
			return err
		}
		delay := time.Duration(10*(1<<min(attempt, 4))+rand.IntN(50)) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
