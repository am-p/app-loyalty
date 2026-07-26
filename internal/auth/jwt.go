package auth

import (
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateToken(userID int64, role string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"rol": role,
		"exp": time.Now().Add(24*time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})
	secret := os.Getenv("JWT_SECRET")
	
	return token.SignedString([]byte(secret))
}
