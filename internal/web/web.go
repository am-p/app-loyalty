package web

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "request_id"

type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}
type ErrorEnvelope struct {
	Error     APIError `json:"error"`
	RequestID string   `json:"request_id"`
}
type Envelope[T any] struct {
	Data      T      `json:"data"`
	RequestID string `json:"request_id"`
}
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int64 `json:"total_pages"`
}
type PageEnvelope[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
	RequestID  string     `json:"request_id"`
}

func RequestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	id := uuid.NewString()
	c.Set(RequestIDKey, id)
	return id
}
func AbortError(c *gin.Context, status int, code, message string, details map[string]any) {
	c.AbortWithStatusJSON(status, ErrorEnvelope{Error: APIError{Code: code, Message: message, Details: details}, RequestID: RequestID(c)})
}
func Error(c *gin.Context, status int, code, message string, details map[string]any) {
	c.JSON(status, ErrorEnvelope{Error: APIError{Code: code, Message: message, Details: details}, RequestID: RequestID(c)})
}
