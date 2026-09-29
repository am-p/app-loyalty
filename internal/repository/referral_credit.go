package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"clientesFrecuentes/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MerchantCreditAllocation records Finance's attestation of an external
// reimbursement against a verified paid subscription invoice. It does not
// change the Mercado Pago recurring amount or initiate a refund.
type MerchantCreditAllocation struct {
	ID                          int64      `json:"id"`
	BrandID                     int64      `json:"brand_id"`
	InvoiceID                   string     `json:"provider_invoice_id"`
	PaymentID                   string     `json:"provider_payment_id"`
	AmountMinor                 int64      `json:"amount_minor"`
	Currency                    string     `json:"currency"`
	ExternalSettlementReference string     `json:"external_settlement_reference"`
	Status                      string     `json:"status"`
	RecoveryDueAt               *time.Time `json:"recovery_due_at,omitempty"`
	CreatedAt                   time.Time  `json:"created_at"`
}

type MerchantCreditAccount struct {
	BalanceMinor int64                      `json:"balance_minor"`
	Allocations  []MerchantCreditAllocation `json:"allocations"`
}

func (r *Repository) MerchantCreditAccount(ctx context.Context, brandID int64) (MerchantCreditAccount, error) {
	var out MerchantCreditAccount
	err := r.Pool.QueryRow(ctx, `SELECT balance_minor FROM referral_merchant_credit_balances WHERE brand_id=$1`, brandID).Scan(&out.BalanceMinor)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	out.Allocations, err = r.ListMerchantCreditAllocations(ctx, brandID)
	return out, err
}

func (r *Repository) RecordMerchantCreditAllocation(ctx context.Context, brandID, financeUserID, amountMinor int64, idempotencyKey uuid.UUID, externalReference string, invoice model.BillingInvoice, payment model.BillingPayment) (MerchantCreditAllocation, error) {
	var out MerchantCreditAllocation
	if brandID < 1 || financeUserID < 1 || amountMinor < 1 || invoice.ID == "" || invoice.SubscriptionID == "" || invoice.PaymentID != payment.ID || payment.Status != "approved" || payment.RefundedMinor != 0 || payment.Currency != "ARS" || payment.AmountMinor != invoice.AmountMinor {
		return out, ErrInvalidRequest
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var financeAllowed bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backoffice_users WHERE id=$1 AND active AND role='FINANZAS')`, financeUserID).Scan(&financeAllowed); err != nil {
		return out, err
	}
	if !financeAllowed {
		return out, ErrForbidden
	}
	var lockedBrandID int64
	if err = tx.QueryRow(ctx, `SELECT id FROM marcas WHERE id=$1 FOR UPDATE`, brandID).Scan(&lockedBrandID); errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	} else if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT id,source_brand_id,provider_invoice_id,provider_payment_id,amount_minor,currency,external_settlement_reference,status,recovery_due_at,created_at FROM referral_merchant_credit_allocations WHERE idempotency_key=$1`, idempotencyKey).Scan(&out.ID, &out.BrandID, &out.InvoiceID, &out.PaymentID, &out.AmountMinor, &out.Currency, &out.ExternalSettlementReference, &out.Status, &out.RecoveryDueAt, &out.CreatedAt)
	if err == nil {
		if out.BrandID != brandID || out.InvoiceID != invoice.ID || out.PaymentID != payment.ID || out.AmountMinor != amountMinor || out.ExternalSettlementReference != externalReference {
			return MerchantCreditAllocation{}, ErrIdempotencyConflict
		}
		return out, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	var linkedBrandID int64
	if err = tx.QueryRow(ctx, `SELECT marca_id FROM suscripciones_marca WHERE proveedor_suscripcion_id=$1 AND marca_id=$2`, invoice.SubscriptionID, brandID).Scan(&linkedBrandID); errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	} else if err != nil {
		return out, err
	}
	var accrued, allocated, invoiceAllocated int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(amount_minor),0) FROM referral_rewards WHERE source_kind='MERCHANT' AND source_brand_id=$1 AND status='PENDING'`, brandID).Scan(&accrued); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(amount_minor),0) FROM referral_merchant_credit_allocations WHERE source_brand_id=$1`, brandID).Scan(&allocated); err != nil {
		return out, err
	}
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(amount_minor),0) FROM referral_merchant_credit_allocations WHERE provider_payment_id=$1`, payment.ID).Scan(&invoiceAllocated); err != nil {
		return out, err
	}
	if amountMinor > accrued-allocated || amountMinor > payment.AmountMinor-invoiceAllocated {
		return out, ErrInsufficientBalance
	}
	err = tx.QueryRow(ctx, `INSERT INTO referral_merchant_credit_allocations(source_brand_id,provider_invoice_id,provider_payment_id,amount_minor,external_settlement_reference,idempotency_key,finance_user_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,source_brand_id,provider_invoice_id,provider_payment_id,amount_minor,currency,external_settlement_reference,status,recovery_due_at,created_at`, brandID, invoice.ID, payment.ID, amountMinor, externalReference, idempotencyKey, financeUserID).Scan(&out.ID, &out.BrandID, &out.InvoiceID, &out.PaymentID, &out.AmountMinor, &out.Currency, &out.ExternalSettlementReference, &out.Status, &out.RecoveryDueAt, &out.CreatedAt)
	if IsUniqueViolation(err) {
		return MerchantCreditAllocation{}, ErrConflict
	}
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO backoffice_audit(user_id,action,object_type,object_id,detail) VALUES($1,'merchant_credit.record','credit_allocation',$2,jsonb_build_object('brand_id',$3::bigint,'invoice_id',$4::text,'payment_id',$5::text,'amount_minor',$6::bigint,'external_settlement_reference',$7::text))`, financeUserID, fmt.Sprint(out.ID), brandID, invoice.ID, payment.ID, amountMinor, externalReference)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func (r *Repository) ListMerchantCreditAllocations(ctx context.Context, brandID int64) ([]MerchantCreditAllocation, error) {
	rows, err := r.Pool.Query(ctx, `SELECT id,source_brand_id,provider_invoice_id,provider_payment_id,amount_minor,currency,external_settlement_reference,status,recovery_due_at,created_at FROM referral_merchant_credit_allocations WHERE source_brand_id=$1 ORDER BY id DESC LIMIT 200`, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MerchantCreditAllocation, 0)
	for rows.Next() {
		var a MerchantCreditAllocation
		if err = rows.Scan(&a.ID, &a.BrandID, &a.InvoiceID, &a.PaymentID, &a.AmountMinor, &a.Currency, &a.ExternalSettlementReference, &a.Status, &a.RecoveryDueAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
