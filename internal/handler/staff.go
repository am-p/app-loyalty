package handler

import (
	"net/http"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func invitationID(c *gin.Context) (string, error) {
	id, err := uuid.Parse(c.Param("invitation_id"))
	return id.String(), err
}

func (h *Handler) Invitations(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	items, err := h.Service.Invitations(c.Request.Context(), a.ID, brandID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]model.BrandInvitation]{Data: items, RequestID: web.RequestID(c)})
}
func (h *Handler) CreateInvitation(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	var req model.CreateInvitationRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	item, err := h.Service.CreateInvitation(c.Request.Context(), a.ID, brandID, req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Header("ETag", accountETag(item.Version))
	c.JSON(http.StatusCreated, web.Envelope[model.BrandInvitation]{Data: item, RequestID: web.RequestID(c)})
}
func (h *Handler) RevokeInvitation(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	id, err := invitationID(c)
	if err != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if err = h.Service.RevokeInvitation(c.Request.Context(), a.ID, brandID, id); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) ResendInvitation(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	id, err := invitationID(c)
	if err != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	item, err := h.Service.ResendInvitation(c.Request.Context(), a.ID, brandID, id)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Header("ETag", accountETag(item.Version))
	c.JSON(http.StatusOK, web.Envelope[model.BrandInvitation]{Data: item, RequestID: web.RequestID(c)})
}
func (h *Handler) PublicInvitation(c *gin.Context) {
	item, err := h.Service.PublicInvitation(c.Request.Context(), c.Param("token"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.PublicInvitation]{Data: item, RequestID: web.RequestID(c)})
}
func (h *Handler) AcceptInvitation(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	item, err := h.Service.AcceptInvitation(c.Request.Context(), a.ID, c.Param("token"))
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[model.StaffMember]{Data: item, RequestID: web.RequestID(c)})
}
func (h *Handler) Staff(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	items, err := h.Service.Staff(c.Request.Context(), a.ID, brandID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]model.StaffMember]{Data: items, RequestID: web.RequestID(c)})
}
func (h *Handler) StaffMember(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, id, err := ids(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	item, err := h.Service.StaffMember(c.Request.Context(), a.ID, brandID, id)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Header("ETag", accountETag(item.Version))
	c.JSON(http.StatusOK, web.Envelope[model.StaffMember]{Data: item, RequestID: web.RequestID(c)})
}
func (h *Handler) UpdateStaff(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, id, err := ids(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	version, ok := accountVersion(c)
	if !ok {
		return
	}
	var req model.UpdateStaffRequest
	if decode(c, &req) != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	item, err := h.Service.UpdateStaff(c.Request.Context(), a.ID, brandID, id, version, req)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.Header("ETag", accountETag(item.Version))
	c.JSON(http.StatusOK, web.Envelope[model.StaffMember]{Data: item, RequestID: web.RequestID(c)})
}
func (h *Handler) DeleteStaff(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, id, err := ids(c)
	if err != nil {
		writeErr(c, err)
		return
	}
	version, ok := accountVersion(c)
	if !ok {
		return
	}
	if err = h.Service.DeleteStaff(c.Request.Context(), a.ID, brandID, id, version); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
