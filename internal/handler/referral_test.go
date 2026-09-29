package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"github.com/gin-gonic/gin"
)

func referralRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/referidos/validacion", h.ValidateReferralCode)
	return r
}

func TestReferralValidationRequestContract(t *testing.T) {
	cases := []struct{ name, body, code string }{
		{"missing code", `{}`, "INVALID_REQUEST"},
		{"null code", `{"code":null}`, "INVALID_REQUEST"},
		{"empty code", `{"code":""}`, "REFERRAL_CODE_INVALID"},
		{"empty program", `{"code":"VALID-123","program_type":""}`, "INVALID_REQUEST"},
		{"null program", `{"code":"VALID-123","program_type":null}`, "INVALID_REQUEST"},
		{"wrong program type", `{"code":"VALID-123","program_type":12}`, "INVALID_REQUEST"},
		{"invalid syntax", `{"code":"bad!"}`, "REFERRAL_CODE_INVALID"},
		{"short", `{"code":"ABC"}`, "REFERRAL_CODE_INVALID"},
		{"long", `{"code":"` + strings.Repeat("A", 41) + `"}`, "REFERRAL_CODE_INVALID"},
		{"invalid program", `{"code":"VALID-123","program_type":"OTHER"}`, "INVALID_REQUEST"},
		{"unknown field", `{"code":"VALID-123","campaign_id":1}`, "INVALID_REQUEST"},
		{"wrong code type", `{"code":12}`, "INVALID_REQUEST"},
		{"array", `[]`, "INVALID_REQUEST"},
		{"null", `null`, "INVALID_REQUEST"},
		{"malformed", `{"code":`, "INVALID_REQUEST"},
		{"extra json", `{"code":"VALID-123"} {}`, "INVALID_REQUEST"},
		{"too large", `{"code":"` + strings.Repeat("A", 4096) + `"}`, "INVALID_REQUEST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := referralRouter(&Handler{Service: &service.Service{}, Limiter: middleware.NewRateLimiter()})
			request := httptest.NewRequest(http.MethodPost, "/v1/referidos/validacion", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, request)
			if w.Code != 422 || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestReferralValidationLimitsRequestsPerIP(t *testing.T) {
	r := referralRouter(&Handler{Service: &service.Service{}, Limiter: middleware.NewRateLimiter()})
	for i := 0; i < 61; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/referidos/validacion", strings.NewReader(`{"code":"bad!"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		want := 422
		if i == 60 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d status=%d body=%s", i, w.Code, w.Body.String())
		}
		if i == 60 && (w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), `"code":"RATE_LIMITED"`)) {
			t.Fatalf("headers=%v body=%s", w.Header(), w.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/referidos/validacion", strings.NewReader(`{"code":"bad!"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.2:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Fatalf("other IP status=%d", w.Code)
	}
}

func TestReferralValidationDependencyUnavailable(t *testing.T) {
	redisLimiter, err := middleware.NewRedisRateLimiter("redis://127.0.0.1:1", "referral-test", 50*time.Millisecond, false)
	if err != nil {
		t.Fatal(err)
	}
	defer redisLimiter.Close()
	for _, h := range []*Handler{
		{Service: &service.Service{}, Limiter: middleware.NewRateLimiter()},
		{Service: &service.Service{}, Limiter: redisLimiter},
		{Service: &service.Service{}},
	} {
		r := referralRouter(h)
		req := httptest.NewRequest(http.MethodPost, "/v1/referidos/validacion", strings.NewReader(`{"code":"VALID-123"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 503 || !strings.Contains(w.Body.String(), `"code":"DEPENDENCY_UNAVAILABLE"`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
}

func TestFinalReferralErrorPrecedesInvalidRequest(t *testing.T) {
	if !errors.Is(repository.ErrReferralCodeInvalid, repository.ErrInvalidRequest) {
		t.Fatal("legacy errors.Is compatibility lost")
	}
	r := gin.New()
	r.POST("/", func(c *gin.Context) { writeErr(c, repository.ErrReferralCodeInvalid) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
	if w.Code != 422 || !strings.Contains(w.Body.String(), `"code":"REFERRAL_CODE_INVALID"`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
