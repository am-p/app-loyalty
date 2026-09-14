package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const refreshLifetime = 30 * 24 * time.Hour

type sessionCredentials struct {
	id        string
	raw       string
	hash      []byte
	expiresAt time.Time
}

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
	session, err := s.issueSession(ctx, u)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session, User: u}, nil
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
	session, err := s.issueSession(ctx, u.User)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session, User: u.User}, nil
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
	session, err := s.issueSession(ctx, u)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session, User: u}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (model.AuthData, error) {
	if len(refreshToken) < 40 || len(refreshToken) > 256 {
		return model.AuthData{}, ErrInvalidCredentials
	}
	credentials, err := newSessionCredentials()
	if err != nil {
		return model.AuthData{}, err
	}
	oldHash := sha256.Sum256([]byte(refreshToken))
	u, err := s.Repo.RotateSession(ctx, oldHash[:], credentials.id, credentials.hash, credentials.expiresAt)
	if err != nil {
		if err == repository.ErrNotFound {
			return model.AuthData{}, ErrInvalidCredentials
		}
		return model.AuthData{}, err
	}
	session, err := s.session(u, credentials)
	if err != nil {
		return model.AuthData{}, err
	}
	return model.AuthData{Session: session, User: u}, nil
}

func (s *Service) Logout(ctx context.Context, userID int64, sessionID string) error {
	if err := s.Repo.RevokeSession(ctx, userID, sessionID); err != nil && err != repository.ErrNotFound {
		return err
	}
	return nil
}

func (s *Service) issueSession(ctx context.Context, u model.User) (model.Session, error) {
	credentials, err := newSessionCredentials()
	if err != nil {
		return model.Session{}, err
	}
	if err = s.Repo.CreateSession(ctx, credentials.id, u.ID, credentials.hash, credentials.expiresAt); err != nil {
		return model.Session{}, err
	}
	return s.session(u, credentials)
}

func (s *Service) session(u model.User, credentials sessionCredentials) (model.Session, error) {
	token, err := s.Tokens.GenerateForSession(u.ID, u.AccountType, credentials.id)
	if err != nil {
		return model.Session{}, err
	}
	return model.Session{AccessToken: token, RefreshToken: credentials.raw, TokenType: "Bearer", ExpiresIn: 900}, nil
}

func newSessionCredentials() (sessionCredentials, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return sessionCredentials{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(encoded))
	return sessionCredentials{id: uuid.NewString(), raw: encoded, hash: hash[:], expiresAt: time.Now().UTC().Add(refreshLifetime)}, nil
}
