package handler

import (
	"net/http"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
)

func (h *Handler) MerchantReferralCodes(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	codes, err := h.Repo.MerchantReferralCodes(c.Request.Context(), a.ID, brandID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]repository.ReferralCode]{Data: codes, RequestID: web.RequestID(c)})
}
