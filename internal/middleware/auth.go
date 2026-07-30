package middleware

import (
	"net/http"
	"strings"

	"clientesFrecuentes/internal/auth"

	"github.com/gin-gonic/gin"
)

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")

		tokenString, found := strings.CutPrefix(header, "Bearer ")
		if !found || tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "Falta el token de acceso"})
			c.Abort()
			return
		}

		userID, role, err := auth.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "Sesión inválida o expirada"})
			c.Abort()
			return
		}

		c.Set("userID", userID)
		c.Set("role", role)

		c.Next()
	}
}
