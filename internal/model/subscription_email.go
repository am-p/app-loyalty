package model

import "time"

// SubscriptionConfirmationDetails is the encrypted snapshot of an authorized
// subscription. It never asserts that a recurring installment has been paid.
type SubscriptionConfirmationDetails struct {
	BrandID            int64      `json:"brand_id"`
	ProviderID         string     `json:"provider_id"`
	BrandName          string     `json:"brand_name"`
	OwnerName          string     `json:"owner_name"`
	ProgramType        string     `json:"program_type"`
	ActiveBranches     int64      `json:"active_branches"`
	MonthlyAmountMinor int64      `json:"monthly_amount_minor"`
	Currency           string     `json:"currency"`
	TrialMonths        int        `json:"trial_months"`
	ConfirmedAt        time.Time  `json:"confirmed_at"`
	NextPaymentDate    *time.Time `json:"next_payment_date,omitempty"`
}
