package service

import (
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func RegisterUser(pool *pgxpool.Pool, req model.RegisterRequest) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	hashStr := string(hash)
	user := model.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: &hashStr,
		Role:         "CLIENTE_FINAL",
	}

	return repository.InsertUser(pool, user)
}
