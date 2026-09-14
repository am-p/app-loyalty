package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/service"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

func (h *Handler) BrandImages(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	items, err := h.Service.BrandImages(c.Request.Context(), a.ID, brandID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, web.Envelope[[]model.BrandImage]{Data: items, RequestID: web.RequestID(c)})
}

func (h *Handler) UploadBrandImage(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	if err = h.Service.AuthorizeBrandMedia(c.Request.Context(), a.ID, brandID); err != nil {
		writeErr(c, err)
		return
	}
	if !h.limit(c, fmt.Sprintf("media-upload:actor:%d", a.ID), mediaActorAttempts, mediaUploadWindow) ||
		!h.limit(c, "media-upload:ip:"+h.clientIP(c), mediaIPAttempts, mediaUploadWindow) {
		return
	}
	if h.Uploads == nil {
		writeErr(c, service.ErrMediaUnavailable)
		return
	}
	release, acquired := h.Uploads.Acquire(c.Request.Context(), a.ID)
	if !acquired {
		writeErr(c, service.ErrMediaUnavailable)
		return
	}
	defer release()
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		writeErr(c, service.ErrMediaType)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (5<<20)+(64<<10))
	if err = c.Request.ParseMultipartForm(64 << 10); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeErr(c, service.ErrMediaTooLarge)
		} else {
			writeErr(c, service.ErrInvalidRequest)
		}
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	if len(c.Request.MultipartForm.File) != 1 || len(c.Request.MultipartForm.File["archivo"]) != 1 {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	for key := range c.Request.MultipartForm.Value {
		if key != "tipo" && key != "benefit_id" {
			writeErr(c, service.ErrInvalidRequest)
			return
		}
	}
	fileHeader := c.Request.MultipartForm.File["archivo"][0]
	file, err := fileHeader.Open()
	if err != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil {
		writeErr(c, service.ErrInvalidRequest)
		return
	}
	if len(body) > 5<<20 {
		writeErr(c, service.ErrMediaTooLarge)
		return
	}
	kind := c.PostForm("tipo")
	var benefitID *int64
	if raw := c.PostForm("benefit_id"); raw != "" {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id < 1 {
			writeErr(c, service.ErrInvalidRequest)
			return
		}
		benefitID = &id
	}
	item, err := h.Service.UploadBrandImage(c.Request.Context(), a.ID, brandID, kind, benefitID, body)
	if err != nil {
		writeErr(c, err)
		return
	}
	if item.URL == "" && h.Logger != nil {
		h.Logger.WarnContext(c.Request.Context(), "media activated without signed URL", "brand_id", brandID, "image_id", item.ID)
	}
	c.Header("ETag", accountETag(item.Version))
	c.JSON(http.StatusCreated, web.Envelope[model.BrandImage]{Data: item, RequestID: web.RequestID(c)})
}

func (h *Handler) DeleteBrandImage(c *gin.Context) {
	a, ok := actor(c)
	if !ok {
		return
	}
	brandID, err := positiveID(c.Param("brand_id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	version, ok := accountVersion(c)
	if !ok {
		return
	}
	if err = h.Service.DeleteBrandImage(c.Request.Context(), a.ID, brandID, c.Param("image_id"), version); err != nil {
		writeErr(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
