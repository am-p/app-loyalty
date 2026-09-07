package handler

import (
	"errors"
	"net/http"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

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
