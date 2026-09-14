package handler

import (
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"
	"github.com/gin-gonic/gin"
	"net/http"
)

func ids(c *gin.Context) (int64, int64, error) {
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		return 0, 0, e
	}
	id, e := positiveID(c.Param("resource_id"))
	return b, id, e
}
func (h *Handler) UpdateBrand(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	var q model.UpdateBrandRequest
	if decode(c, &q) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	d, e := h.Service.UpdateBrand(c.Request.Context(), a.ID, b, v, q)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.BrandVersion))
	c.JSON(200, web.Envelope[model.MerchantContext]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) DeleteBrand(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	if e = h.Service.DeleteBrand(c.Request.Context(), a.ID, b, v); e != nil {
		writeErr(c, e)
		return
	}
	c.Status(204)
}
func (h *Handler) Branches(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	d, e := h.Service.Branches(c.Request.Context(), a.ID, b)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.JSON(200, web.Envelope[[]model.Branch]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) Branch(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	d, e := h.Service.Branch(c.Request.Context(), a.ID, b, id)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Branch]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) CreateBranch(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	var q model.CreateBranchRequest
	if decode(c, &q) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	d, e := h.Service.CreateBranch(c.Request.Context(), a.ID, b, q)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(http.StatusCreated, web.Envelope[model.Branch]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) UpdateBranch(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	var q model.UpdateBranchRequest
	if decode(c, &q) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	d, e := h.Service.UpdateBranch(c.Request.Context(), a.ID, b, id, v, q)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Branch]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) DeleteBranch(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	if e = h.Service.DeleteBranch(c.Request.Context(), a.ID, b, id, v); e != nil {
		writeErr(c, e)
		return
	}
	c.Status(204)
}
func (h *Handler) Program(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	d, e := h.Service.Program(c.Request.Context(), a.ID, b)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Program]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) UpdateProgram(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, e := positiveID(c.Param("brand_id"))
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	var q model.UpdateProgramRequest
	if decode(c, &q) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	d, e := h.Service.UpdateProgram(c.Request.Context(), a.ID, b, v, q)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Program]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) Benefit(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	d, e := h.Service.Benefit(c.Request.Context(), a.ID, b, id)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Benefit]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) ReplaceBenefit(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	var q model.ReplaceBenefitRequest
	if decode(c, &q) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	d, e := h.Service.ReplaceBenefit(c.Request.Context(), a.ID, b, id, v, q)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Benefit]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) PatchBenefit(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	var q model.UpdateBenefitRequest
	if decode(c, &q) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	d, e := h.Service.UpdateBenefit(c.Request.Context(), a.ID, b, id, v, q)
	if e != nil {
		writeErr(c, e)
		return
	}
	c.Header("ETag", accountETag(d.Version))
	c.JSON(200, web.Envelope[model.Benefit]{Data: d, RequestID: web.RequestID(c)})
}
func (h *Handler) DeleteBenefit(c *gin.Context) {
	a, o := actor(c)
	if !o {
		return
	}
	b, id, e := ids(c)
	if e != nil {
		writeErr(c, e)
		return
	}
	v, o := accountVersion(c)
	if !o {
		return
	}
	if e = h.Service.DeleteBenefit(c.Request.Context(), a.ID, b, id, v); e != nil {
		writeErr(c, e)
		return
	}
	c.Status(204)
}
