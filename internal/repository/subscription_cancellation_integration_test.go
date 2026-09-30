package repository_test

import (
	"testing"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"github.com/google/uuid"
)

func TestPostgresCancellationClearsCheckoutAndScheduledCharge(t *testing.T) {
	for _, webhook := range []bool{false, true} {
		name := "provider response"
		if webhook {
			name = "webhook"
		}
		t.Run(name, func(t *testing.T) {
			pool := referralBillingPool(t)
			f := accountDeletionFixture(t, pool, "AUTHORIZED")
			repo := repository.New(pool)
			next := time.Now().Add(30 * 24 * time.Hour)
			provider := model.BillingSubscriptionResult{ID: f.provider.id, ExternalReference: f.provider.external, Status: "cancelled", NextPaymentDate: &next, CheckoutURL: "https://provider.example.test/old-checkout"}
			apply := func(notification string) {
				t.Helper()
				if webhook {
					if err := repo.RecordSubscriptionWebhook(t.Context(), notification, "subscription_preapproval", provider); err != nil {
						t.Fatal(err)
					}
				} else {
					out, err := repo.UpdateSubscriptionFromProvider(t.Context(), provider)
					if err != nil || out.Status != "CANCELLED" || out.NextPaymentDate != nil || out.CheckoutURL != "" {
						t.Fatalf("cancellation response=%+v err=%v", out, err)
					}
				}
				record, err := repo.GetSubscriptionRecord(t.Context(), f.brand)
				if err != nil || record.Subscription.Status != "CANCELLED" || record.Subscription.NextPaymentDate != nil || record.Subscription.CheckoutURL != "" {
					t.Fatalf("stored cancellation=%+v err=%v", record, err)
				}
			}
			apply("cancelled-event")
			provider.Status = "authorized"
			apply("late-authorized-event")
		})
	}
}

func TestPostgresCancellationRetryClearsLegacyScheduleWithoutProviderCall(t *testing.T) {
	pool := referralBillingPool(t)
	f := accountDeletionFixture(t, pool, "CANCELLED")
	if _, err := pool.Exec(t.Context(), `UPDATE suscripciones_marca SET checkout_url='https://provider.example.test/old-checkout',proximo_cobro_at=now()+interval '1 month' WHERE marca_id=$1`, f.brand); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(pool)
	svc := service.New(repo, nil, config.Config{MercadoPagoBranchPrice: 15000})
	svc.Billing = f.provider
	for range 2 {
		out, err := svc.CancelSubscription(t.Context(), f.owner, f.brand, uuid.NewString())
		if err != nil || out.Status != "CANCELLED" || out.NextPaymentDate != nil || out.CheckoutURL != "" {
			t.Fatalf("retry result=%+v err=%v", out, err)
		}
	}
	if len(f.provider.keys) != 0 {
		t.Fatal("already cancelled subscription called the provider again")
	}
}
