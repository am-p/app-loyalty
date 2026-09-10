package middleware

import (
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.Config{
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Idempotency-Key", "X-Demo-Access-Code", "X-Request-ID"},
		ExposeHeaders:    []string{"Idempotent-Replayed", "Retry-After", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}

	origins := os.Getenv("CORS_ORIGINS")
	if origins == "" {
		return func(c *gin.Context) { c.Next() }
	}
	if origins != "" {
		for origin := range strings.SplitSeq(origins, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" {
				config.AllowOrigins = append(config.AllowOrigins, trimmed)
			}
		}
	}

	return cors.New(config)
}
