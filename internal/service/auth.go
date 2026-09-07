package service

import (
	"context"
	"crypto/rand"
	"strings"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/model"

	"golang.org/x/crypto/bcrypt"
)

func (s *Service) RegisterCustomer(ctx context.Context, req model.RegisterCustomerRequest) (model.AuthData, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil || !validPassword(req.Password) {
		return model.AuthData{}, ErrInvalidRequest
	}
	name, err := cleanName(req.Name, 120)
	if err != nil {
		return model.AuthData{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return model.AuthData{}, err
	}
	provisional := make([]byte, 32)
	if _, err = rand.Read(provisional); err != nil {
		return model.AuthData{}, err
	}
	// The final QR is derived after PostgreSQL assigns the immutable user id.
	u, err := s.Repo.CreateCustomer(ctx, email, string(hash), name, provisional, func(id int64) []byte { _, finalHash := s.QRForUser(id); return finalHash })
	if err != nil {
		return model.AuthData{}, err
	}
	token, err := s.Tokens.Generate(u.ID, u.AccountType)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session(token), User: u}, nil
}

func (s *Service) Login(ctx context.Context, req model.LoginRequest) (model.AuthData, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return model.AuthData{}, ErrInvalidCredentials
	}
	u, err := s.Repo.GetUserByEmail(ctx, email)
	if err != nil {
		return model.AuthData{}, ErrInvalidCredentials
	}
	if !u.User.Active || u.PasswordHash == nil || bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(req.Password)) != nil {
		return model.AuthData{}, ErrInvalidCredentials
	}
	token, err := s.Tokens.Generate(u.User.ID, u.User.AccountType)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session(token), User: u.User}, nil
}

func (s *Service) LoginGoogle(ctx context.Context, idToken string) (model.AuthData, error) {
	if strings.TrimSpace(idToken) == "" {
		return model.AuthData{}, ErrInvalidRequest
	}
	googleID, email, name, err := auth.VerifyGoogleToken(ctx, idToken)
	if err != nil {
		return model.AuthData{}, ErrInvalidCredentials
	}
	email, err = normalizeEmail(email)
	if err != nil {
		return model.AuthData{}, ErrInvalidCredentials
	}
	name, err = cleanName(name, 120)
	if err != nil {
		return model.AuthData{}, ErrInvalidCredentials
	}
	provisional := make([]byte, 32)
	if _, err = rand.Read(provisional); err != nil {
		return model.AuthData{}, err
	}
	u, err := s.Repo.LoginGoogle(ctx, googleID, email, name, provisional, func(id int64) []byte { _, hash := s.QRForUser(id); return hash })
	if err != nil {
		return model.AuthData{}, err
	}
	token, err := s.Tokens.Generate(u.ID, u.AccountType)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session(token), User: u}, nil
}

func session(token string) model.Session {
	return model.Session{AccessToken: token, TokenType: "Bearer", ExpiresIn: 86400}
}
