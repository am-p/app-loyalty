package handler

import "time"

// Policies count requests (not only failures). Legacy and v1 password login
// share their IP/email keys; preview, accumulation and redemption share actor keys.
const (
	loginAttempts          = 10
	loginWindow            = 10 * time.Minute
	customerSignupAttempts = 10
	customerSignupWindow   = 10 * time.Minute
	merchantIPAttempts     = 5
	merchantIPWindow       = 10 * time.Minute
	merchantEmailAttempts  = 3
	merchantEmailWindow    = 30 * time.Minute
	movementAttempts       = 60
	movementWindow         = time.Minute
)
