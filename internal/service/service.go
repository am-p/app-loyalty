package service

import (
	"errors"
	"time"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidRequest     = errors.New("invalid request")
	ErrForbidden          = errors.New("forbidden")
	ErrDemoDisabled       = errors.New("demo signup disabled")
	ErrDemoAccess         = errors.New("demo access denied")
	ErrEmailUnverified    = errors.New("email unverified")
	ErrIdentityToken      = errors.New("identity token invalid")
)

type Service struct {
	Repo   *repository.Repository
	Tokens *auth.Tokens
	Config config.Config
	Now    func() time.Time
}

func New(repo *repository.Repository, tokens *auth.Tokens, cfg config.Config) *Service {
	return &Service{Repo: repo, Tokens: tokens, Config: cfg, Now: time.Now}
}
