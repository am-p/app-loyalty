package handler

import (
	"net/http"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

func (h *Handler) CustomerStoreGoogleReview(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	id, err := positiveID(c.Param("branch_id"))
	if err != nil {
		web.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Sucursal inválida", nil)
		return
	}
	if !h.reviewProviderLimit(c, a.ID) {
		return
	}
	data, err := h.Service.CustomerStoreGoogleReview(c.Request.Context(), a.ID, id)
	if err != nil {
		h.storeError(c, err)
		return
	}
	reviewJSON(c, data)
}

func (h *Handler) CustomerStoreLocations(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	search, err := service.NormalizeStoreSearch(c.Query("search"))
	if err != nil {
		web.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Búsqueda inválida; máximo 200 caracteres", nil)
		return
	}
	data, err := h.Service.CustomerStoreLocations(c.Request.Context(), a.ID, search)
	if err != nil {
		h.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]model.StoreLocation]{Data: data, RequestID: web.RequestID(c)})
}

func (h *Handler) SetCustomerStoreFavorite(c *gin.Context)    { h.customerStoreFavorite(c, true) }
func (h *Handler) DeleteCustomerStoreFavorite(c *gin.Context) { h.customerStoreFavorite(c, false) }
func (h *Handler) customerStoreFavorite(c *gin.Context, favorite bool) {
	a, ok := actor(c)
	if !ok {
		return
	}
	if a.AccountType != "CLIENTE_FINAL" {
		writeErr(c, service.ErrForbidden)
		return
	}
	id, err := positiveID(c.Param("branch_id"))
	if err != nil {
		web.Error(c, http.StatusBadRequest, "INVALID_REQUEST", "Sucursal inválida", nil)
		return
	}
	data, err := h.Service.SetCustomerStoreFavorite(c.Request.Context(), a.ID, id, favorite)
	if err != nil {
		h.storeError(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.StoreFavorite]{Data: data, RequestID: web.RequestID(c)})
}
func (h *Handler) storeError(c *gin.Context, err error) {
	if h.Logger != nil {
		h.Logger.ErrorContext(c.Request.Context(), "customer store request failed", "request_id", web.RequestID(c), "route", c.FullPath())
	}
	writeErr(c, err)
}
