package auth

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

func GenerateToken(userID int64, role string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", errors.New("JWT_SECRET is not set")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"rol": role,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	return token.SignedString([]byte(secret))
}

func ParseToken(tokenString string) (int64, string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return 0, "", errors.New("JWT_SECRET is not set")
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return 0, "", ErrInvalidToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", ErrInvalidToken
	}

	sub, ok := claims["sub"].(float64)
	if !ok {
		return 0, "", ErrInvalidToken
	}

	role, ok := claims["rol"].(string)
	if !ok {
		return 0, "", ErrInvalidToken
	}

	return int64(sub), role, nil
}
