package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"

	"github.com/google/uuid"
)

func cleanInvitation(req *model.CreateInvitationRequest) error {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return ErrInvalidRequest
	}
	req.Email = email
	req.Role = strings.ToUpper(strings.TrimSpace(req.Role))
	if req.Role != "ADMINISTRADOR" && req.Role != "OPERADOR" {
		return ErrInvalidRequest
	}
	if req.Role == "OPERADOR" && len(req.BranchIDs) == 0 {
		return ErrInvalidRequest
	}
	seen := make(map[int64]struct{}, len(req.BranchIDs))
	for _, id := range req.BranchIDs {
		if id < 1 {
			return ErrInvalidRequest
		}
		if _, ok := seen[id]; ok {
			return ErrInvalidRequest
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (s *Service) Invitations(ctx context.Context, actorID, brandID int64) ([]model.BrandInvitation, error) {
	return s.Repo.ListInvitations(ctx, actorID, brandID)
}
func (s *Service) CreateInvitation(ctx context.Context, actorID, brandID int64, key, requestID string, req model.CreateInvitationRequest) (repository.IdempotentResult, error) {
	if err := cleanInvitation(&req); err != nil {
		return repository.IdempotentResult{}, err
	}
	if _, err := uuid.Parse(key); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	token, hash, err := identityToken()
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	fingerprint := KeyedFingerprint(s.Config.QRPepper, struct {
		BrandID int64
		Request model.CreateInvitationRequest
	}{brandID, req})
	var result repository.IdempotentResult
	err = retry(ctx, func() error {
		var retryErr error
		result, retryErr = s.Repo.CreateInvitation(ctx, actorID, brandID, key, fingerprint, req, token, hash, s.Now().Add(72*time.Hour), func(item model.BrandInvitation) ([]byte, error) {
			return json.Marshal(web.Envelope[model.BrandInvitation]{Data: item, RequestID: requestID})
		})
		return retryErr
	})
	return result, err
}
func (s *Service) RevokeInvitation(ctx context.Context, actorID, brandID int64, id string) error {
	return s.Repo.RevokeInvitation(ctx, actorID, brandID, id)
}
func (s *Service) ResendInvitation(ctx context.Context, actorID, brandID int64, id string) (model.BrandInvitation, error) {
	token, hash, err := identityToken()
	if err != nil {
		return model.BrandInvitation{}, err
	}
	return s.Repo.ResendInvitation(ctx, actorID, brandID, id, token, hash, s.Now().Add(72*time.Hour))
}
func invitationHash(token string) ([]byte, error) {
	if len(token) < 40 || len(token) > 256 {
		return nil, ErrIdentityToken
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:], nil
}
func (s *Service) PublicInvitation(ctx context.Context, token string) (model.PublicInvitation, error) {
	h, e := invitationHash(token)
	if e != nil {
		return model.PublicInvitation{}, e
	}
	x, e := s.Repo.PublicInvitation(ctx, h, s.Now())
	if e == repository.ErrInvitationInvalid {
		return x, ErrIdentityToken
	}
	return x, e
}
func (s *Service) AcceptInvitation(ctx context.Context, actorID int64, token string) (model.StaffMember, error) {
	h, e := invitationHash(token)
	if e != nil {
		return model.StaffMember{}, e
	}
	x, e := s.Repo.AcceptInvitation(ctx, actorID, h, s.Now())
	if e == repository.ErrInvitationInvalid {
		return x, ErrIdentityToken
	}
	return x, e
}
func (s *Service) Staff(ctx context.Context, actorID, brandID int64) ([]model.StaffMember, error) {
	return s.Repo.ListStaff(ctx, actorID, brandID)
}
func (s *Service) StaffMember(ctx context.Context, actorID, brandID, membershipID int64) (model.StaffMember, error) {
	return s.Repo.GetStaffMember(ctx, actorID, brandID, membershipID)
}
func (s *Service) UpdateStaff(ctx context.Context, actorID, brandID, membershipID int64, version int, req model.UpdateStaffRequest) (model.StaffMember, error) {
	if version < 1 || (req.Role == nil && req.BranchIDs == nil) {
		return model.StaffMember{}, ErrInvalidRequest
	}
	if req.Role != nil {
		role := strings.ToUpper(strings.TrimSpace(*req.Role))
		if role != "ADMINISTRADOR" && role != "OPERADOR" {
			return model.StaffMember{}, ErrInvalidRequest
		}
		req.Role = &role
	}
	if req.BranchIDs != nil {
		seen := map[int64]struct{}{}
		for _, id := range *req.BranchIDs {
			if id < 1 {
				return model.StaffMember{}, ErrInvalidRequest
			}
			if _, ok := seen[id]; ok {
				return model.StaffMember{}, ErrInvalidRequest
			}
			seen[id] = struct{}{}
		}
	}
	return s.Repo.UpdateStaff(ctx, actorID, brandID, membershipID, version, req)
}
func (s *Service) DeleteStaff(ctx context.Context, actorID, brandID, membershipID int64, version int) error {
	if version < 1 {
		return ErrInvalidRequest
	}
	return s.Repo.DeleteStaff(ctx, actorID, brandID, membershipID, version)
}
