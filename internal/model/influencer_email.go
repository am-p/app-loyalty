package model

import "time"

// Conditions at registration, encrypted in the outbox. Campaign edits cannot
// silently change a delayed welcome email. These are not billing attributions.
type InfluencerWelcomeDetails struct {
	InfluencerID    int64     `json:"influencer_id"`
	CodeID          int64     `json:"code_id"`
	Name            string    `json:"name"`
	Code            string    `json:"code"`
	CampaignID      int64     `json:"campaign_id"`
	CampaignName    string    `json:"campaign_name"`
	ProgramTypes    []string  `json:"program_types"`
	DiscountBPS     int       `json:"discount_bps"`
	DiscountCharges int       `json:"discount_charges"`
	RewardBPS       int       `json:"reward_bps"`
	RewardCharges   int       `json:"reward_charges"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	Active          bool      `json:"active"`
}
