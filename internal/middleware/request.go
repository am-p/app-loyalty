package middleware

import (
	"log/slog"
	"net/http"
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
