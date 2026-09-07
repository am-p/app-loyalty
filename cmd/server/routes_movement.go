package main

import (
	"clientesFrecuentes/internal/handler"

	"github.com/gin-gonic/gin"
)

func registerMovementRoutes(r *gin.RouterGroup, h *handler.Handler) {
	r.POST("/movimientos/preview", h.Preview)
	r.POST("/movimientos/scan", h.ConfirmAccumulation)
	r.POST("/movimientos/canje", h.ConfirmRedemption)
	r.GET("/movimientos/idempotencia/:idempotency_key", h.Idempotency)
}
