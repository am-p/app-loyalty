package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"clientesFrecuentes/internal/model"
)

func NormalizeStoreSearch(search string) (string, error) {
	search = strings.TrimSpace(search)
	if !utf8.ValidString(search) || strings.ContainsRune(search, 0) || utf8.RuneCountInString(search) > 200 {
		return "", ErrInvalidRequest
	}
	return search, nil
}

func (s *Service) CustomerStoreLocations(ctx context.Context, actorID int64, search string) ([]model.StoreLocation, error) {
	search, err := NormalizeStoreSearch(search)
	if err != nil {
		return nil, err
	}
	items, err := s.Repo.ListCustomerStoreLocations(ctx, actorID, search)
	if err != nil {
		return nil, err
	}
	signed := map[string]string{}
	for i := range items {
		key := items[i].LogoObjectKey
		if s.Media != nil && key != "" {
			url, found := signed[key]
			if !found {
				url, _ = s.Media.SignedGet(ctx, key, s.Config.MediaURLTTL)
				signed[key] = url
			}
			items[i].ImageURL = url
		}
	}
	return items, nil
}

func (s *Service) SetCustomerStoreFavorite(ctx context.Context, actorID, branchID int64, favorite bool) (model.StoreFavorite, error) {
	if branchID <= 0 {
		return model.StoreFavorite{}, ErrInvalidRequest
	}
	return s.Repo.SetCustomerStoreFavorite(ctx, actorID, branchID, favorite)
}
