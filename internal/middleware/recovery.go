package middleware

import (
	"log/slog"
	"net/http"

	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

// Recovery handles panics in the request goroutine. It does not undo committed
// writes, resume the failed handler, or recover panics in child goroutines.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		logger.Error("panic recovered", "request_id", web.RequestID(c))
		web.AbortError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
	})
}
