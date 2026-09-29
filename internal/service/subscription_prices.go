package service

import (
	"context"
	"log/slog"
	"time"

	"clientesFrecuentes/internal/repository"
	"github.com/google/uuid"
)

func (s *Service) SubscriptionPriceUpdatesAvailable() bool {
	_, ok := s.Billing.(referralBillingProvider)
	return ok
}

func (s *Service) ChangeSubscriptionPrice(ctx context.Context, adminID int64, key uuid.UUID, program string, amount, version int64, includeExisting bool) (repository.SubscriptionPriceChange, error) {
	fallback, ok := s.subscriptionUnitPrice(program)
	if !ok {
		return repository.SubscriptionPriceChange{}, repository.ErrInvalidRequest
	}
	return s.Repo.ChangeSubscriptionPrice(ctx, adminID, key, program, amount, version, fallback, includeExisting)
}

// Durable jobs survive server restarts and never keep an HTTP request open.
func (s *Service) RunSubscriptionPriceChanges(ctx context.Context, logger *slog.Logger) {
	provider, ok := s.Billing.(referralBillingProvider)
	if !ok {
		return
	}
	for ctx.Err() == nil {
		workCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		handled, err := s.Repo.ApplyNextSubscriptionPriceChange(workCtx, func(ctx context.Context, id string, amount int64, key string) error {
			updated, err := provider.UpdateSubscriptionAmount(ctx, id, amount, key)
			if err != nil || updated.ID != id || updated.AmountMinor != amount {
				return ErrBillingProviderFailure
			}
			return nil
		})
		cancel()
		if err != nil {
			logger.Warn("subscription price update requires retry", "code", "BILLING_UPDATE_FAILED")
		}
		if handled && err == nil {
			continue
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) SubscriptionPrices(ctx context.Context) ([]repository.SubscriptionPrice, error) {
	out := make([]repository.SubscriptionPrice, 0, 2)
	for _, program := range []string{"SELLOS", "PUNTOS"} {
		fallback, _ := s.subscriptionUnitPrice(program)
		price, err := s.Repo.SubscriptionPrice(ctx, program, fallback)
		if err != nil {
			return nil, err
		}
		out = append(out, price)
	}
	return out, nil
}
