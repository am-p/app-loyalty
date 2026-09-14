package main

import (
	"clientesFrecuentes/internal/handler"

	"github.com/gin-gonic/gin"
)

func registerMerchantRoutes(r *gin.RouterGroup, h *handler.Handler) {
	r.GET("/marcas", h.ListBrands)
	r.GET("/marcas/:brand_id", h.Brand)
	r.GET("/marcas/:brand_id/beneficios", h.Benefits)
	r.POST("/marcas/:brand_id/beneficios", h.CreateBenefit)
	r.GET("/marcas/:brand_id/movimientos", h.BrandMovements)
	r.GET("/marcas/:brand_id/clientes", h.BrandCustomers)
	r.GET("/marcas/:brand_id/metricas/resumen", h.BrandMetrics)
}
