package handler

import (
	"net/http"
	"time"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

const (
	clientPlatformHeader = "X-Client-Platform"
	refreshCookieName    = "puntazo_refresh"
	refreshCookiePath    = "/v1/auth"
	refreshCookieMaxAge  = int((30 * 24 * time.Hour) / time.Second)
)

func clientPlatform(c *gin.Context) (string, bool) {
	platform := c.GetHeader(clientPlatformHeader)
	if platform != "web" && platform != "native" {
		writeErr(c, service.ErrInvalidRequest)
		return "", false
	}
	return platform, true
}

func setRefreshCookie(c *gin.Context, value string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, value, refreshCookieMaxAge, refreshCookiePath, "", true, true)
}

func clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", true, true)
}

func writeAuth(c *gin.Context, status int, platform string, data model.AuthData) {
	if platform == "web" {
		setRefreshCookie(c, data.Session.RefreshToken)
		data.Session.RefreshToken = ""
	}
	c.JSON(status, web.Envelope[model.AuthData]{Data: data, RequestID: web.RequestID(c)})
}

func writeCustomerRegistration(c *gin.Context, status int, platform string, data model.RegisterCustomerData) {
	if data.Session != nil && platform == "web" {
		setRefreshCookie(c, data.Session.RefreshToken)
		data.Session.RefreshToken = ""
	}
	c.JSON(status, web.Envelope[model.RegisterCustomerData]{Data: data, RequestID: web.RequestID(c)})
}
