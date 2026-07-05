package service

import (
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func RegisterCustomer(pool *pgxpool.Pool, customer model.Customer) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(customer.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	customer.Password = string(hash)
	return repository.InsertQuery(pool, customer)
}
