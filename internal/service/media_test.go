package service

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestNormalizeImageReencodesJPEGAndPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	for _, tc := range []struct {
		name   string
		encode func(*bytes.Buffer) error
		mime   string
	}{{"jpeg", func(b *bytes.Buffer) error { return jpeg.Encode(b, img, nil) }, "image/jpeg"}, {"png", func(b *bytes.Buffer) error { return png.Encode(b, img) }, "image/png"}} {
		t.Run(tc.name, func(t *testing.T) {
			var source bytes.Buffer
			if err := tc.encode(&source); err != nil {
				t.Fatal(err)
			}
			body, mime, w, h, digest, err := normalizeImage(source.Bytes(), "BENEFICIO")
			if err != nil {
				t.Fatal(err)
			}
			if mime != tc.mime || w != 4 || h != 3 || len(digest) != 32 || len(body) == 0 {
				t.Fatalf("normalized mime=%s dimensions=%dx%d digest=%d", mime, w, h, len(digest))
			}
			if _, _, err = image.Decode(bytes.NewReader(body)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNormalizeImageRejectsUnsupportedAndOversized(t *testing.T) {
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black})
	var encoded bytes.Buffer
	if err := gif.Encode(&encoded, img, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, _, err := normalizeImage(encoded.Bytes(), "LOGO"); !errors.Is(err, ErrMediaType) {
		t.Fatalf("GIF error=%v", err)
	}
	if _, _, _, _, _, err := normalizeImage(make([]byte, maxMediaBytes+1), "LOGO"); !errors.Is(err, ErrMediaTooLarge) {
		t.Fatalf("large error=%v", err)
	}
}

func TestNormalizeImageAcceptsWebPAsPNGAndResizesIcon(t *testing.T) {
	webpBytes, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	body, mime, w, h, _, err := normalizeImage(webpBytes, "ICONO")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/png" || w != 1 || h != 1 || len(body) == 0 {
		t.Fatalf("webp normalized mime=%s size=%dx%d bytes=%d", mime, w, h, len(body))
	}
	large := image.NewRGBA(image.Rect(0, 0, 2048, 1024))
	var source bytes.Buffer
	if err = png.Encode(&source, large); err != nil {
		t.Fatal(err)
	}
	_, _, w, h, _, err = normalizeImage(source.Bytes(), "ICONO")
	if err != nil {
		t.Fatal(err)
	}
	if w != 512 || h != 256 {
		t.Fatalf("icon dimensions=%dx%d", w, h)
	}
}
