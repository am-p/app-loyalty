package handler

import (
	"clientesFrecuentes/internal/push"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"encoding/json"
	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

func (h *Handler) WebPushKey(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	public, _, err := h.Repo.WebPushKeys(c.Request.Context(), push.GenerateWebPushKeys)
	if err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Notificaciones no disponibles", nil)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, web.Envelope[map[string]string]{Data: map[string]string{"public_key": public}, RequestID: web.RequestID(c)})
}
func (h *Handler) SaveWebPushSubscription(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "web-push:"+strconv.FormatInt(a.ID, 10), 20, time.Minute) {
		return
	}
	var input struct {
		Endpoint       string       `json:"endpoint"`
		Keys           webpush.Keys `json:"keys"`
		ExpirationTime *int64       `json:"expirationTime"`
	}
	if decode(c, &input) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	subscription := webpush.Subscription{Endpoint: input.Endpoint, Keys: input.Keys}
	if push.ValidateWebSubscription(subscription) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	body, _ := json.Marshal(subscription)
	if err := h.Repo.SaveWebPushSubscription(c.Request.Context(), a.ID, input.Endpoint, string(body)); err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "No se pudieron activar las notificaciones", nil)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[map[string]bool]{Data: map[string]bool{"registered": true}, RequestID: web.RequestID(c)})
}
func (h *Handler) DeleteWebPushSubscription(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	var input struct {
		Endpoint string `json:"endpoint"`
	}
	if decode(c, &input) != nil || input.Endpoint == "" || len(input.Endpoint) > 4096 {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if err := h.Repo.DeleteWebPushSubscription(c.Request.Context(), a.ID, input.Endpoint); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
