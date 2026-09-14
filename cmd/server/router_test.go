package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/service"
)

// Fix the external route inventory and check authentication on every protected
// route: moving registration between groups must not silently expose a domain.
func TestRouteInventoryAndAuthentication(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &handler.Handler{Service: &service.Service{}, Limiter: middleware.NewRateLimiter(), Logger: logger}
	r := newRouter(h, auth.NewTokens("01234567890123456789012345678901", "puntazo"), logger)
	public := []string{
		"GET /v1/health/live", "GET /v1/health/ready", "GET /v1/version",
		"POST /v1/auth/register", "POST /v1/auth/login", "POST /v1/auth/google", "POST /v1/auth/refresh", "POST /v1/demo/comercios",
		"POST /v1/auth/email-verification/request", "POST /v1/auth/email-verification/confirm",
		"POST /v1/auth/password-reset/request", "POST /v1/auth/password-reset/confirm",
		"POST /auth/register", "POST /auth/login", "POST /auth/google",
	}
	protected := []struct{ method, pattern, request string }{
		{"GET", "/v1/me", "/v1/me"},
		{"PATCH", "/v1/me", "/v1/me"},
		{"GET", "/v1/me/export", "/v1/me/export"},
		{"DELETE", "/v1/me", "/v1/me"},
		{"POST", "/v1/auth/logout", "/v1/auth/logout"},
		{"GET", "/me", "/me"},
		{"GET", "/v1/marcas", "/v1/marcas"},
		{"GET", "/v1/marcas/:brand_id", "/v1/marcas/1"},
		{"GET", "/v1/marcas/:brand_id/beneficios", "/v1/marcas/1/beneficios"},
		{"POST", "/v1/marcas/:brand_id/beneficios", "/v1/marcas/1/beneficios"},
		{"GET", "/v1/marcas/:brand_id/movimientos", "/v1/marcas/1/movimientos"},
		{"GET", "/v1/marcas/:brand_id/clientes", "/v1/marcas/1/clientes"},
		{"GET", "/v1/marcas/:brand_id/metricas/resumen", "/v1/marcas/1/metricas/resumen"},
		{"GET", "/v1/clientes/me", "/v1/clientes/me"},
		{"GET", "/v1/clientes/me/tarjetas", "/v1/clientes/me/tarjetas"},
		{"GET", "/v1/clientes/me/tarjetas/:card_id/movimientos", "/v1/clientes/me/tarjetas/1/movimientos"},
		{"POST", "/v1/movimientos/preview", "/v1/movimientos/preview"},
		{"POST", "/v1/movimientos/scan", "/v1/movimientos/scan"},
		{"POST", "/v1/movimientos/canje", "/v1/movimientos/canje"},
		{"GET", "/v1/movimientos/idempotencia/:idempotency_key", "/v1/movimientos/idempotencia/test"},
	}
	expected := make(map[string]bool)
	for _, route := range public {
		expected[route] = true
	}
	for _, route := range protected {
		expected[route.method+" "+route.pattern] = true
		t.Run(route.method+" "+route.request, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(route.method, route.request, nil))
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		if !expected[key] {
			t.Errorf("unexpected route %s", key)
		}
		delete(expected, key)
	}
	for key := range expected {
		t.Errorf("missing route %s", key)
	}
}
