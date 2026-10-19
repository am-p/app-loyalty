package handler

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestEmailBrandAssetAllowlist(t *testing.T) {
	r := gin.New()
	h := &Handler{}
	r.GET("/assets/:name", h.EmailBrandAsset)
	for _, name := range []string{"puntazo-logo", "mr-puntazo", "unknown.png"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/assets/"+name, nil))
		if name == "unknown.png" {
			if w.Code != 404 {
				t.Fatal(w.Code)
			}
			continue
		}
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
			t.Fatal("image response")
		}
		if _, err := png.DecodeConfig(bytes.NewReader(w.Body.Bytes())); err != nil {
			t.Fatal(err)
		}
	}
}
