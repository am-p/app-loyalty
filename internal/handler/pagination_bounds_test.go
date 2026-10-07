package handler

import (
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCardsAndMovementEndpointsRejectUnboundedPagesBeforeQueries(t *testing.T) {
	h := &Handler{Service: &service.Service{}}
	for _, tc := range []struct {
		route, path, account string
		call                 gin.HandlerFunc
	}{{"/clientes/me/tarjetas", "/clientes/me/tarjetas", "CLIENTE_FINAL", h.Cards}, {"/clientes/me/movimientos", "/clientes/me/movimientos", "CLIENTE_FINAL", h.CustomerMovements}, {"/clientes/me/tarjetas/:card_id/movimientos", "/clientes/me/tarjetas/1/movimientos", "CLIENTE_FINAL", h.CardMovements}, {"/marcas/:brand_id/movimientos", "/marcas/1/movimientos", "PERSONAL_MARCA", h.BrandMovements}} {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(middleware.ActorKey, middleware.Actor{ID: 1, AccountType: tc.account})
			c.Next()
		})
		r.GET(tc.route, tc.call)
		for _, query := range []string{"?page_size=101", "?page_size=999999999999999999999999", "?page_size=0", "?page=-1"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path+query, nil))
			if w.Code != 422 || !strings.Contains(w.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("%s%s: %d %s", tc.path, query, w.Code, w.Body.String())
			}
		}
	}
}
