package service

import (
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func RegisterShop(pool *pgxpool.Pool, shop model.Shop) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(shop.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	shop.Password = string(hash)
	return repository.InsertShop(pool, shop)
}
