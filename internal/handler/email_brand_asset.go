package handler

import (
	"clientesFrecuentes/internal/mailer"
	"github.com/gin-gonic/gin"
	"net/http"
)

// EmailBrandAsset serves an allowlist of public brand images without cookies,
// account information, redirects, or per-recipient tracking.
func (h *Handler) EmailBrandAsset(c *gin.Context) {
	image := mailer.PublicEmailImage(c.Param("name"))
	if len(image) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "image/png", image)
}
