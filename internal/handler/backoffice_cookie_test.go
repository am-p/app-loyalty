package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBackofficeCookieTransportByEnvironment(t *testing.T) {
	for _, test := range []struct {
		environment string
		publicURL   string
		secure      bool
	}{{"", "", false}, {"development", "http://localhost:8081", false}, {"staging", "", true}, {"production", "", true}, {"", "https://puntazo.pro", true}, {"development", "https://puntazo.pro", true}} {
		t.Run(test.environment+test.publicURL, func(t *testing.T) {
			t.Setenv("APP_ENV", test.environment)
			t.Setenv("PUBLIC_APP_URL", test.publicURL)
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			(&Handler{}).setBackofficeCookie(ctx, "test-session", 8*3600)
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing cookie")
			}
			c := cookies[0]
			if c.Secure != test.secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/v1/backoffice" || c.MaxAge != 8*3600 {
				t.Fatalf("invalid attributes for %s", test.environment)
			}
		})
	}
}
