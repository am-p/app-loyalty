package handler

import (
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

func (h *Handler) RequestEmailVerificationCode(c *gin.Context) {
	var req model.EmailRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "verify-code-request:ip:"+h.clientIP(c), 10, loginWindow) || !h.limit(c, "verify-code-request:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 6, loginWindow) {
		return
	}
	if err := h.Service.RequestEmailVerificationCode(c.Request.Context(), req); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, web.Envelope[map[string]string]{Data: map[string]string{"message": "Si la cuenta está pendiente, enviaremos un código"}, RequestID: web.RequestID(c)})
}
func (h *Handler) ConfirmEmailVerificationCode(c *gin.Context) {
	platform, ok := clientPlatform(c)
	if !ok {
		return
	}
	var req model.VerificationCodeConfirmRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if !h.limit(c, "verify-code-confirm:ip:"+h.clientIP(c), 20, loginWindow) || !h.limit(c, "verify-code-confirm:email:"+strings.ToLower(strings.TrimSpace(req.Email)), 10, loginWindow) {
		return
	}
	data, err := h.Service.ConfirmEmailVerificationCode(c.Request.Context(), req)
	if err != nil {
		writeErr(c, err)
		return
	}
	writeVerificationCode(c, platform, data)
}
func writeVerificationCode(c *gin.Context, platform string, data model.VerificationCodeData) {
	if data.Session != nil && platform == "web" {
		setRefreshCookie(c, data.Session.RefreshToken)
		data.Session.RefreshToken = ""
	}
	c.JSON(http.StatusOK, web.Envelope[model.VerificationCodeData]{Data: data, RequestID: web.RequestID(c)})
}
