package middleware

import (
	"github.com/gin-gonic/gin"
	"net"
	"strings"
)

// ClientIP ignores legacy hop-count trust. Only explicitly trusted network peers
// may supply forwarding headers. Malformed chains fall back to the socket peer.
func ClientIP(c *gin.Context, _ int, trusted ...*net.IPNet) string {
	remote := c.Request.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	peer := net.ParseIP(strings.Trim(remote, "[]"))
	if peer == nil {
		return "unknown"
	}
	fallback := peer.String()
	isTrusted := func(ip net.IP) bool {
		for _, network := range trusted {
			if network.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(peer) {
		return fallback
	}
	raw := strings.TrimSpace(c.GetHeader("X-Forwarded-For"))
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 32 {
		return fallback
	}
	ips := make([]net.IP, len(parts))
	for i, part := range parts {
		ips[i] = net.ParseIP(strings.TrimSpace(part))
		if ips[i] == nil {
			return fallback
		}
	}
	for i := len(ips) - 1; i >= 0; i-- {
		if !isTrusted(ips[i]) {
			return ips[i].String()
		}
	}
	return fallback
}
func ParseTrustedProxyCIDRs(raw []string) ([]*net.IPNet, error) {
	out := make([]*net.IPNet, 0, len(raw))
	for _, part := range raw {
		if strings.TrimSpace(part) == "" {
			continue
		}
		_, network, err := net.ParseCIDR(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		out = append(out, network)
	}
	return out, nil
}
