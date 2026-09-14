package handler

import (
	"context"
	"net/http"
	"time"

	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func (h *Handler) HealthLive(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }

func (h *Handler) HealthReady(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.Repo.Pool.Ping(ctx); err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "PostgreSQL no disponible", nil)
		return
	}
	if err := h.Repo.CheckSchema(ctx, h.Service.Config.ExpectedSchemaVersion); err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Esquema no disponible", nil)
		return
	}
	if err := h.Limiter.Ready(ctx); err != nil {
		web.Error(c, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Redis no disponible", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": h.Service.Config.AppVersion, "commit": h.Service.Config.GitCommit, "schema_version": h.Service.Config.ExpectedSchemaVersion})
}
