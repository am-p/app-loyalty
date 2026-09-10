package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout propagates a deadline. Work must respect the context; this middleware
// does not forcibly interrupt handlers or write a timeout response.
func Timeout(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
