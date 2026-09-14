package service

import (
	"context"
	"encoding/json"
	"strings"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) RegisterDemoMerchant(ctx context.Context, key, requestID string, req model.RegisterDemoMerchantRequest) (repository.IdempotentResult, error) {
	if !s.Config.DemoSignupEnabled {
		return repository.IdempotentResult{}, ErrDemoDisabled
	}
	if len(req.AccessCode) < 12 || len(req.AccessCode) > 128 {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	if bcrypt.CompareHashAndPassword([]byte(s.Config.DemoAccessCodeHash), []byte(req.AccessCode)) != nil {
		return repository.IdempotentResult{}, ErrDemoAccess
	}
	if _, err := uuid.Parse(key); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	email, err := normalizeEmail(req.Email)
	if err != nil || !validPassword(req.Password) {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	owner, err := cleanName(req.OwnerName, 120)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	brand, err := cleanName(req.BrandName, 120)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	branch, err := cleanName(req.BranchName, 120)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	if req.BranchAddress != nil {
		v := strings.TrimSpace(*req.BranchAddress)
		if len([]rune(v)) > 240 {
			return repository.IdempotentResult{}, ErrInvalidRequest
		}
		req.BranchAddress = &v
	}
	programType, err := normalizeProgramType(req.ProgramType)
	if err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	// This fingerprint is persisted. Key it so a database leak cannot turn the
	// low-entropy password field into a fast, offline SHA-256 oracle.
	fingerprint := KeyedFingerprint(s.Config.QRPepper, struct {
		Email, Password, OwnerName, BrandName, BranchName string
		BranchAddress                                     *string
		ProgramType                                       string
	}{email, req.Password, owner, brand, branch, req.BranchAddress, programType})
	credentials, err := newSessionCredentials()
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	var result repository.IdempotentResult
	err = retry(ctx, func() error {
		var e error
		result, e = s.Repo.CreateDemoMerchant(ctx, key, fingerprint, email, string(passwordHash), owner, brand, branch, req.BranchAddress, programType, credentials.id, credentials.hash, credentials.expiresAt, credentials.authTime, func(u model.User, m model.MerchantContext) ([]byte, error) {
			merchantSession, e := s.session(u, credentials)
			if e != nil {
				return nil, e
			}
			// Idempotency persistence must never retain the bearer-equivalent
			// refresh secret. It is injected only into the in-memory response.
			merchantSession.RefreshToken = ""
			return json.Marshal(web.Envelope[model.DemoMerchantData]{Data: model.DemoMerchantData{Session: merchantSession, User: u, Merchant: m}, RequestID: requestID})
		})
		return e
	})
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	var envelope web.Envelope[model.DemoMerchantData]
	if err = json.Unmarshal(result.Body, &envelope); err != nil {
		return repository.IdempotentResult{}, err
	}
	if result.Replayed {
		envelope.Data.Session, err = s.issueSession(ctx, envelope.Data.User)
		if err != nil {
			return repository.IdempotentResult{}, err
		}
	} else {
		envelope.Data.Session.RefreshToken = credentials.raw
	}
	result.Body, err = json.Marshal(envelope)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	return result, nil
}

func normalizeProgramType(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != "SELLOS" && value != "PUNTOS" {
		return "", ErrInvalidRequest
	}
	return value, nil
}

func (s *Service) ListBrands(ctx context.Context, actorID int64) ([]model.MerchantContext, error) {
	return s.Repo.ListMerchantContexts(ctx, actorID)
}

func (s *Service) Brand(ctx context.Context, actorID, brandID int64) (model.MerchantContext, error) {
	return s.Repo.GetMerchantContext(ctx, actorID, brandID)
}

func (s *Service) Benefits(ctx context.Context, actorID, brandID int64) ([]model.Benefit, error) {
	return s.Repo.ListBenefits(ctx, actorID, brandID)
}

func (s *Service) CreateBenefit(ctx context.Context, actorID, brandID int64, req model.CreateBenefitRequest) (model.Benefit, error) {
	name, err := cleanName(req.Name, 120)
	if err != nil || req.Requirement < 1 || req.Requirement > 10000000 {
		return model.Benefit{}, ErrInvalidRequest
	}
	return s.Repo.CreateBenefit(ctx, actorID, brandID, name, req.Requirement)
}

func (s *Service) BrandMovements(ctx context.Context, actorID, brandID int64, page, size int) ([]model.Movement, web.Pagination, error) {
	items, total, err := s.Repo.ListBrandMovements(ctx, actorID, brandID, page, size)
	return items, pagination(page, size, total), err
}

func (s *Service) BrandCustomers(ctx context.Context, actorID, brandID int64, page, size int, search string) ([]model.BrandCustomer, web.Pagination, error) {
	search = strings.TrimSpace(search)
	if len([]rune(search)) > 120 {
		return nil, web.Pagination{}, ErrInvalidRequest
	}
	items, total, err := s.Repo.ListBrandCustomers(ctx, actorID, brandID, page, size, search)
	return items, pagination(page, size, total), err
}

func (s *Service) BrandMetrics(ctx context.Context, actorID, brandID int64) (model.BrandMetricsSummary, error) {
	return s.Repo.BrandMetricsSummary(ctx, actorID, brandID)
}
