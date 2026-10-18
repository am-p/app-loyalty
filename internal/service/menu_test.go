package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

func TestMenuPDFValidation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		pages int
		err   error
	}{{"menu.pdf", 1, nil}, {"menu-30.pdf", 30, nil}, {"menu-31.pdf", 0, ErrMenuPDFPages}, {"menu-scanned.pdf", 1, nil}} {
		t.Run(tt.name, func(t *testing.T) {
			body, err := os.ReadFile("testdata/" + tt.name)
			if err != nil {
				t.Fatal(err)
			}
			pages, sum, err := validateMenuPDF(t.Context(), body)
			if !errors.Is(err, tt.err) || pages != tt.pages {
				t.Fatalf("pages=%d err=%v", pages, err)
			}
			if err == nil && len(sum) != 32 {
				t.Fatal("missing SHA256")
			}
		})
	}
	for _, tt := range []struct {
		body []byte
		err  error
	}{{[]byte("not a PDF"), ErrMenuPDFType}, {[]byte("%PDF-1.4\nfake"), ErrMenuPDFInvalid}, {bytes.Repeat([]byte{'x'}, int(MaxMenuPDFBytes)+1), ErrMenuPDFTooLarge}} {
		if _, _, err := validateMenuPDF(t.Context(), tt.body); !errors.Is(err, tt.err) {
			t.Fatalf("err=%v want=%v", err, tt.err)
		}
	}
	body, err := os.ReadFile("testdata/menu.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, userPassword := range []string{"secret", ""} {
		conf, err := api.LoadConfiguration(api.ConfigurationOptions{Mode: api.ConfigurationModeStateless})
		if err != nil {
			t.Fatal(err)
		}
		conf.UserPW = userPassword
		conf.OwnerPW = "owner-secret"
		var encrypted bytes.Buffer
		if err = api.Encrypt(t.Context(), bytes.NewReader(body), &encrypted, conf); err != nil {
			t.Fatal(err)
		}
		if _, _, err = validateMenuPDF(t.Context(), encrypted.Bytes()); !errors.Is(err, ErrMenuPDFPassword) {
			t.Fatalf("encrypted with userpw=%t err=%v", userPassword != "", err)
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err = validateMenuPDF(cancelled, body); err == nil {
		t.Fatal("cancelled parser succeeded")
	}
}
func TestMenuFileNameSanitization(t *testing.T) {
	for _, tt := range []struct{ in, want string }{{"../../menu.pdf", "menu.pdf"}, {`C:\fakepath\carta.pdf`, "carta.pdf"}, {"\x00\n.pdf", "menu.pdf"}, {"carta", "carta.pdf"}, {"Café.pdf", "Café.pdf"}} {
		if got := menuFileName(tt.in); got != tt.want {
			t.Fatalf("got=%q want=%q", got, tt.want)
		}
	}
	if got := menuFileName(strings.Repeat("á", 200) + ".pdf"); len(got) > 160 || !strings.HasSuffix(got, ".pdf") {
		t.Fatalf("name=%q", got)
	}
}
