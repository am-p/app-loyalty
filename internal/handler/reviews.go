package handler

import (
	"fmt"
	"net/http"
	"time"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func reviewJSON(c *gin.Context, data any) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, web.Envelope[any]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) ReviewSettings(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	if !h.reviewProviderLimit(c, a.ID) {
		return
	}
	out, e := h.Service.GetReviewSettings(c.Request.Context(), a.ID, b, id)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(out.Version))
	reviewJSON(c, out)
}
func (h *Handler) PutReviewSettings(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	v, ok := accountVersion(c)
	if !ok {
		return
	}
	var in model.ReviewSettingsInput
	if decode(c, &in) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.reviewProviderLimit(c, a.ID) {
		return
	}
	out, e := h.Service.PutReviewSettings(c.Request.Context(), a.ID, b, id, v, in)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(out.Version))
	reviewJSON(c, out)
}
func (h *Handler) SearchReviewPlaces(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	var in struct {
		Query string `json:"query"`
	}
	if decode(c, &in) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	// Authorization precedes quota consumption and every provider call.
	if _, e = h.Repo.ReviewBranch(c.Request.Context(), a.ID, b, id); e != nil {
		writeErr(c, e)
		return
	}
	if !h.limit(c, fmt.Sprintf("review-search:%d", a.ID), 20, time.Minute) {
		return
	}
	out, e := h.Service.SearchReviewPlaces(c.Request.Context(), a.ID, b, id, in.Query)
	if e != nil {
		writeErr(c, e)
		return
	}
	reviewJSON(c, out)
}
func (h *Handler) ReviewMetrics(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	out, e := h.Repo.ReviewMetrics(c.Request.Context(), a.ID, b, id)
	if e != nil {
		writeErr(c, e)
		return
	}
	reviewJSON(c, out)
}
func (h *Handler) PendingReviews(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if !h.reviewProviderLimit(c, a.ID) {
		return
	}
	out, e := h.Service.PendingReviews(c.Request.Context(), a.ID)
	if e != nil {
		writeErr(c, e)
		return
	}
	reviewJSON(c, out)
}
func (h *Handler) ReserveReview(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if !h.reviewProviderLimit(c, a.ID) {
		return
	}
	out, e := h.Service.ReserveReview(c.Request.Context(), a.ID, c.Param("invitation_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	reviewJSON(c, out)
}
func (h *Handler) ReviewEvent(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	var in model.ReviewEventRequest
	if decode(c, &in) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	out, e := h.Service.ReviewEvent(c.Request.Context(), a.ID, c.Param("invitation_id"), in)
	if e != nil {
		writeErr(c, e)
		return
	}
	reviewJSON(c, out)
}

// Shared quota bounds billed Place Details traffic across all resolution endpoints.
func (h *Handler) reviewProviderLimit(c *gin.Context, a int64) bool {
	if h.Service.Places == nil || !h.Service.Places.Available() {
		return true
	}
	return h.limit(c, fmt.Sprintf("review-resolve:%d", a), 60, time.Minute)
}
