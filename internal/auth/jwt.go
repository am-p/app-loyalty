package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	AccountType string `json:"account_type"`
	AuthTime    int64  `json:"auth_time"`
	AuthVersion int    `json:"auth_version"`
	jwt.RegisteredClaims
}
type Tokens struct {
	Secret []byte
	Issuer string
	Now    func() time.Time
}

func NewTokens(secret, issuer string) *Tokens {
	return &Tokens{Secret: []byte(secret), Issuer: issuer, Now: time.Now}
}

func (t *Tokens) Generate(userID int64, accountType string) (string, error) {
	return t.GenerateForSession(userID, accountType, uuid.NewString())
}

func (t *Tokens) GenerateForSession(userID int64, accountType, sessionID string) (string, error) {
	return t.GenerateForSessionAt(userID, accountType, sessionID, t.Now().UTC())
}

func (t *Tokens) GenerateForSessionAt(userID int64, accountType, sessionID string, authTime time.Time) (string, error) {
	return t.GenerateForSessionAtVersion(userID, accountType, sessionID, authTime, 1)
}

func (t *Tokens) GenerateForSessionAtVersion(userID int64, accountType, sessionID string, authTime time.Time, authVersion int) (string, error) {
	now := t.Now().UTC()
	claims := Claims{AccountType: accountType, AuthTime: authTime.UTC().Unix(), AuthVersion: authVersion, RegisteredClaims: jwt.RegisteredClaims{
		Subject: strconv.FormatInt(userID, 10), ID: sessionID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		Issuer: t.Issuer, Audience: []string{"puntazo-app"},
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.Secret)
}

func (t *Tokens) Parse(raw string) (int64, string, error) {
	id, accountType, _, err := t.ParseSession(raw)
	return id, accountType, err
}

func (t *Tokens) ParseSession(raw string) (int64, string, string, error) {
	id, accountType, sessionID, _, err := t.ParseSessionVersion(raw)
	return id, accountType, sessionID, err
}

func (t *Tokens) ParseSessionVersion(raw string) (int64, string, string, int, error) {
	claims := new(Claims)
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return t.Secret, nil
	}, jwt.WithAudience("puntazo-app"), jwt.WithIssuer(t.Issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(t.Now))
	if err != nil || !token.Valid {
		return 0, "", "", 0, ErrInvalidToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id < 1 || claims.AuthVersion < 1 || claims.AuthTime < 1 || claims.AuthTime > t.Now().UTC().Unix()+60 || uuid.Validate(claims.ID) != nil || (claims.AccountType != "CLIENTE_FINAL" && claims.AccountType != "PERSONAL_MARCA") {
		return 0, "", "", 0, ErrInvalidToken
	}
	return id, claims.AccountType, claims.ID, claims.AuthVersion, nil
}
