package service

import (
	"context"
	"net/url"
	"strings"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/web"
)

func (s *Service) CurrentUser(ctx context.Context, actorID int64) (model.CurrentUser, error) {
	return s.Repo.GetCurrentUser(ctx, actorID)
}

func (s *Service) UpdateCurrentUser(ctx context.Context, actorID int64, expectedVersion int, req model.UpdateAccountRequest) (model.CurrentUser, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 120 || expectedVersion < 1 {
		return model.CurrentUser{}, ErrInvalidRequest
	}
	if !normalizeOptional(&req.LastName, 120) || !normalizeOptional(&req.Alias, 80) || !normalizeOptional(&req.PhotoURL, 2048) {
		return model.CurrentUser{}, ErrInvalidRequest
	}
	if req.PhotoURL != nil && *req.PhotoURL != "" {
		parsed, err := url.ParseRequestURI(*req.PhotoURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return model.CurrentUser{}, ErrInvalidRequest
		}
	}
	return s.Repo.UpdateAccount(ctx, actorID, expectedVersion, req)
}

func (s *Service) ExportCurrentUser(ctx context.Context, actorID int64) (model.AccountExport, error) {
	return s.Repo.ExportAccount(ctx, actorID)
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
	return items, pagination(page, size, total), err
}

func (s *Service) CardMovements(ctx context.Context, actorID, cardID int64, page, size int) ([]model.Movement, web.Pagination, error) {
	items, total, err := s.Repo.ListCustomerMovements(ctx, actorID, cardID, page, size)
	return items, pagination(page, size, total), err
}
