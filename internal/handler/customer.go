package handler

import (
	"net/http"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func (h *Handler) Customer(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	data, err := h.Service.Customer(c.Request.Context(), a.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.Customer]{Data: data, RequestID: web.RequestID(c)})
}

func (h *Handler) Cards(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	page, size, err := pagination(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	data, p, err := h.Service.Cards(c.Request.Context(), a.ID, page, size)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.PageEnvelope[model.Card]{Data: data, Pagination: p, RequestID: web.RequestID(c)})
}

func (h *Handler) CardMovements(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	cardID, err := positiveID(c.Param("card_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	page, size, err := pagination(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	data, p, err := h.Service.CardMovements(c.Request.Context(), a.ID, cardID, page, size)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.PageEnvelope[model.Movement]{Data: data, Pagination: p, RequestID: web.RequestID(c)})
}

func (h *Handler) CustomerMovements(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	page, size, err := pagination(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	data, p, err := h.Service.CustomerMovements(c.Request.Context(), a.ID, page, size)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.PageEnvelope[model.Movement]{Data: data, Pagination: p, RequestID: web.RequestID(c)})
}
