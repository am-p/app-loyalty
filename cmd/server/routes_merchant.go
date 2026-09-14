package main

import (
	"clientesFrecuentes/internal/handler"

	"github.com/gin-gonic/gin"
)

func registerMerchantRoutes(r *gin.RouterGroup, h *handler.Handler) {
	r.GET("/marcas", h.ListBrands)
	r.GET("/marcas/:brand_id", h.Brand)
	r.PATCH("/marcas/:brand_id", h.UpdateBrand)
	r.DELETE("/marcas/:brand_id", h.DeleteBrand)
	r.GET("/marcas/:brand_id/sucursales", h.Branches)
	r.POST("/marcas/:brand_id/sucursales", h.CreateBranch)
	r.GET("/marcas/:brand_id/sucursales/:resource_id", h.Branch)
	r.PATCH("/marcas/:brand_id/sucursales/:resource_id", h.UpdateBranch)
	r.DELETE("/marcas/:brand_id/sucursales/:resource_id", h.DeleteBranch)
	r.GET("/marcas/:brand_id/programa", h.Program)
	r.PUT("/marcas/:brand_id/programa", h.UpdateProgram)
	r.GET("/marcas/:brand_id/beneficios", h.Benefits)
	r.POST("/marcas/:brand_id/beneficios", h.CreateBenefit)
	r.GET("/marcas/:brand_id/beneficios/:resource_id", h.Benefit)
	r.PUT("/marcas/:brand_id/beneficios/:resource_id", h.ReplaceBenefit)
	r.PATCH("/marcas/:brand_id/beneficios/:resource_id", h.PatchBenefit)
	r.DELETE("/marcas/:brand_id/beneficios/:resource_id", h.DeleteBenefit)
	r.GET("/marcas/:brand_id/movimientos", h.BrandMovements)
	r.GET("/marcas/:brand_id/clientes", h.BrandCustomers)
	r.GET("/marcas/:brand_id/metricas/resumen", h.BrandMetrics)
}
