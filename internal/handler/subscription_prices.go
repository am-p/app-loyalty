package handler

import (
	"net/http"
	"strconv"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) SubscriptionPrices(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	out, err := h.Service.SubscriptionPrices(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]repository.SubscriptionPrice]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficePricePreview(c *gin.Context) {
	amount, err := strconv.ParseInt(c.Query("unit_price_minor"), 10, 64)
	if err != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	out, err := h.Repo.PreviewSubscriptionPrice(c.Request.Context(), c.Param("program"), amount)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]repository.SubscriptionPricePreview]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeChangePrice(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	var in struct {
		UnitPriceMinor  int64  `json:"unit_price_minor"`
		Version         *int64 `json:"version"`
		IncludeExisting *bool  `json:"include_existing"`
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	if err != nil || key == uuid.Nil || c.ShouldBindJSON(&in) != nil || in.Version == nil || in.IncludeExisting == nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	if *in.IncludeExisting && !h.Service.SubscriptionPriceUpdatesAvailable() {
		writeErr(c, service.ErrBillingUnavailable)
		return
	}
	out, err := h.Service.ChangeSubscriptionPrice(c.Request.Context(), backofficeUser(c).ID, key, c.Param("program"), in.UnitPriceMinor, *in.Version, *in.IncludeExisting)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, web.Envelope[repository.SubscriptionPriceChange]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficePriceHistory(c *gin.Context) {
	limit := 20
	if raw, ok := c.GetQuery("limit"); ok {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeErr(c, repository.ErrInvalidRequest)
			return
		}
		limit = value
	}
	var before *uuid.UUID
	if raw, ok := c.GetQuery("before"); ok {
		value, err := uuid.Parse(raw)
		if err != nil || value == uuid.Nil {
			writeErr(c, repository.ErrInvalidRequest)
			return
		}
		before = &value
	}
	out, err := h.Repo.ListSubscriptionPriceChanges(c.Request.Context(), limit, before)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[repository.SubscriptionPriceHistoryPage]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficePriceChange(c *gin.Context) {
	key, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	out, err := h.Repo.SubscriptionPriceChange(c.Request.Context(), key)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[repository.SubscriptionPriceChange]{Data: out, RequestID: web.RequestID(c)})
}
func (h *Handler) BackofficeRetryPriceChange(c *gin.Context) {
	if !requireSystemAdmin(c) {
		return
	}
	key, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeErr(c, repository.ErrInvalidRequest)
		return
	}
	if !h.Service.SubscriptionPriceUpdatesAvailable() {
		writeErr(c, service.ErrBillingUnavailable)
		return
	}
	if err = h.Repo.RetrySubscriptionPriceChange(c.Request.Context(), key, backofficeUser(c).ID); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
