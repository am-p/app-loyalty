package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"clientesFrecuentes/internal/emailcode"
	"clientesFrecuentes/internal/mailer"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
)

func (s *Service) registrationVerification(email, method, merchantKey string) (string, []byte, time.Time, *model.EmailMessage, error) {
	if method != "" && method != "code" {
		return "", nil, time.Time{}, nil, ErrInvalidRequest
	}
	if !s.Config.EmailVerificationRequired {
		return "", nil, time.Time{}, nil, nil
	}
	if method == "" {
		token, hash, err := identityToken()
		if err != nil {
			return "", nil, time.Time{}, nil, err
		}
		message := mailer.VerificationMessage(s.Config.PublicAppURL, email, token)
		return "", hash, s.Now().Add(24 * time.Hour), &message, nil
	}
	code, err := emailcode.Generate()
	if err != nil {
		return "", nil, time.Time{}, nil, err
	}
	token, _, err := identityToken()
	if err != nil {
		return "", nil, time.Time{}, nil, err
	}
	if merchantKey != "" {
		token = emailcode.MerchantRegistrationToken(s.Config.OutboxEncryptionKey, email, merchantKey)
	}
	message := mailer.VerificationCodeMessage(email, code)
	message.RegistrationHash = emailcode.RegistrationHash(token)
	message.RegistrationExpires = s.Now().Add(30 * time.Minute)
	return token, emailcode.Hash(s.Config.OutboxEncryptionKey, email, code), s.Now().Add(10 * time.Minute), &message, nil
}

func (s *Service) RequestEmailVerificationCode(ctx context.Context, req model.EmailRequest) error {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return ErrInvalidRequest
	}
	if err = s.checkCaptcha(ctx, "identity"); err != nil {
		return err
	}
	// Verification remains available for previously pending accounts even if new
	// registrations no longer require it in this environment.
	code, err := emailcode.Generate()
	if err != nil {
		return err
	}
	return s.Repo.EnqueueVerificationCode(ctx, email, code, s.Now().Add(10*time.Minute), mailer.VerificationCodeMessage(email, code))
}
func (s *Service) ConfirmEmailVerificationCode(ctx context.Context, req model.VerificationCodeConfirmRequest) (model.VerificationCodeData, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil || !emailcode.Valid(req.Code) || len(req.RegistrationToken) > 128 {
		return model.VerificationCodeData{}, ErrIdentityToken
	}
	identity, automatic, err := s.Repo.ConfirmVerificationCode(ctx, email, emailcode.Hash(s.Config.OutboxEncryptionKey, email, req.Code), req.RegistrationToken, s.Now())
	if errors.Is(err, repository.ErrIdentityTokenInvalid) {
		return model.VerificationCodeData{}, ErrIdentityToken
	}
	if err != nil {
		return model.VerificationCodeData{}, err
	}
	result := model.VerificationCodeData{Verified: true}
	if !automatic {
		return result, nil
	}
	user, err := s.Repo.GetAuthUserByID(ctx, identity.UserID)
	if err != nil {
		slog.Warn("email verified; automatic session unavailable")
		return result, nil
	}
	if user.User.AuthVersion != identity.AuthVersion || user.User.Email != email || !user.User.EmailVerified || !user.User.Active {
		return result, nil
	}
	session, err := s.issueSession(ctx, user.User)
	if err != nil {
		slog.Warn("email verified; automatic session unavailable")
		return result, nil
	}
	result.Session = &session
	result.User = &user.User
	return result, nil
}
