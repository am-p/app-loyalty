package service

import (
	"context"
	"encoding/json"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"

	"github.com/google/uuid"
)

func (s *Service) Preview(ctx context.Context, actorID int64, req model.MovementPreviewRequest) (model.Preview, error) {
	if req.Operation != "ACUMULACION" && req.Operation != "CANJE" || req.BranchID < 1 {
		return model.Preview{}, ErrInvalidRequest
	}
	if req.Operation == "ACUMULACION" && req.BenefitID != nil {
		return model.Preview{}, ErrInvalidRequest
	}
	if req.Operation == "CANJE" && (req.BenefitID == nil || *req.BenefitID < 1) {
		return model.Preview{}, ErrInvalidRequest
	}
	if req.Operation == "CANJE" && req.PointsAmount != nil {
		return model.Preview{}, ErrInvalidRequest
	}
	token, err := s.resolveMovementIdentity(req.QRToken, req.CustomerCode)
	if err != nil {
		return model.Preview{}, err
	}
	req.QRToken = token
	req.CustomerCode = ""
	return s.Repo.CreatePreview(ctx, actorID, req, s.QRHash(req.QRToken), Fingerprint(req))
}

func (s *Service) ConfirmAccumulation(ctx context.Context, actorID int64, key, requestID string, req model.ConfirmAccumulationRequest) (repository.IdempotentResult, error) {
	if req.BranchID < 1 {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	if _, err := uuid.Parse(req.PreviewID); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	token, err := s.resolveMovementIdentity(req.QRToken, req.CustomerCode)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	req.QRToken = token
	req.CustomerCode = ""
	return s.confirm(ctx, repository.ConfirmInput{ActorID: actorID, Key: key, Fingerprint: Fingerprint(req), PreviewID: req.PreviewID, QRHash: s.QRHash(req.QRToken), BranchID: req.BranchID, Operation: "ACUMULACION"}, requestID)
}

func (s *Service) ConfirmRedemption(ctx context.Context, actorID int64, key, requestID string, req model.ConfirmRedemptionRequest) (repository.IdempotentResult, error) {
	if req.BranchID < 1 || req.BenefitID < 1 {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	if _, err := uuid.Parse(req.PreviewID); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	token, err := s.resolveMovementIdentity(req.QRToken, req.CustomerCode)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	req.QRToken = token
	req.CustomerCode = ""
	return s.confirm(ctx, repository.ConfirmInput{ActorID: actorID, Key: key, Fingerprint: Fingerprint(req), PreviewID: req.PreviewID, QRHash: s.QRHash(req.QRToken), BranchID: req.BranchID, BenefitID: &req.BenefitID, Operation: "CANJE"}, requestID)
}

func (s *Service) confirm(ctx context.Context, in repository.ConfirmInput, requestID string) (repository.IdempotentResult, error) {
	if _, err := uuid.Parse(in.Key); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	var result repository.IdempotentResult
	err := retry(ctx, func() error {
		var e error
		result, e = s.Repo.ConfirmMovement(ctx, in, func(m model.Movement) ([]byte, error) {
			return json.Marshal(web.Envelope[model.Movement]{Data: m, RequestID: requestID})
		})
		return e
	})
	return result, err
}
