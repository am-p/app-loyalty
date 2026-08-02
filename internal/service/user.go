package service

import (
	"errors"

	"clientesFrecuentes/internal/auth"
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

func LoginWithGoogle(pool *pgxpool.Pool, idToken string) (model.User, error) {
	googleID, email, name, err := auth.VerifyGoogleToken(idToken)
	if err != nil {
		return model.User{}, err
	}

	user, err := repository.GetUserByGoogleID(pool, googleID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, err
	}

	user, err = repository.GetUserByEmail(pool, email)
	if err == nil {
		if err := repository.LinkGoogleID(pool, user.ID, googleID); err != nil {
			return model.User{}, err
		}
		user.GoogleID = &googleID
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, err
	}

	newUser := model.User{
		Name:         name,
		Email:        email,
		GoogleID:     &googleID,
		PasswordHash: nil,
		Role:         "CLIENTE_FINAL",
	}

	id, err := repository.InsertUser(pool, newUser)
	if err != nil {
		return model.User{}, err
	}
	newUser.ID = id

	return newUser, nil
}
