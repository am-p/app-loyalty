package handler

import (
	"clientesFrecuentes/internal/model"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerificationCodeSessionTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{"web", "native"} {
		out := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(out)
		writeVerificationCode(c, platform, model.VerificationCodeData{Verified: true, Session: &model.Session{AccessToken: "access", RefreshToken: "secret", TokenType: "Bearer", ExpiresIn: 900}})
		exposed := strings.Contains(out.Body.String(), `"refresh_token":"secret"`)
		cookie := out.Header().Get("Set-Cookie")
		if platform == "web" {
			if exposed || !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Strict") || !strings.Contains(cookie, "Secure") {
				t.Fatal("web session transport")
			}
		} else if !exposed || cookie != "" {
			t.Fatal("native session transport")
		}
	}
	out := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(out)
	writeVerificationCode(c, "web", model.VerificationCodeData{Verified: true})
	if out.Header().Get("Set-Cookie") != "" || strings.Contains(out.Body.String(), "session") {
		t.Fatal("manual login response emitted session")
	}
}
