package model

type VerificationCodeConfirmRequest struct {
	Email             string `json:"email"`
	Code              string `json:"code"`
	RegistrationToken string `json:"registration_token,omitempty"`
}
type VerificationCodeData struct {
	Verified bool     `json:"verified"`
	Session  *Session `json:"session,omitempty"`
	User     *User    `json:"user,omitempty"`
}
