package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clientesFrecuentes/internal/model"

	"github.com/gin-gonic/gin"
)

func TestWebAuthUsesHardenedRefreshCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	writeAuth(c, 200, "web", model.AuthData{Session: model.Session{AccessToken: "access", RefreshToken: "refresh-secret", TokenType: "Bearer", ExpiresIn: 900}})
	if strings.Contains(response.Body.String(), "refresh-secret") || strings.Contains(response.Body.String(), "refresh_token") {
		t.Fatalf("web JSON exposed refresh token: %s", response.Body.String())
	}
	cookie := response.Header().Get("Set-Cookie")
	for _, attribute := range []string{"puntazo_refresh=refresh-secret", "Path=/v1/auth", "Max-Age=2592000", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(cookie, attribute) {
			t.Fatalf("cookie=%q missing=%q", cookie, attribute)
		}
	}
}

func TestNativeAuthUsesJSONRefreshToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	writeAuth(c, 200, "native", model.AuthData{Session: model.Session{AccessToken: "access", RefreshToken: "refresh-secret", TokenType: "Bearer", ExpiresIn: 900}})
	if response.Header().Get("Set-Cookie") != "" {
		t.Fatalf("native response set cookie=%q", response.Header().Get("Set-Cookie"))
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Body.String(), `"refresh_token":"refresh-secret"`) {
		t.Fatalf("native JSON omitted refresh token: %s", response.Body.String())
	}
}
