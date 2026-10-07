package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestCanonicalClientIPTrustsOnlyConfiguredNetworks(t *testing.T) {
	trusted, e := ParseTrustedProxyCIDRs([]string{"10.0.0.0/8", "2001:db8:1::/48"})
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ remote, header, want string }{
		{"192.0.2.1:123", "203.0.113.9", "192.0.2.1"}, {"10.0.0.2:123", "203.0.113.9, 10.0.0.3", "203.0.113.9"}, {"10.0.0.2:123", "spoof,203.0.113.9", "10.0.0.2"}, {"[2001:db8:1::2]:123", "2001:0db8:0002::1", "2001:db8:2::1"}, {"10.0.0.2:123", "", "10.0.0.2"}, {"10.0.0.2:123", "10.0.0.3", "10.0.0.2"},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.RemoteAddr = tc.remote
		c.Request.Header.Set("X-Forwarded-For", tc.header)
		if got := ClientIP(c, 1, trusted...); got != tc.want {
			t.Fatalf("%+v got %s", tc, got)
		}
	}
}
