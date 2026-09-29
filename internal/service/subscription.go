package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"github.com/google/uuid"
)

func (s *Service) Subscription(ctx context.Context, actorID, brandID int64) (model.Subscription, error) {
	billing, err := s.Repo.BillingContext(ctx, actorID, brandID)
	if err != nil {
		return model.Subscription{}, err
	}
	unitPrice, ok := s.subscriptionUnitPrice(billing.ProgramType)
	if !ok {
		return model.Subscription{}, ErrInvalidRequest
	}
	record, err := s.Repo.GetSubscriptionRecord(ctx, brandID)
	if errors.Is(err, repository.ErrNotFound) {
		price, priceErr := s.Repo.SubscriptionPrice(ctx, billing.ProgramType, unitPrice)
		if priceErr != nil {
			return model.Subscription{}, priceErr
		}
		unitPrice = price.UnitPriceMinor
		discounted, remaining, priceErr := s.Repo.ReferralCheckoutPrice(ctx, brandID, unitPrice)
		if priceErr != nil {
			return model.Subscription{}, priceErr
		}
		return model.Subscription{BrandID: brandID, Provider: "MERCADO_PAGO", Status: "NOT_CONFIGURED", Currency: "ARS", UnitAmountCents: discounted, ActiveBranches: billing.ActiveBranches, MonthlyAmountCents: discounted * billing.ActiveBranches, FullMonthlyAmountCents: unitPrice * billing.ActiveBranches, DiscountRemainingCharges: remaining, ProviderConfigured: s.Billing != nil, TrialAvailable: true, UpdatedAt: s.Now()}, nil
	}
	if err != nil {
		return model.Subscription{}, err
	}
	record.Subscription.ProviderConfigured = s.Billing != nil
	return record.Subscription, nil
}

func (s *Service) CreateSubscriptionCheckout(ctx context.Context, actorID, brandID int64, idempotencyKey string) (model.Subscription, error) {
	if s.Billing == nil {
		return model.Subscription{}, ErrBillingUnavailable
	}
	key, err := uuid.Parse(strings.TrimSpace(idempotencyKey))
	if err != nil {
		return model.Subscription{}, ErrInvalidRequest
	}
	external := fmt.Sprintf("puntazo:brand:%d:%s", brandID, key.String())
	billing, reserved, err := s.Repo.ReserveSubscriptionCheckout(ctx, actorID, brandID, external, s.Config.MercadoPagoBranchPrice, s.Config.MercadoPagoPointsPrice)
	if errors.Is(err, repository.ErrConflict) {
		return model.Subscription{}, ErrSubscriptionExists
	}
	if err != nil {
		return model.Subscription{}, err
	}
	if reserved.Subscription.Status != "CREATING" {
		reserved.Subscription.ProviderConfigured = true
		return reserved.Subscription, nil
	}
	claimed, err := s.Repo.ClaimSubscriptionProviderCall(ctx, brandID, reserved.ExternalReference)
	if err != nil {
		return model.Subscription{}, err
	}
	if !claimed {
		existing, lookupErr := s.Repo.GetSubscriptionRecord(ctx, brandID)
		if lookupErr == nil && existing.ExternalReference == reserved.ExternalReference && existing.Subscription.Status != "CREATING" {
			existing.Subscription.ProviderConfigured = true
			return existing.Subscription, nil
		}
		return model.Subscription{}, ErrBillingInProgress
	}
	providerKey := strings.TrimPrefix(reserved.ExternalReference, fmt.Sprintf("puntazo:brand:%d:", brandID))
	if _, err = uuid.Parse(providerKey); err != nil {
		return model.Subscription{}, ErrInvalidRequest
	}
	created, err := s.Billing.CreateSubscription(ctx, model.BillingSubscriptionRequest{
		Reason: fmt.Sprintf("Puntazo %s mensual · %d sucursal(es)", billing.ProgramType, reserved.Subscription.ActiveBranches), ExternalReference: reserved.ExternalReference,
		PayerEmail: billing.PayerEmail, BackURL: strings.TrimRight(s.Config.PublicAppURL, "/") + "/suscripcion/resultado",
		IdempotencyKey: providerKey, Currency: "ARS", AmountMinor: reserved.Subscription.MonthlyAmountCents, FreeTrialMonths: reserved.TrialMonths,
	})
	if err != nil {
		return model.Subscription{}, ErrBillingProviderFailure
	}
	if created.ExternalReference != reserved.ExternalReference || created.ID == "" {
		return model.Subscription{}, ErrBillingProviderFailure
	}
	out, err := s.Repo.SaveSubscriptionCheckout(ctx, brandID, created)
	if errors.Is(err, repository.ErrConflict) {
		existing, lookupErr := s.Repo.GetSubscriptionRecord(ctx, brandID)
		if lookupErr == nil && existing.ExternalReference == reserved.ExternalReference && existing.Subscription.Status != "CREATING" {
			existing.Subscription.ProviderConfigured = true
			return existing.Subscription, nil
		}
	}
	if err != nil {
		return out, err
	}
	current, err := s.Repo.GetSubscriptionRecord(ctx, brandID)
	if err != nil {
		return out, err
	}
	current.Subscription.ProviderConfigured = true
	return current.Subscription, nil
}

func (s *Service) ApplySubscriptionWebhook(ctx context.Context, notificationID, topic, resourceID string) error {
	if s.Billing == nil {
		return ErrBillingUnavailable
	}
	if notificationID == "" || resourceID == "" {
		return ErrInvalidRequest
	}
	if topic == "subscription_authorized_payment" || topic == "payment" {
		return s.applyReferralPaymentWebhook(ctx, notificationID, topic, resourceID)
	}
	if topic != "subscription_preapproval" {
		return ErrInvalidRequest
	}
	provider, err := s.Billing.GetSubscription(ctx, resourceID)
	if err != nil {
		return ErrBillingProviderFailure
	}
	if provider.ID != resourceID || !strings.HasPrefix(provider.ExternalReference, "puntazo:brand:") {
		return ErrInvalidRequest
	}
	return s.Repo.RecordSubscriptionWebhook(ctx, notificationID, topic, provider)
}

type referralBillingProvider interface {
	GetAuthorizedPayment(context.Context, string) (model.BillingInvoice, error)
	GetPayment(context.Context, string) (model.BillingPayment, error)
	UpdateSubscriptionAmount(context.Context, string, int64, string) (model.BillingSubscriptionResult, error)
}

var settlementReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9/_:-]{5,119}$`)

// RecordManualMerchantCredit records a Finance attestation of a completed
// external reimbursement. The provider invoice and payment are read back here;
// the external settlement itself remains an operator-supplied reference.
func (s *Service) RecordManualMerchantCredit(ctx context.Context, brandID, financeUserID, amountMinor int64, idempotencyKey, invoiceID, externalReference string) (repository.MerchantCreditAllocation, error) {
	var zero repository.MerchantCreditAllocation
	provider, ok := s.Billing.(referralBillingProvider)
	if !ok {
		return zero, ErrBillingUnavailable
	}
	key, err := uuid.Parse(strings.TrimSpace(idempotencyKey))
	if err != nil || brandID < 1 || financeUserID < 1 || amountMinor < 1 || !settlementReferencePattern.MatchString(externalReference) {
		return zero, ErrInvalidRequest
	}
	invoice, err := provider.GetAuthorizedPayment(ctx, invoiceID)
	if err != nil {
		return zero, ErrBillingProviderFailure
	}
	if invoice.ID != invoiceID || invoice.SubscriptionID == "" || invoice.Currency != "ARS" || invoice.PaymentID == "" || invoice.AmountMinor < 1 {
		return zero, ErrInvalidRequest
	}
	payment, err := provider.GetPayment(ctx, invoice.PaymentID)
	if err != nil {
		return zero, ErrBillingProviderFailure
	}
	if payment.ID != invoice.PaymentID || payment.Status != "approved" || payment.Currency != "ARS" || payment.AmountMinor != invoice.AmountMinor || payment.RefundedMinor != 0 {
		return zero, ErrInvalidRequest
	}
	return s.Repo.RecordMerchantCreditAllocation(ctx, brandID, financeUserID, amountMinor, key, externalReference, invoice, payment)
}

func (s *Service) applyReferralPaymentWebhook(ctx context.Context, notificationID, topic, resourceID string) error {
	provider, ok := s.Billing.(referralBillingProvider)
	if !ok {
		return ErrBillingUnavailable
	}
	if topic == "payment" {
		if _, err := s.Repo.ReferralInvoiceForPayment(ctx, resourceID); errors.Is(err, repository.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		payment, err := provider.GetPayment(ctx, resourceID)
		if err != nil {
			return ErrBillingProviderFailure
		}
		if payment.ID != resourceID || payment.Currency != "ARS" || payment.AmountMinor < 1 || payment.RefundedMinor < 0 || payment.RefundedMinor > payment.AmountMinor {
			return ErrInvalidRequest
		}
		return s.Repo.ReverseReferralPayment(ctx, notificationID, payment)
	}
	invoiceID := resourceID
	invoice, err := provider.GetAuthorizedPayment(ctx, invoiceID)
	if err != nil {
		return ErrBillingProviderFailure
	}
	if invoice.ID != invoiceID || invoice.SubscriptionID == "" || invoice.Currency != "ARS" || invoice.AmountMinor < 0 {
		return ErrInvalidRequest
	}
	if invoice.PaymentID == "" || invoice.AmountMinor == 0 {
		return nil
	} // scheduled, failed or free invoices do not earn rewards
	payment, err := provider.GetPayment(ctx, invoice.PaymentID)
	if err != nil {
		return ErrBillingProviderFailure
	}
	if payment.ID != invoice.PaymentID || payment.Currency != "ARS" || payment.AmountMinor != invoice.AmountMinor || payment.RefundedMinor < 0 || payment.RefundedMinor > payment.AmountMinor {
		return ErrInvalidRequest
	}
	advance := func(ctx context.Context, id string, amountMinor int64, key string) error {
		updated, err := provider.UpdateSubscriptionAmount(ctx, id, amountMinor, key)
		if err != nil {
			return ErrBillingProviderFailure
		}
		if updated.ID != id || updated.AmountMinor != amountMinor {
			return ErrBillingProviderFailure
		}
		return nil
	}
	return s.Repo.RecordReferralInvoice(ctx, notificationID, invoice, payment, advance)
}

func (s *Service) CancelSubscription(ctx context.Context, actorID, brandID int64, idempotencyKey string) (model.Subscription, error) {
	if s.Billing == nil {
		return model.Subscription{}, ErrBillingUnavailable
	}
	key, err := uuid.Parse(strings.TrimSpace(idempotencyKey))
	if err != nil {
		return model.Subscription{}, ErrInvalidRequest
	}
	billing, err := s.Repo.BillingContext(ctx, actorID, brandID)
	if err != nil {
		return model.Subscription{}, err
	}
	if _, ok := s.subscriptionUnitPrice(billing.ProgramType); !ok {
		return model.Subscription{}, ErrInvalidRequest
	}
	record, err := s.Repo.GetSubscriptionRecord(ctx, brandID)
	if err != nil {
		return model.Subscription{}, err
	}
	if record.Subscription.Status == "CANCELLED" {
		record.Subscription.ProviderConfigured = true
		return record.Subscription, nil
	}
	if record.Subscription.Status == "CREATING" {
		return model.Subscription{}, ErrBillingInProgress
	}
	provider, err := s.Billing.CancelSubscription(ctx, record.ProviderID, key.String())
	if err != nil {
		return model.Subscription{}, ErrBillingProviderFailure
	}
	if provider.ID != record.ProviderID || provider.ExternalReference != record.ExternalReference {
		return model.Subscription{}, ErrBillingProviderFailure
	}
	out, err := s.Repo.UpdateSubscriptionFromProvider(ctx, provider)
	if err != nil {
		return out, err
	}
	current, err := s.Repo.GetSubscriptionRecord(ctx, brandID)
	if err != nil {
		return out, err
	}
	current.Subscription.ProviderConfigured = true
	return current.Subscription, nil
}

func (s *Service) subscriptionUnitPrice(programType string) (int64, bool) {
	switch programType {
	case "SELLOS":
		return s.Config.MercadoPagoBranchPrice, true
	case "PUNTOS":
		return s.Config.MercadoPagoPointsPrice, true
	default:
		return 0, false
	}
}
