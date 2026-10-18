package middleware

import (
	"bytes"
	"clientesFrecuentes/internal/config"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLimitsPermitUploadBeyondJSONLimitAndBoundOverhead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestContext(slog.New(slog.NewTextHandler(io.Discard, nil))))
	read := func(c *gin.Context) {
		_, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			c.Status(413)
			return
		}
		c.Status(204)
	}
	r.POST("/v1/me/foto", read)
	r.POST("/v1/marcas/:brand_id/imagenes", read)
	r.POST("/v1/auth/register", read)
	for _, tc := range []struct {
		path   string
		n      int64
		status int
	}{{"/v1/me/foto", config.MaxUploadFileBytes, 204}, {"/v1/marcas/1/imagenes", config.MaxMultipartBytes, 204}, {"/v1/me/foto", config.MaxMultipartBytes + 1, 413}, {"/v1/auth/register", config.MaxJSONBytes + 1, 413}} {
		req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(make([]byte, tc.n)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s %d got %d", tc.path, tc.n, w.Code)
		}
	}
}
func TestRequestLogNeverIncludesInvitationTokenOrChallengeReceipt(t *testing.T) {
	var logs bytes.Buffer
	r := gin.New()
	r.Use(RequestContext(slog.New(slog.NewJSONHandler(&logs, nil))))
	r.GET("/v1/invitaciones/:token", func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest(http.MethodGet, "/v1/invitaciones/secret-invitation?receipt=secret-captcha&state=private", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	for _, secret := range []string{"secret-invitation", "secret-captcha", "private"} {
		if bytes.Contains(logs.Bytes(), []byte(secret)) {
			t.Fatal("sensitive URL in logs", logs.String())
		}
	}
}

func TestRequestLimitsPermitMenuPDFOnlyOnRegisteredUploadRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestContext(slog.New(slog.NewTextHandler(io.Discard, nil))))
	read := func(c *gin.Context) {
		n, err := io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		if n != c.Request.ContentLength {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusNoContent)
	}
	const route = "/v1/marcas/:brand_id/sucursales/:resource_id/menu/documentos"
	const path = "/v1/marcas/1/sucursales/2/menu/documentos"
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodGet} {
		r.Handle(method, route, read)
	}
	r.POST(route+"/extra", read)
	r.POST("/v1/auth/register", read)
	const menuMultipartLimit int64 = (10 << 20) + (64 << 10)
	for _, tc := range []struct {
		name, method, path string
		n                  int64
		status             int
	}{
		{"three-megabyte-PDF-body", http.MethodPost, path, 3 << 20, 204},
		{"ten-megabyte-file", http.MethodPost, path, 10 << 20, 204},
		{"multipart-boundary", http.MethodPost, path, menuMultipartLimit, 204},
		{"multipart-overflow", http.MethodPost, path, menuMultipartLimit + 1, 413},
		{"neighbor-route", http.MethodPost, path + "/extra", config.MaxJSONBytes + 1, 413},
		{"ordinary-JSON", http.MethodPost, "/v1/auth/register", config.MaxJSONBytes + 1, 413},
		{"PUT-stays-JSON", http.MethodPut, path, config.MaxJSONBytes + 1, 413},
		{"DELETE-stays-JSON", http.MethodDelete, path, config.MaxJSONBytes + 1, 413},
		{"GET-stays-JSON", http.MethodGet, path, config.MaxJSONBytes + 1, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, bytes.NewReader(make([]byte, tc.n))))
			if w.Code != tc.status {
				t.Fatalf("%s %s %d bytes: got %d, want %d", tc.method, tc.path, tc.n, w.Code, tc.status)
			}
		})
	}
}

func TestUploadSemaphoreRejectsDistinctActorsWithoutUnboundedWaiters(t *testing.T) {
	gate := NewUploadSemaphore(2, 1)
	r1, ok := gate.Acquire(t.Context(), 1)
	if !ok {
		t.Fatal("acquire1")
	}
	r2, ok := gate.Acquire(t.Context(), 2)
	if !ok {
		t.Fatal("acquire2")
	}
	for actor := int64(3); actor < 10003; actor++ {
		if _, ok := gate.Acquire(t.Context(), actor); ok {
			t.Fatal("exceeded global limit")
		}
	}
	active, actors := gate.Stats()
	if active != 2 || actors != 2 {
		t.Fatalf("unbounded gates %d/%d", active, actors)
	}
	r1()
	r2()
	active, actors = gate.Stats()
	if active != 0 || actors != 0 {
		t.Fatal("leaked gates")
	}
}
