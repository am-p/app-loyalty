package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	Service           *service.Service
	Repo              *repository.Repository
	Limiter           *middleware.RateLimiter
	Logger            *slog.Logger
	TrustedProxyCount int
}

func decode(c *gin.Context, dst any) error {
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
		return service.ErrInvalidRequest
	}
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return service.ErrInvalidRequest
	}
	return nil
}
func actor(c *gin.Context) (middleware.Actor, bool) {
	a, ok := middleware.CurrentActor(c)
	if !ok {
		web.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Sesión inválida", nil)
	}
	return a, ok
}
func (h *Handler) limit(c *gin.Context, key string, n int, w time.Duration) bool {
	ok, retry := h.Limiter.Allow(key, n, w)
	if ok {
		return true
	}
	c.Header("Retry-After", strconv.Itoa(retry))
	web.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "Demasiados intentos", nil)
	return false
}
func (h *Handler) clientIP(c *gin.Context) string { return middleware.ClientIP(c, h.TrustedProxyCount) }

func (h *Handler) RegisterCustomer(c *gin.Context) {
	var req model.RegisterCustomerRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "customer-register:ip:"+h.clientIP(c), 10, 10*time.Minute) {
		return
	}
	data, err := h.Service.RegisterCustomer(c.Request.Context(), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, web.Envelope[model.AuthData]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) Login(c *gin.Context) {
	var req model.LoginRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "login:ip:"+h.clientIP(c), 10, 10*time.Minute) || !h.limit(c, "login:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 10, 10*time.Minute) {
		return
	}
	data, err := h.Service.Login(c.Request.Context(), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.AuthData]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) RegisterDemoMerchant(c *gin.Context) {
	var req model.RegisterDemoMerchantRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "merchant-register:ip:"+h.clientIP(c), 5, 10*time.Minute) || !h.limit(c, "merchant-register:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 3, 30*time.Minute) {
		return
	}
	result, err := h.Service.RegisterDemoMerchant(c.Request.Context(), c.GetHeader("Idempotency-Key"), c.GetHeader("X-Demo-Access-Code"), web.RequestID(c), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	if result.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.Data(result.Status, "application/json", result.Body)
}
func (h *Handler) Me(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	data, err := h.Service.CurrentUser(c.Request.Context(), a.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.CurrentUser]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) Customer(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	data, err := h.Service.Customer(c.Request.Context(), a.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.Customer]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) ListBrands(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	data, err := h.Service.ListBrands(c.Request.Context(), a.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]model.MerchantContext]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) Brand(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	id, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	data, err := h.Service.Brand(c.Request.Context(), a.ID, id)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.MerchantContext]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) Cards(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	page, size, err := pagination(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	data, p, err := h.Service.Cards(c.Request.Context(), a.ID, page, size)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.PageEnvelope[model.Card]{Data: data, Pagination: p, RequestID: web.RequestID(c)})
}
func (h *Handler) CardMovements(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	cardID, err := positiveID(c.Param("card_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	page, size, err := pagination(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	data, p, err := h.Service.CardMovements(c.Request.Context(), a.ID, cardID, page, size)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.PageEnvelope[model.Movement]{Data: data, Pagination: p, RequestID: web.RequestID(c)})
}
func (h *Handler) BrandMovements(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	page, size, err := pagination(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	data, p, err := h.Service.BrandMovements(c.Request.Context(), a.ID, brandID, page, size)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.PageEnvelope[model.Movement]{Data: data, Pagination: p, RequestID: web.RequestID(c)})
}
func (h *Handler) Preview(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "movement:actor:"+strconv.FormatInt(a.ID, 10), 60, time.Minute) {
		return
	}
	var req model.MovementPreviewRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	data, err := h.Service.Preview(c.Request.Context(), a.ID, req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.Preview]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) ConfirmAccumulation(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "movement:actor:"+strconv.FormatInt(a.ID, 10), 60, time.Minute) {
		return
	}
	var req model.ConfirmAccumulationRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	result, err := h.Service.ConfirmAccumulation(c.Request.Context(), a.ID, c.GetHeader("Idempotency-Key"), web.RequestID(c), req)
	h.writeIdempotent(c, result, err)
}
func (h *Handler) ConfirmRedemption(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "movement:actor:"+strconv.FormatInt(a.ID, 10), 60, time.Minute) {
		return
	}
	var req model.ConfirmRedemptionRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	result, err := h.Service.ConfirmRedemption(c.Request.Context(), a.ID, c.GetHeader("Idempotency-Key"), web.RequestID(c), req)
	h.writeIdempotent(c, result, err)
}
func (h *Handler) writeIdempotent(c *gin.Context, result repository.IdempotentResult, err error) {
	if err != nil {
		writeErr(c, err)
		return
	}
	if result.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.Data(result.Status, "application/json", result.Body)
}
func (h *Handler) Idempotency(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	body, err := h.Service.IdempotentMovement(c.Request.Context(), a.ID, c.Param("idempotency_key"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

func (h *Handler) HealthLive(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }
func (h *Handler) HealthReady(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.Repo.Pool.Ping(ctx); err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "PostgreSQL no disponible", nil)
		return
	}
	if err := h.Repo.CheckSchema(ctx, h.Service.Config.ExpectedSchemaVersion); err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Esquema no disponible", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
func (h *Handler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": h.Service.Config.AppVersion, "commit": h.Service.Config.GitCommit, "schema_version": h.Service.Config.ExpectedSchemaVersion})
}

func (h *Handler) LegacyRegister(c *gin.Context) {
	var req model.LegacyRegisterRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "customer-register:ip:"+h.clientIP(c), 10, 10*time.Minute) {
		return
	}
	data, err := h.Service.RegisterCustomer(c.Request.Context(), model.RegisterCustomerRequest{Email: req.Email, Password: req.Password, Name: req.Name})
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, legacyAuth(data))
}
func (h *Handler) LegacyLogin(c *gin.Context) {
	var req model.LoginRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "login:ip:"+h.clientIP(c), 10, 10*time.Minute) || !h.limit(c, "login:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 10, 10*time.Minute) {
		return
	}
	data, err := h.Service.Login(c.Request.Context(), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, legacyAuth(data))
}
func (h *Handler) LegacyGoogle(c *gin.Context) {
	var req model.GoogleAuthRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "login-google:ip:"+h.clientIP(c), 10, 10*time.Minute) {
		return
	}
	data, err := h.Service.LoginGoogle(c.Request.Context(), req.IDToken)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, legacyAuth(data))
}
func (h *Handler) LegacyMe(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	data, err := h.Service.CurrentUser(c.Request.Context(), a.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, model.LegacyUser{ID: data.User.ID, Email: data.User.Email, Name: data.User.Name, Role: legacyRole(data.User.AccountType)})
}
func legacyAuth(data model.AuthData) model.LegacyAuth {
	return model.LegacyAuth{Token: data.Session.AccessToken, User: model.LegacyUser{ID: data.User.ID, Email: data.User.Email, Name: data.User.Name, Role: legacyRole(data.User.AccountType)}}
}
func legacyRole(accountType string) string {
	if accountType == "PERSONAL_MARCA" {
		return "TIENDA"
	}
	return "CLIENTE_FINAL"
}

func positiveID(value string) (int64, error) {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil || v < 1 {
		return 0, service.ErrInvalidRequest
	}
	return v, nil
}
func pagination(c *gin.Context) (int, int, error) {
	page, size := 1, 20
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return 0, 0, service.ErrInvalidRequest
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil || size < 1 || size > 100 {
			return 0, 0, service.ErrInvalidRequest
		}
	}
	return page, size, nil
}

func writeErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidRequest):
		web.Error(c, http.StatusUnprocessableEntity, "INVALID_REQUEST", "Solicitud inválida", nil)
	case errors.Is(err, service.ErrInvalidCredentials):
		web.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Credenciales inválidas", nil)
	case errors.Is(err, service.ErrForbidden):
		web.Error(c, http.StatusForbidden, "FORBIDDEN", "Acceso denegado", nil)
	case errors.Is(err, service.ErrDemoDisabled):
		web.Error(c, http.StatusForbidden, "DEMO_SIGNUP_DISABLED", "Las altas demo están cerradas", nil)
	case errors.Is(err, service.ErrDemoAccess):
		web.Error(c, http.StatusForbidden, "DEMO_ACCESS_DENIED", "Código de acceso inválido", nil)
	case errors.Is(err, repository.ErrEmailExists):
		web.Error(c, http.StatusConflict, "EMAIL_EXISTS", "El email ya está registrado", nil)
	case errors.Is(err, repository.ErrIdempotencyConflict):
		web.Error(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "La clave ya fue usada con otra solicitud", nil)
	case errors.Is(err, repository.ErrIdempotencyInProgress):
		web.Error(c, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "La solicitud todavía está en curso", nil)
	case errors.Is(err, repository.ErrPreviewExpired):
		web.Error(c, http.StatusConflict, "PREVIEW_EXPIRED", "La preview venció", nil)
	case errors.Is(err, repository.ErrPreviewConsumed):
		web.Error(c, http.StatusConflict, "PREVIEW_CONSUMED", "La preview ya fue consumida", nil)
	case errors.Is(err, repository.ErrPreviewChanged):
		web.Error(c, http.StatusConflict, "PREVIEW_CHANGED", "La preview ya no coincide con el estado actual", nil)
	case errors.Is(err, repository.ErrInsufficientBalance):
		web.Error(c, http.StatusConflict, "INSUFFICIENT_BALANCE", "Saldo insuficiente", nil)
	case errors.Is(err, repository.ErrNotFound):
		web.Error(c, http.StatusNotFound, "NOT_FOUND", "Recurso no encontrado", nil)
	default:
		web.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
	}
}
