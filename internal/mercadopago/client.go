package mercadopago

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"
)

type Client struct {
	baseURL, accessToken string
	http                 *http.Client
}

func New(baseURL, accessToken string, timeout time.Duration) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), accessToken: accessToken, http: &http.Client{Timeout: timeout}}
}

type preapproval struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	ExternalReference string     `json:"external_reference"`
	InitPoint         string     `json:"init_point"`
	NextPaymentDate   *time.Time `json:"next_payment_date"`
	AutoRecurring     struct {
		Amount json.Number `json:"transaction_amount"`
	} `json:"auto_recurring"`
}

func decimalMinor(raw string) (int64, error) {
	parts := strings.Split(raw, ".")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" || strings.HasPrefix(raw, "-") {
		return 0, errors.New("invalid payment amount")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > (int64(^uint64(0)>>1)-99)/100 {
		return 0, errors.New("payment amount overflow")
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) == 0 || len(parts[1]) > 2 {
			return 0, errors.New("invalid payment precision")
		}
		fraction = parts[1] + strings.Repeat("0", 2-len(parts[1]))
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, err
	}
	return whole*100 + cents, nil
}

func minorJSON(cents int64) json.Number {
	return json.Number(fmt.Sprintf("%d.%02d", cents/100, cents%100))
}

func (c *Client) CreateSubscription(ctx context.Context, in model.BillingSubscriptionRequest) (model.BillingSubscriptionResult, error) {
	amount := any(in.Amount)
	if in.AmountMinor > 0 {
		amount = minorJSON(in.AmountMinor)
	}
	autoRecurring := map[string]any{"frequency": 1, "frequency_type": "months", "transaction_amount": amount, "currency_id": in.Currency}
	if in.FreeTrialMonths > 0 {
		autoRecurring["free_trial"] = map[string]any{"frequency": in.FreeTrialMonths, "frequency_type": "months"}
	}
	body := map[string]any{
		"reason": in.Reason, "external_reference": in.ExternalReference, "payer_email": in.PayerEmail,
		"auto_recurring": autoRecurring,
		"back_url":       in.BackURL, "status": "pending",
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/preapproval", bytes.NewReader(encoded))
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", in.IdempotencyKey)
	return c.do(req)
}

func (c *Client) UpdateSubscriptionAmount(ctx context.Context, id string, amountMinor int64, key string) (model.BillingSubscriptionResult, error) {
	if id == "" || amountMinor < 1 || key == "" {
		return model.BillingSubscriptionResult{}, errors.New("invalid subscription amount update")
	}
	body, err := json.Marshal(map[string]any{"auto_recurring": map[string]any{"transaction_amount": minorJSON(amountMinor), "currency_id": "ARS"}})
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/preapproval/"+id, bytes.NewReader(body))
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", key)
	return c.do(req)
}

func numericID(id string) bool {
	if id == "" {
		return false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mercado pago returned %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst)
}

func (c *Client) GetAuthorizedPayment(ctx context.Context, id string) (model.BillingInvoice, error) {
	if !numericID(id) {
		return model.BillingInvoice{}, errors.New("invalid invoice id")
	}
	var raw struct {
		ID            json.Number `json:"id"`
		PreapprovalID string      `json:"preapproval_id"`
		Currency      string      `json:"currency_id"`
		Amount        json.Number `json:"transaction_amount"`
		CreatedAt     *time.Time  `json:"date_created"`
		Payment       struct {
			ID     json.Number `json:"id"`
			Status string      `json:"status"`
		} `json:"payment"`
	}
	if err := c.getJSON(ctx, "/authorized_payments/"+id, &raw); err != nil {
		return model.BillingInvoice{}, err
	}
	amount, err := decimalMinor(raw.Amount.String())
	if err != nil {
		return model.BillingInvoice{}, err
	}
	invoice := model.BillingInvoice{ID: raw.ID.String(), SubscriptionID: raw.PreapprovalID, Currency: raw.Currency, AmountMinor: amount, PaymentID: raw.Payment.ID.String(), PaymentStatus: raw.Payment.Status}
	if raw.CreatedAt != nil {
		invoice.CreatedAt = *raw.CreatedAt
	}
	return invoice, nil
}

func (c *Client) GetPayment(ctx context.Context, id string) (model.BillingPayment, error) {
	if !numericID(id) {
		return model.BillingPayment{}, errors.New("invalid payment id")
	}
	var raw struct {
		ID       json.Number `json:"id"`
		Status   string      `json:"status"`
		Currency string      `json:"currency_id"`
		Amount   json.Number `json:"transaction_amount"`
		Refunded json.Number `json:"transaction_amount_refunded"`
	}
	if err := c.getJSON(ctx, "/v1/payments/"+id, &raw); err != nil {
		return model.BillingPayment{}, err
	}
	amount, err := decimalMinor(raw.Amount.String())
	if err != nil {
		return model.BillingPayment{}, err
	}
	refunded := int64(0)
	if raw.Refunded != "" {
		refunded, err = decimalMinor(raw.Refunded.String())
	}
	if err != nil {
		return model.BillingPayment{}, err
	}
	return model.BillingPayment{ID: raw.ID.String(), Status: raw.Status, Currency: raw.Currency, AmountMinor: amount, RefundedMinor: refunded}, nil
}

func (c *Client) GetSubscription(ctx context.Context, id string) (model.BillingSubscriptionResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/preapproval/"+id, nil)
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	return c.do(req)
}

func (c *Client) CancelSubscription(ctx context.Context, id, idempotencyKey string) (model.BillingSubscriptionResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/preapproval/"+id, strings.NewReader(`{"status":"canceled"}`))
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", idempotencyKey)
	return c.do(req)
}

func (c *Client) do(req *http.Request) (model.BillingSubscriptionResult, error) {
	response, err := c.http.Do(req)
	if err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, 1<<20)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, limited)
		return model.BillingSubscriptionResult{}, fmt.Errorf("mercado pago returned %d", response.StatusCode)
	}
	var out preapproval
	if err = json.NewDecoder(limited).Decode(&out); err != nil {
		return model.BillingSubscriptionResult{}, err
	}
	if out.ID == "" || out.ExternalReference == "" {
		return model.BillingSubscriptionResult{}, errors.New("mercado pago returned an incomplete subscription")
	}
	amount := int64(0)
	if out.AutoRecurring.Amount != "" {
		amount, err = decimalMinor(out.AutoRecurring.Amount.String())
		if err != nil {
			return model.BillingSubscriptionResult{}, err
		}
	}
	return model.BillingSubscriptionResult{ID: out.ID, Status: out.Status, ExternalReference: out.ExternalReference, CheckoutURL: out.InitPoint, NextPaymentDate: out.NextPaymentDate, AmountMinor: amount}, nil
}

func ValidateSignature(header, requestID, resourceID, secret string, now time.Time) bool {
	parts := map[string]string{}
	for _, raw := range strings.Split(header, ",") {
		pair := strings.SplitN(strings.TrimSpace(raw), "=", 2)
		if len(pair) == 2 {
			parts[pair[0]] = pair[1]
		}
	}
	ts, signature := parts["ts"], parts["v1"]
	provided, err := hex.DecodeString(signature)
	if err != nil || ts == "" || requestID == "" || resourceID == "" || secret == "" || len(provided) != sha256.Size {
		return false
	}
	timestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || timestamp <= 0 {
		return false
	}
	// Mercado Pago documents both Unix seconds and milliseconds in x-signature.
	// Convert only for freshness checks; the HMAC must retain the original ts.
	var sentAt time.Time
	switch len(ts) {
	case 10:
		sentAt = time.Unix(timestamp, 0)
	case 13:
		sentAt = time.UnixMilli(timestamp)
	default:
		return false
	}
	if sentAt.Before(now.Add(-5*time.Minute)) || sentAt.After(now.Add(time.Minute)) {
		return false
	}
	manifest := "id:" + strings.ToLower(resourceID) + ";request-id:" + requestID + ";ts:" + ts + ";"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(manifest))
	return subtle.ConstantTimeCompare(provided, mac.Sum(nil)) == 1
}
