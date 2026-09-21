package service

import (
	"context"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/web"

	"github.com/google/uuid"
)

func (s *Service) CurrentUser(ctx context.Context, actorID int64) (model.CurrentUser, error) {
	return s.Repo.GetCurrentUser(ctx, actorID)
}

func (s *Service) UpdateCurrentUser(ctx context.Context, actorID int64, expectedVersion int, req model.UpdateAccountRequest) (model.CurrentUser, error) {
	if expectedVersion < 1 || (!req.Name.Set && !req.LastName.Set && !req.Alias.Set) {
		return model.CurrentUser{}, ErrInvalidRequest
	}
	if req.Name.Set && (req.Name.Value == nil || !normalizePatch(&req.Name, 120) || *req.Name.Value == "") {
		return model.CurrentUser{}, ErrInvalidRequest
	}
	if !normalizePatch(&req.LastName, 120) || !normalizePatch(&req.Alias, 80) {
		return model.CurrentUser{}, ErrInvalidRequest
	}
	return s.Repo.UpdateAccount(ctx, actorID, expectedVersion, req)
}

func normalizePatch(value *model.OptionalString, limit int) bool {
	if !value.Set || value.Value == nil {
		return true
	}
	normalized := strings.TrimSpace(*value.Value)
	if len(normalized) > limit {
		return false
	}
	value.Value = &normalized
	return true
}

func (s *Service) ExportCurrentUser(ctx context.Context, actorID int64) (model.AccountExport, error) {
	return s.Repo.ExportAccount(ctx, actorID)
}

func (s *Service) AnonymizeCurrentUser(ctx context.Context, actorID int64, authTime time.Time, expectedVersion int, req model.AnonymizeAccountRequest) (model.Anonymization, error) {
	if req.Confirmation != "ANONIMIZAR" {
		return model.Anonymization{}, ErrInvalidRequest
	}
	now := s.Now().UTC()
	if authTime.IsZero() || now.Sub(authTime) > 10*time.Minute {
		return model.Anonymization{}, ErrRecentAuthRequired
	}
	deletedAt, err := s.Repo.AnonymizeAccount(ctx, actorID, expectedVersion)
	if err != nil {
		return model.Anonymization{}, err
	}
	return model.Anonymization{RequestID: uuid.NewString(), Status: "COMPLETADA", AccessRevoked: true, LedgerPreserved: true, RequestedAt: deletedAt}, nil
}

func normalizeOptional(value **string, limit int) bool {
	if *value == nil {
		return true
	}
	normalized := strings.TrimSpace(**value)
	if len(normalized) > limit {
		return false
	}
	*value = &normalized
	return true
}

func (s *Service) Customer(ctx context.Context, actorID int64) (model.Customer, error) {
	u, err := s.Repo.GetCustomer(ctx, actorID)
	if err != nil {
		return model.Customer{}, err
	}
	token, _ := s.QRForUser(u.ID)
	return model.Customer{ID: u.ID, Email: u.Email, Name: u.Name, QRToken: token}, nil
}

func (s *Service) Cards(ctx context.Context, actorID int64, page, size int) ([]model.Card, web.Pagination, error) {
	items, total, err := s.Repo.ListCards(ctx, actorID, page, size)
	if err == nil && s.Media != nil {
		for i := range items {
			if items[i].BrandLogoObjectKey == "" {
				continue
			}
			// A temporary signing failure must not hide the customer's cards;
			// the UI keeps its storefront fallback and the next live refresh retries.
			items[i].BrandLogo, _ = s.Media.SignedGet(ctx, items[i].BrandLogoObjectKey, s.Config.MediaURLTTL)
		}
	}
	return items, pagination(page, size, total), err
}

func (s *Service) CardMovements(ctx context.Context, actorID, cardID int64, page, size int) ([]model.Movement, web.Pagination, error) {
	items, total, err := s.Repo.ListCustomerMovements(ctx, actorID, cardID, page, size)
	return items, pagination(page, size, total), err
}
