package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	AccountType string `json:"account_type"`
	jwt.RegisteredClaims
}
type Tokens struct {
	Secret []byte
	Now    func() time.Time
}

func NewTokens(secret string) *Tokens { return &Tokens{Secret: []byte(secret), Now: time.Now} }

func (t *Tokens) Generate(userID int64, accountType string) (string, error) {
	now := t.Now().UTC()
	claims := Claims{AccountType: accountType, RegisteredClaims: jwt.RegisteredClaims{
		Subject: strconv.FormatInt(userID, 10), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		Issuer: "puntazo-preview", Audience: []string{"puntazo-app"},
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.Secret)
}

func (t *Tokens) Parse(raw string) (int64, string, error) {
	claims := new(Claims)
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return t.Secret, nil
	}, jwt.WithAudience("puntazo-app"), jwt.WithIssuer("puntazo-preview"), jwt.WithExpirationRequired(), jwt.WithTimeFunc(t.Now))
	if err != nil || !token.Valid {
		return 0, "", ErrInvalidToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id < 1 || (claims.AccountType != "CLIENTE_FINAL" && claims.AccountType != "PERSONAL_MARCA") {
		return 0, "", ErrInvalidToken
	}
	return id, claims.AccountType, nil
}
