package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

func ClientIP(c *gin.Context, trustedProxyCount int) string {
	remote := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	if trustedProxyCount <= 0 {
		return remote
	}
	parts := strings.Split(c.GetHeader("X-Forwarded-For"), ",")
	index := len(parts) - trustedProxyCount
	if index < 0 || index >= len(parts) {
		return remote
	}
	ip := strings.TrimSpace(parts[index])
	if net.ParseIP(ip) == nil {
		return remote
	}
	return ip
}
