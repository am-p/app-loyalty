package handler

import (
	"net/http"
	"strconv"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Preview(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "movement:actor:"+strconv.FormatInt(a.ID, 10), movementAttempts, movementWindow) {
		return
	}
	var req model.MovementPreviewRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	data, err := h.Service.Preview(c.Request.Context(), a.ID, req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.Preview]{Data: data, RequestID: web.RequestID(c)})
}

func (h *Handler) ConfirmAccumulation(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "movement:actor:"+strconv.FormatInt(a.ID, 10), movementAttempts, movementWindow) {
		return
	}
	var req model.ConfirmAccumulationRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	result, err := h.Service.ConfirmAccumulation(c.Request.Context(), a.ID, c.GetHeader("Idempotency-Key"), web.RequestID(c), req)
	h.writeIdempotent(c, result, err)
}

func (h *Handler) ConfirmRedemption(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "PERSONAL_MARCA" {
		writeErr(c, service.ErrForbidden)
		return
	}
	if !h.limit(c, "movement:actor:"+strconv.FormatInt(a.ID, 10), movementAttempts, movementWindow) {
		return
	}
	var req model.ConfirmRedemptionRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	result, err := h.Service.ConfirmRedemption(c.Request.Context(), a.ID, c.GetHeader("Idempotency-Key"), web.RequestID(c), req)
	h.writeIdempotent(c, result, err)
}

func (h *Handler) writeIdempotent(c *gin.Context, result repository.IdempotentResult, err error) {
	if err != nil {
		writeErr(c, err)
		return
	}
	if result.Replayed {
		c.Header("Idempotent-Replayed", "true")
	}
	c.Data(result.Status, "application/json", result.Body)
}

func (h *Handler) Idempotency(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	body, err := h.Service.IdempotentMovement(c.Request.Context(), a.ID, c.Param("idempotency_key"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}
