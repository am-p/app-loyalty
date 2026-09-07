package handler

import (
	"net/http"
	"strings"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func (h *Handler) RegisterCustomer(c *gin.Context) {
	var req model.RegisterCustomerRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "customer-register:ip:"+h.clientIP(c), customerSignupAttempts, customerSignupWindow) {
		return
	}
	data, err := h.Service.RegisterCustomer(c.Request.Context(), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, web.Envelope[model.AuthData]{Data: data, RequestID: web.RequestID(c)})
}

func (h *Handler) Login(c *gin.Context) {
	var req model.LoginRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "login:ip:"+h.clientIP(c), loginAttempts, loginWindow) || !h.limit(c, "login:email:"+strings.ToLower(strings.TrimSpace(req.Email)), loginAttempts, loginWindow) {
		return
	}
	data, err := h.Service.Login(c.Request.Context(), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.AuthData]{Data: data, RequestID: web.RequestID(c)})
}

func (h *Handler) Me(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	data, err := h.Service.CurrentUser(c.Request.Context(), a.ID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.CurrentUser]{Data: data, RequestID: web.RequestID(c)})
}
