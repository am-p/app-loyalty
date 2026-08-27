package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

type actorStoreStub struct {
	accountType string
	err         error
	calls       int
}

func (s *actorStoreStub) ActiveAccountType(_ context.Context, _ int64) (string, error) {
	s.calls++
	return s.accountType, s.err
}

func TestPreviouslyIssuedTokenStopsAfterSuspension(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokens("01234567890123456789012345678901", "puntazo")
	raw, err := tokens.Generate(7, "CLIENTE_FINAL")
	if err != nil {
		t.Fatal(err)
	}
	store := &actorStoreStub{accountType: "CLIENTE_FINAL"}
	router := gin.New()
	router.GET("/protected", RequireAuth(tokens, store), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := request(); w.Code != http.StatusNoContent {
		t.Fatalf("active token status=%d body=%s", w.Code, w.Body.String())
	}
	store.accountType = ""
	store.err = errors.New("suspended")
	w := request()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("suspended token status=%d body=%s", w.Code, w.Body.String())
	}
	assertUnauthenticated(t, w.Body.Bytes())
	if store.calls != 2 {
		t.Fatalf("active lookup calls=%d", store.calls)
	}
}

func TestUnauthenticatedResponseIsUniform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := auth.NewTokens("01234567890123456789012345678901", "puntazo")
	store := &actorStoreStub{err: errors.New("inactive")}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(web.RequestIDKey, "00000000-0000-0000-0000-000000000001") })
	router.GET("/protected", RequireAuth(tokens, store), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	responses := make([]web.ErrorEnvelope, 0, 3)
	for _, header := range []string{"", "Bearer invalid", "Bearer " + mustToken(t, tokens, 99, "PERSONAL_MARCA")} {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("header %q status=%d", header, w.Code)
		}
		var envelope web.ErrorEnvelope
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, envelope)
	}
	for i := 1; i < len(responses); i++ {
		if responses[i].Error.Code != responses[0].Error.Code || responses[i].Error.Message != responses[0].Error.Message {
			t.Fatalf("non-uniform responses: %+v", responses)
		}
	}
}

func mustToken(t *testing.T, tokens *auth.Tokens, id int64, accountType string) string {
	t.Helper()
	raw, err := tokens.Generate(id, accountType)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func assertUnauthenticated(t *testing.T, body []byte) {
	t.Helper()
	var envelope web.ErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "UNAUTHENTICATED" || envelope.Error.Message != "Sesión inválida o expirada" {
		t.Fatalf("unexpected error %+v", envelope.Error)
	}
}
