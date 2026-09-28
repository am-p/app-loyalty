package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"clientesFrecuentes/internal/repository"
)

var ErrReferralValidationUnavailable = errors.New("referral validation unavailable")

type ReferralValidationRequest struct {
	Code        string `json:"code"`
	ProgramType string `json:"program_type,omitempty"`
}

func (s *Service) ValidateReferralCode(ctx context.Context, req ReferralValidationRequest) (repository.ReferralValidation, error) {
	if req.ProgramType != "" && req.ProgramType != "SELLOS" && req.ProgramType != "PUNTOS" {
		return repository.ReferralValidation{}, ErrInvalidRequest
	}
	code, err := repository.NormalizeReferralCode(req.Code)
	if err != nil {
		return repository.ReferralValidation{}, err
	}
	if s.Repo == nil || s.Repo.Pool == nil {
		return repository.ReferralValidation{}, ErrReferralValidationUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := s.Repo.ValidateReferralCode(bounded, code, req.ProgramType)
	if err != nil && !errors.Is(err, repository.ErrReferralCodeInvalid) {
		return repository.ReferralValidation{}, fmt.Errorf("%w: %w", ErrReferralValidationUnavailable, err)
	}
	return out, err
}
