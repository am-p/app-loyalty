package handler

import (
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGoogleConflictsUseStableCodes(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{repository.ErrGoogleLinkRequired, "GOOGLE_LINK_REQUIRED"}, {repository.ErrGoogleIdentityConflict, "GOOGLE_IDENTITY_CONFLICT"}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		writeErr(c, tc.err)
		if w.Code != 409 || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestDeleteAccountRequiresRecentAuthenticationBeforeRepositoryMutation(t *testing.T) {
	for _, authTime := range []time.Time{time.Time{}, time.Now().Add(-11 * time.Minute), time.Now().Add(time.Hour)} {
		h := &Handler{Service: &service.Service{}}
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ActorKey, middleware.Actor{ID: 1, AuthTime: authTime})
			c.Next()
		})
		r.DELETE("/v1/me", h.DeleteMe)
		req := httptest.NewRequest(http.MethodDelete, "/v1/me", strings.NewReader(`{"confirmacion":"ELIMINAR"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", `"1"`)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 401 || !strings.Contains(w.Body.String(), "RECENT_AUTH_REQUIRED") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestDisabledCaptchaChallengeNeverClaimsProtection(t *testing.T) {
	h := &Handler{Service: &service.Service{}}
	r := gin.New()
	r.GET("/v1/auth/challenge", h.Challenge)
	r.POST("/v1/auth/challenge/verify", h.VerifyChallenge)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := "/v1/auth/challenge"
		if method == http.MethodPost {
			path += "/verify"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != 503 || !strings.Contains(w.Body.String(), "CAPTCHA_UNAVAILABLE") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
