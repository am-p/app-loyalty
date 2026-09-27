package model

// BillingInvoice is the provider's authorized-payment resource. Amounts are ARS minor units.
type BillingInvoice struct {
	ID, SubscriptionID, PaymentID, Currency, PaymentStatus string
	AmountMinor                                            int64
}

// BillingPayment is fetched independently so a refunded payment cannot accrue rewards.
type BillingPayment struct {
	ID, Status, Currency       string
	AmountMinor, RefundedMinor int64
}
