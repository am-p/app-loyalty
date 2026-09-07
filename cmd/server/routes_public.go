package main

import (
	"clientesFrecuentes/internal/handler"

	"github.com/gin-gonic/gin"
)

func registerPublicRoutes(r *gin.RouterGroup, h *handler.Handler) {
	r.GET("/health/live", h.HealthLive)
	r.GET("/health/ready", h.HealthReady)
	r.GET("/version", h.Version)
	r.POST("/auth/register", h.RegisterCustomer)
	r.POST("/auth/login", h.Login)
	r.POST("/demo/comercios", h.RegisterDemoMerchant)
}
