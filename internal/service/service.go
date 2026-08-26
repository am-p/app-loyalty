package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidRequest     = errors.New("invalid request")
	ErrForbidden          = errors.New("forbidden")
	ErrDemoDisabled       = errors.New("demo signup disabled")
	ErrDemoAccess         = errors.New("demo access denied")
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

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 254 {
		return "", ErrInvalidRequest
	}
	return value, nil
}
func cleanName(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > max {
		return "", ErrInvalidRequest
	}
	return value, nil
}
func validPassword(value string) bool { return len(value) >= 10 && len(value) <= 128 }

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

func (s *Service) CurrentUser(ctx context.Context, actorID int64) (model.CurrentUser, error) {
	return s.Repo.GetCurrentUser(ctx, actorID)
}
func (s *Service) Customer(ctx context.Context, actorID int64) (model.Customer, error) {
	u, err := s.Repo.GetCustomer(ctx, actorID)
	if err != nil {
		return model.Customer{}, err
	}
	token, _ := s.QRForUser(u.ID)
	return model.Customer{ID: u.ID, Email: u.Email, Name: u.Name, QRToken: token}, nil
}

func (s *Service) QRForUser(userID int64) (string, []byte) {
	mac := hmac.New(sha256.New, []byte(s.Config.QRPepper))
	fmt.Fprintf(mac, "puntazo:customer:%d", userID)
	token := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	h := sha256.New()
	h.Write([]byte(s.Config.QRPepper))
	h.Write([]byte(token))
	return token, h.Sum(nil)
}
func (s *Service) QRHash(token string) []byte {
	h := sha256.New()
	h.Write([]byte(s.Config.QRPepper))
	h.Write([]byte(token))
	return h.Sum(nil)
}

func (s *Service) RegisterDemoMerchant(ctx context.Context, key, accessCode, requestID string, req model.RegisterDemoMerchantRequest) (repository.IdempotentResult, error) {
	if !s.Config.DemoSignupEnabled {
		return repository.IdempotentResult{}, ErrDemoDisabled
	}
	if len(accessCode) < 12 || len(accessCode) > 128 {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	if bcrypt.CompareHashAndPassword([]byte(s.Config.DemoAccessCodeHash), []byte(accessCode)) != nil {
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
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	// This fingerprint is persisted. Key it so a database leak cannot turn the
	// low-entropy password field into a fast, offline SHA-256 oracle.
	fingerprint := KeyedFingerprint(s.Config.QRPepper, struct {
		Email, Password, OwnerName, BrandName, BranchName string
		BranchAddress                                     *string
	}{email, req.Password, owner, brand, branch, req.BranchAddress})
	var result repository.IdempotentResult
	err = retry(ctx, func() error {
		var e error
		result, e = s.Repo.CreateDemoMerchant(ctx, key, fingerprint, email, string(passwordHash), owner, brand, branch, req.BranchAddress, func(u model.User, m model.MerchantContext) ([]byte, error) {
			token, e := s.Tokens.Generate(u.ID, u.AccountType)
			if e != nil {
				return nil, e
			}
			return json.Marshal(web.Envelope[model.DemoMerchantData]{Data: model.DemoMerchantData{Session: session(token), User: u, Merchant: m}, RequestID: requestID})
		})
		return e
	})
	return result, err
}

func (s *Service) ListBrands(ctx context.Context, actorID int64) ([]model.MerchantContext, error) {
	return s.Repo.ListMerchantContexts(ctx, actorID)
}
func (s *Service) Brand(ctx context.Context, actorID, brandID int64) (model.MerchantContext, error) {
	return s.Repo.GetMerchantContext(ctx, actorID, brandID)
}
func (s *Service) Cards(ctx context.Context, actorID int64, page, size int) ([]model.Card, web.Pagination, error) {
	items, total, err := s.Repo.ListCards(ctx, actorID, page, size)
	return items, pagination(page, size, total), err
}
func (s *Service) CardMovements(ctx context.Context, actorID, cardID int64, page, size int) ([]model.Movement, web.Pagination, error) {
	items, total, err := s.Repo.ListCustomerMovements(ctx, actorID, cardID, page, size)
	return items, pagination(page, size, total), err
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
func pagination(page, size int, total int64) web.Pagination {
	pages := int64(0)
	if total > 0 {
		pages = (total + int64(size) - 1) / int64(size)
	}
	return web.Pagination{Page: page, PageSize: size, TotalItems: total, TotalPages: pages}
}

func (s *Service) Preview(ctx context.Context, actorID int64, req model.MovementPreviewRequest) (model.Preview, error) {
	if req.Operation != "ACUMULACION" && req.Operation != "CANJE" || req.BranchID < 1 {
		return model.Preview{}, ErrInvalidRequest
	}
	if req.Operation == "ACUMULACION" && req.BenefitID != nil {
		return model.Preview{}, ErrInvalidRequest
	}
	if req.Operation == "CANJE" && (req.BenefitID == nil || *req.BenefitID < 1) {
		return model.Preview{}, ErrInvalidRequest
	}
	token, err := s.resolveMovementIdentity(req.QRToken, req.CustomerCode)
	if err != nil {
		return model.Preview{}, err
	}
	req.QRToken = token
	req.CustomerCode = ""
	return s.Repo.CreatePreview(ctx, actorID, req, s.QRHash(req.QRToken), Fingerprint(req))
}

func (s *Service) ConfirmAccumulation(ctx context.Context, actorID int64, key, requestID string, req model.ConfirmAccumulationRequest) (repository.IdempotentResult, error) {
	if req.BranchID < 1 {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	if _, err := uuid.Parse(req.PreviewID); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	token, err := s.resolveMovementIdentity(req.QRToken, req.CustomerCode)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	req.QRToken = token
	req.CustomerCode = ""
	return s.confirm(ctx, repository.ConfirmInput{ActorID: actorID, Key: key, Fingerprint: Fingerprint(req), PreviewID: req.PreviewID, QRHash: s.QRHash(req.QRToken), BranchID: req.BranchID, Operation: "ACUMULACION"}, requestID)
}
func (s *Service) ConfirmRedemption(ctx context.Context, actorID int64, key, requestID string, req model.ConfirmRedemptionRequest) (repository.IdempotentResult, error) {
	if req.BranchID < 1 || req.BenefitID < 1 {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	if _, err := uuid.Parse(req.PreviewID); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	token, err := s.resolveMovementIdentity(req.QRToken, req.CustomerCode)
	if err != nil {
		return repository.IdempotentResult{}, err
	}
	req.QRToken = token
	req.CustomerCode = ""
	return s.confirm(ctx, repository.ConfirmInput{ActorID: actorID, Key: key, Fingerprint: Fingerprint(req), PreviewID: req.PreviewID, QRHash: s.QRHash(req.QRToken), BranchID: req.BranchID, BenefitID: &req.BenefitID, Operation: "CANJE"}, requestID)
}

func (s *Service) resolveMovementIdentity(qrToken, customerCode string) (string, error) {
	qrToken = strings.TrimSpace(qrToken)
	customerCode = strings.TrimSpace(customerCode)
	if (qrToken == "") == (customerCode == "") {
		return "", ErrInvalidRequest
	}
	if qrToken != "" {
		if len(qrToken) < 32 || len(qrToken) > 256 {
			return "", ErrInvalidRequest
		}
		return qrToken, nil
	}

	normalizedCode := strings.ToUpper(strings.TrimPrefix(customerCode, "#"))
	if !strings.HasPrefix(normalizedCode, "USER-") {
		return "", ErrInvalidRequest
	}
	digits := strings.TrimPrefix(normalizedCode, "USER-")
	if len(digits) < 4 || len(digits) > 19 {
		return "", ErrInvalidRequest
	}
	customerID, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || customerID < 1 {
		return "", ErrInvalidRequest
	}
	token, _ := s.QRForUser(customerID)
	return token, nil
}
func (s *Service) confirm(ctx context.Context, in repository.ConfirmInput, requestID string) (repository.IdempotentResult, error) {
	if _, err := uuid.Parse(in.Key); err != nil {
		return repository.IdempotentResult{}, ErrInvalidRequest
	}
	var result repository.IdempotentResult
	err := retry(ctx, func() error {
		var e error
		result, e = s.Repo.ConfirmMovement(ctx, in, func(m model.Movement) ([]byte, error) {
			return json.Marshal(web.Envelope[model.Movement]{Data: m, RequestID: requestID})
		})
		return e
	})
	return result, err
}
func (s *Service) IdempotentMovement(ctx context.Context, actorID int64, key string) (json.RawMessage, error) {
	if _, err := uuid.Parse(key); err != nil {
		return nil, ErrInvalidRequest
	}
	return s.Repo.GetMovementIdempotency(ctx, actorID, key)
}

func Fingerprint(v any) []byte { b, _ := json.Marshal(v); sum := sha256.Sum256(b); return sum[:] }
func KeyedFingerprint(key string, v any) []byte {
	b, _ := json.Marshal(v)
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(b)
	return mac.Sum(nil)
}
func retry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if !repository.IsRetryable(err) {
			return err
		}
		delay := time.Duration(10+attempt*15) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
