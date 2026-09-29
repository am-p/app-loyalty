package mailer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clientesFrecuentes/internal/model"
)

func TestInfluencerWelcomeConditionsAndEscaping(t *testing.T) {
	for _, test := range []struct {
		programs []string
		label    string
	}{
		{[]string{"SELLOS"}, "Sellos"}, {[]string{"PUNTOS"}, "Puntos"}, {[]string{"SELLOS", "PUNTOS"}, "Sellos y puntos"},
	} {
		t.Run(test.label, func(t *testing.T) {
			d := model.InfluencerWelcomeDetails{Name: "María <script>", Code: "MARIA-BARRIO", CampaignName: "Campaña <img src=x>", ProgramTypes: test.programs, DiscountBPS: 3525, DiscountCharges: 3, RewardBPS: 1050, RewardCharges: 12, StartsAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 12, 28, 12, 0, 0, 0, time.UTC), Active: true}
			m := InfluencerWelcomeMessage("https://app.puntazo.test/?source=mail", "maria@example.test", d)
			for _, expected := range []string{test.label, "MARIA-BARRIO", "35,25% en los primeros 3 cobros", "10,5% en los primeros 12 cobros", "28/09/2026 09:00", "efectivamente cobrado", "después del descuento", "ref=MARIA-BARRIO", "source=mail", "cid:" + mascotContentID} {
				if !strings.Contains(m.HTML, expected) {
					t.Errorf("HTML missing %q", expected)
				}
			}
			if m.To != "maria@example.test" || m.Kind != "INFLUENCER_WELCOME" || m.Token != "" || strings.Contains(m.HTML, "<script>") || strings.Contains(m.HTML, "<img src=x>") || strings.Contains(m.HTML, "%!") {
				t.Fatalf("invalid welcome=%+v", m)
			}
			if !strings.Contains(m.HTML, "&lt;script&gt;") || !strings.Contains(m.Text, test.label) || !strings.Contains(m.Text, "Campaña <img src=x>") {
				t.Fatal("plain text or escaping failed")
			}
		})
	}
	if got := influencerBenefit(2500, 1, "Sin descuento"); got != "25% en el primer cobro" {
		t.Fatal(got)
	}
	for _, d := range []model.InfluencerWelcomeDetails{{DiscountBPS: 0, DiscountCharges: 3}, {DiscountBPS: 5000, DiscountCharges: 0}} {
		m := InfluencerWelcomeMessage("https://app.puntazo.test/", "zero@example.test", d)
		for _, expected := range []string{"Sin descuento", "Sin comisión", "pausada"} {
			if !strings.Contains(m.Text, expected) {
				t.Fatalf("missing %s", expected)
			}
		}
	}
}

func TestCaptureSenderWritesPrivateMIMEWithoutNetwork(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "mailbox")
	sender := CaptureSender{Directory: directory, FromName: "Puntazo", FromAddress: "hola@puntazo.test"}
	m := InfluencerWelcomeMessage("https://app.puntazo.test", "local@example.test", model.InfluencerWelcomeDetails{Name: "Local", Code: "LOCAL-123", ProgramTypes: []string{"SELLOS"}})
	m.InlineImages = []model.EmailInlineImage{{ContentID: mascotContentID, Filename: "mr-puntazo.png", ContentType: "image/png", Data: mascotPNG}}
	if err := sender.Send(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.eml"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"To: <local@example.test>", "multipart/related", "Content-ID: <" + mascotContentID + ">"} {
		if !strings.Contains(string(body), expected) {
			t.Errorf("MIME missing %s", expected)
		}
	}
	stat, _ := os.Stat(files[0])
	if stat.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", stat.Mode())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sender.Send(ctx, m); err == nil {
		t.Fatal("cancelled send accepted")
	}
}
