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
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}

	origins := os.Getenv("CORS_ORIGINS")
	if origins == "" {
		config.AllowAllOrigins = true
	} else {
		for origin := range strings.SplitSeq(origins, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" {
				config.AllowOrigins = append(config.AllowOrigins, trimmed)
			}
		}
	}

	return cors.New(config)
}
