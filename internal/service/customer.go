package service

import (
	"context"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/web"
)

func (s *Service) CurrentUser(ctx context.Context, actorID int64) (model.CurrentUser, error) {
	return s.Repo.GetCurrentUser(ctx, actorID)
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
