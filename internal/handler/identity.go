package handler

import (
	"net/http"
	"strings"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

func (h *Handler) RequestEmailVerification(c *gin.Context) {
	var req model.EmailRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "verify-email:ip:"+h.clientIP(c), 5, loginWindow) || !h.limit(c, "verify-email:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 3, loginWindow) {
		return
	}
	if err := h.Service.RequestEmailVerification(c.Request.Context(), req); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, web.Envelope[map[string]string]{Data: map[string]string{"message": "Si la cuenta existe, enviaremos instrucciones"}, RequestID: web.RequestID(c)})
}
func (h *Handler) ConfirmEmailVerification(c *gin.Context) {
	var req model.TokenRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "verify-confirm:ip:"+h.clientIP(c), 10, loginWindow) {
		return
	}
	if err := h.Service.ConfirmEmailVerification(c.Request.Context(), req); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[map[string]string]{Data: map[string]string{"message": "Correo verificado. Ya podés iniciar sesión"}, RequestID: web.RequestID(c)})
}

func (h *Handler) RequestPasswordReset(c *gin.Context) {
	var req model.EmailRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "reset-password:ip:"+h.clientIP(c), 5, loginWindow) || !h.limit(c, "reset-password:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 3, loginWindow) {
		return
	}
	if err := h.Service.RequestPasswordReset(c.Request.Context(), req); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, web.Envelope[map[string]string]{Data: map[string]string{"message": "Si la cuenta existe, enviaremos instrucciones"}, RequestID: web.RequestID(c)})
}
func (h *Handler) ConfirmPasswordReset(c *gin.Context) {
	var req model.PasswordResetConfirmRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "reset-confirm:ip:"+h.clientIP(c), 10, loginWindow) {
		return
	}
	if err := h.Service.ConfirmPasswordReset(c.Request.Context(), req); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[map[string]string]{Data: map[string]string{"message": "Contraseña actualizada. Iniciá sesión nuevamente"}, RequestID: web.RequestID(c)})
}
