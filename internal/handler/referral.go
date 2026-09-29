package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ValidateReferralCode(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if h.Limiter == nil {
		writeErr(c, service.ErrReferralValidationUnavailable)
		return
	}
	if !h.limit(c, "referral-validation:ip:"+h.clientIP(c), 60, time.Minute) {
		return
	}
	var input struct {
		Code        *string         `json:"code"`
		ProgramType json.RawMessage `json:"program_type"`
	}
	if decode(c, &input) != nil || input.Code == nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	req := service.ReferralValidationRequest{Code: *input.Code}
	if len(input.ProgramType) > 0 {
		var program *string
		if json.Unmarshal(input.ProgramType, &program) != nil || program == nil || (*program != "SELLOS" && *program != "PUNTOS") {
			writeErr(c, service.ErrInvalidRequest)
			return
		}
		req.ProgramType = *program
	}
	if h.Service == nil {
		writeErr(c, service.ErrReferralValidationUnavailable)
		return
	}
	out, err := h.Service.ValidateReferralCode(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrReferralValidationUnavailable) && h.Logger != nil {
			h.Logger.ErrorContext(c.Request.Context(), "referral validation unavailable", "error", err)
		}
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[repository.ReferralValidation]{Data: out, RequestID: web.RequestID(c)})
}

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
