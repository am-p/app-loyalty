package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"clientesFrecuentes/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestStoreBoundaryRejectsInvalidActorAndInput(t *testing.T) {
	h := &Handler{}
	for _, tt := range []struct {
		method, path, kind string
		want               int
	}{
		{"GET", "/tiendas", "", 401},
		{"PUT", "/tiendas/1/favorito", "", 401},
		{"DELETE", "/tiendas/1/favorito", "", 401},
		{"GET", "/tiendas", "PERSONAL_MARCA", 403},
		{"PUT", "/tiendas/1/favorito", "PERSONAL_MARCA", 403},
		{"DELETE", "/tiendas/1/favorito", "PERSONAL_MARCA", 403},
		{"GET", "/tiendas?search=" + strings.Repeat("a", 201), "CLIENTE_FINAL", 400},
		{"GET", "/tiendas?search=%00", "CLIENTE_FINAL", 400},
		{"PUT", "/tiendas/0/favorito", "CLIENTE_FINAL", 400},
		{"DELETE", "/tiendas/-1/favorito", "CLIENTE_FINAL", 400},
		{"PUT", "/tiendas/nope/favorito", "CLIENTE_FINAL", 400},
		{"PUT", "/tiendas/9223372036854775808/favorito", "CLIENTE_FINAL", 400},
	} {
		t.Run(tt.method+tt.path+tt.kind, func(t *testing.T) {
			r := gin.New()
			if tt.kind != "" {
				r.Use(func(c *gin.Context) { c.Set(middleware.ActorKey, middleware.Actor{ID: 1, AccountType: tt.kind}) })
			}
			r.GET("/tiendas", h.CustomerStoreLocations)
			r.PUT("/tiendas/:branch_id/favorito", h.SetCustomerStoreFavorite)
			r.DELETE("/tiendas/:branch_id/favorito", h.DeleteCustomerStoreFavorite)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tt.method, tt.path, nil))
			if w.Code != tt.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
