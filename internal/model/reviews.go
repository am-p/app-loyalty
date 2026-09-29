package model

import "time"

type ReviewSettingsInput struct {
	Enabled           bool    `json:"enabled"`
	PurchaseThreshold int     `json:"purchase_threshold"`
	Message           string  `json:"message"`
	DestinationType   string  `json:"destination_type"`
	GooglePlaceID     *string `json:"google_place_id"`
	ManualReviewURL   *string `json:"manual_review_url"`
}
type ReviewSettings struct {
	ReviewSettingsInput
	BranchID        int64   `json:"branch_id"`
	Version         int     `json:"version"`
	PlacesAvailable bool    `json:"places_available"`
	ReviewURL       *string `json:"review_url"`
}
type ReviewAttribution struct {
	Provider    string `json:"provider"`
	ProviderURI string `json:"provider_uri"`
}

type ReviewPlace struct {
	Attributions []ReviewAttribution `json:"attributions,omitempty"`
	PlaceID      string              `json:"place_id"`
	Name         string              `json:"name"`
	Address      string              `json:"address"`
	ReviewURL    *string             `json:"review_url"`
}
type ReviewMetrics struct {
	Shown   int64 `json:"shown"`
	Skipped int64 `json:"skipped"`
	Clicked int64 `json:"clicked"`
}
type ReviewInvitation struct {
	ID          string    `json:"id"`
	CardID      int64     `json:"card_id"`
	BrandID     int64     `json:"brand_id"`
	BranchID    int64     `json:"branch_id"`
	BranchName  string    `json:"branch_name"`
	OperationID string    `json:"operation_id"`
	Message     string    `json:"message"`
	ReviewURL   string    `json:"review_url"`
	OccurredAt  time.Time `json:"occurred_at"`
}
type ReviewCandidate struct {
	Invitation ReviewInvitation
	Settings   ReviewSettings
}
type ReviewReservation struct {
	Invitation       ReviewInvitation `json:"invitation"`
	ReservationToken string           `json:"reservation_token"`
	ExpiresAt        time.Time        `json:"expires_at"`
}
type ReviewEventRequest struct {
	ReservationToken string `json:"reservation_token"`
	Event            string `json:"event"`
}
type ReviewEvents struct {
	ID      string `json:"id"`
	Shown   bool   `json:"shown"`
	Skipped bool   `json:"skipped"`
	Clicked bool   `json:"clicked"`
}
type ReviewProgressExport struct {
	BranchID  int64 `json:"branch_id"`
	Purchases int64 `json:"purchases"`
}
type ReviewInvitationExport struct {
	ID          string     `json:"id"`
	BranchID    int64      `json:"branch_id"`
	CardID      *int64     `json:"card_id"`
	OperationID *string    `json:"operation_id"`
	OccurredAt  time.Time  `json:"occurred_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
	ShownAt     *time.Time `json:"shown_at"`
	SkippedAt   *time.Time `json:"skipped_at"`
	ClickedAt   *time.Time `json:"clicked_at"`
}
