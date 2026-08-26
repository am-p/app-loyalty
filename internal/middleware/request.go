package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequestContext(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := c.GetHeader("X-Request-ID")
		if _, err := uuid.Parse(requestID); err != nil {
			requestID = uuid.NewString()
		}
		c.Set(web.RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, config.MaxJSONBytes)
		}
		c.Next()
		logger.Info("request", "request_id", requestID, "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}

func Timeout(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func ClientIP(c *gin.Context, trustedProxyCount int) string {
	remote := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if trustedProxyCount <= 0 {
		return remote
	}
	parts := strings.Split(c.GetHeader("X-Forwarded-For"), ",")
	index := len(parts) - trustedProxyCount
	if index < 0 || index >= len(parts) {
		return remote
	}
	ip := strings.TrimSpace(parts[index])
	if net.ParseIP(ip) == nil {
		return remote
	}
	return ip
}

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic recovered", "request_id", web.RequestID(c))
		web.AbortError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
	})
}
