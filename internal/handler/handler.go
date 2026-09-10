package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"clientesFrecuentes/internal/middleware"
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
