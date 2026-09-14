package handler

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"

	"github.com/gin-gonic/gin"
)

func decode(c *gin.Context, dst any) error {
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
		return service.ErrInvalidRequest
	}
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return service.ErrInvalidRequest
	}
	return nil
}

func positiveID(value string) (int64, error) {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil || v < 1 {
		return 0, service.ErrInvalidRequest
	}
	return v, nil
}

func accountETag(version int) string { return `"` + strconv.Itoa(version) + `"` }

func accountVersion(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		writeErr(c, repository.ErrPreconditionFailed)
		return 0, false
	}
	version, err := strconv.Atoi(raw[1 : len(raw)-1])
	if err != nil || version < 1 {
		writeErr(c, repository.ErrPreconditionFailed)
		return 0, false
	}
	return version, true
}

func pagination(c *gin.Context) (int, int, error) {
	page, size := 1, 20
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			return 0, 0, service.ErrInvalidRequest
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		size, err = strconv.Atoi(raw)
		if err != nil || size < 1 || size > 100 {
			return 0, 0, service.ErrInvalidRequest
		}
	}
	return page, size, nil
}
