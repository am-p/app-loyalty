package repository_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/repository"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func TestPostgresBackofficePasswordLogin(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	t.Setenv("APP_ENV", "")
	password := "PasswordLoginTest!2026"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	var admin, inactive int64
	for _, x := range []struct {
		email, role, secret string
		active              bool
		id                  *int64
	}{
		{"password-admin@example.test", "ADMIN_SISTEMA", "legacy-secret", true, &admin},
		{"password-finance@example.test", "FINANZAS", "", true, new(int64)},
		{"password-inactive@example.test", "ADMIN_SISTEMA", "", false, &inactive},
	} {
		if err := pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,last_totp_step,role,active) VALUES($1,$2,$3,77,$4,$5) RETURNING id`, x.email, string(hash), x.secret, x.role, x.active).Scan(x.id); err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	newRouter := func() *gin.Engine {
		h := &handler.Handler{Repo: repo, Limiter: middleware.NewRateLimiter()}
		r := gin.New()
		r.POST("/login", h.BackofficeLogin)
		r.GET("/me", h.RequireBackoffice, h.BackofficeMe)
		r.POST("/logout", h.RequireBackoffice, h.BackofficeLogout)
		return r
	}
	for _, x := range []struct {
		name, email, password string
		legacy                bool
		status                int
	}{
		{"admin without code", "  PASSWORD-ADMIN@EXAMPLE.TEST  ", password, false, 200},
		{"finance without code", "password-finance@example.test", password, false, 200},
		{"legacy unused field", "password-admin@example.test", password, true, 200},
		{"wrong password", "password-admin@example.test", "incorrect", false, 401},
		{"missing password", "password-admin@example.test", "", false, 401},
		{"unknown identity", "unknown@example.test", password, false, 401},
		{"inactive identity", "password-inactive@example.test", password, false, 401},
	} {
		t.Run(x.name, func(t *testing.T) {
			r := newRouter()
			body := map[string]string{"email": x.email, "password": x.password}
			if x.legacy {
				body["totp"] = "unused"
			}
			encoded, _ := json.Marshal(body)
			request := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(encoded))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != x.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			cookies := response.Result().Cookies()
			if x.status != 200 {
				if len(cookies) != 0 {
					t.Fatal("invalid credentials issued a cookie")
				}
				return
			}
			if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].MaxAge != 8*3600 {
				t.Fatalf("session cookie attributes invalid")
			}
			if strings.Contains(response.Body.String(), "password_hash") || strings.Contains(response.Body.String(), "legacy-secret") || strings.Contains(response.Body.String(), "totp") {
				t.Fatal("private credentials exposed")
			}
			me := httptest.NewRequest(http.MethodGet, "/me", nil)
			me.AddCookie(cookies[0])
			identity := httptest.NewRecorder()
			r.ServeHTTP(identity, me)
			if identity.Code != 200 {
				t.Fatalf("identity status=%d", identity.Code)
			}
			logout := httptest.NewRequest(http.MethodPost, "/logout", nil)
			logout.AddCookie(cookies[0])
			logout.Header.Set("X-Backoffice-Request", "1")
			closed := httptest.NewRecorder()
			r.ServeHTTP(closed, logout)
			if closed.Code != 204 {
				t.Fatalf("logout status=%d", closed.Code)
			}
			revoked := httptest.NewRecorder()
			r.ServeHTTP(revoked, me)
			if revoked.Code != 401 {
				t.Fatal("revoked session accepted")
			}
		})
	}
	if err := repo.CreateBackofficeSession(ctx, inactive, "inactive-token", time.Now().Add(time.Hour)); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("inactive session err=%v", err)
	}
	for _, token := range []string{"first-password-session", "second-password-session"} {
		if err := repo.CreateBackofficeSession(ctx, admin, token, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(token))
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM backoffice_sessions WHERE token_hash=$1)`, digest[:]).Scan(&exists); err != nil || !exists {
			t.Fatal("session digest missing")
		}
	}
	var lastStep int64
	if err := pool.QueryRow(ctx, `SELECT last_totp_step FROM backoffice_users WHERE id=$1`, admin).Scan(&lastStep); err != nil || lastStep != 77 {
		t.Fatalf("legacy state changed: step=%d err=%v", lastStep, err)
	}
	if err := repo.CreateBackofficeSession(ctx, admin, "expired-token", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BackofficeSession(ctx, "expired-token"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expired session err=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE backoffice_users SET active=false WHERE id=$1`, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.BackofficeSession(ctx, "first-password-session"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("deactivated identity err=%v", err)
	}
}
