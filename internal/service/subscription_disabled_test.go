package service

import (
	"errors"
	"testing"

	"clientesFrecuentes/internal/config"
)

func TestDisabledSubscriptionWritesRejectBeforeDependencies(t *testing.T) {
	s := New(nil, nil, config.Config{MercadoPagoProvider: "disabled"})
	if s.Billing != nil {
		t.Fatal("disabled service must not have a billing client")
	}
	if _, err := s.CreateSubscriptionCheckout(t.Context(), 1, 1, "unused-key"); !errors.Is(err, ErrBillingUnavailable) {
		t.Fatalf("checkout error=%v", err)
	}
	if _, err := s.CancelSubscription(t.Context(), 1, 1, "unused-key"); !errors.Is(err, ErrBillingUnavailable) {
		t.Fatalf("cancel error=%v", err)
	}
}
