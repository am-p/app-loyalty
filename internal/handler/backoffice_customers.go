package handler

import (
	"net/http"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

func (h *Handler) BackofficeCustomers(c *gin.Context) {
	out, err := h.Repo.ListBackofficeCustomers(c.Request.Context())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]repository.BackofficeCustomer]{Data: out, RequestID: web.RequestID(c)})
}
