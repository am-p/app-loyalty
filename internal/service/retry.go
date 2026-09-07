package service

import (
	"context"
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
