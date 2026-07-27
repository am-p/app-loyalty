package service

import (
	"errors"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

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

func LoginUser(pool *pgxpool.Pool, req model.LoginRequest) (model.User, error) {
	user, err := repository.GetUserByEmail(pool, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrInvalidCredentials
		}
		return model.User{}, err
	}

	if user.PasswordHash == nil {
		return model.User{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		return model.User{}, ErrInvalidCredentials
	}

	return user, nil
}
