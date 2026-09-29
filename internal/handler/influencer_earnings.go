package handler

import (
	"net/http"
	"strconv"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

func (h *Handler) BackofficeInfluencerEarnings(c *gin.Context) {
	id, ok := backofficeID(c)
	if !ok {
		return
	}
	limit := 12
	if raw, supplied := c.GetQuery("limit"); supplied {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 24 {
			writeErr(c, repository.ErrInvalidRequest)
			return
		}
	}
	out, err := h.Repo.InfluencerEarnings(c.Request.Context(), id, c.Query("before"), limit)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[repository.InfluencerEarnings]{Data: out, RequestID: web.RequestID(c)})
}
