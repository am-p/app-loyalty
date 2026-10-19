package mailer

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
)

func TestMessagesContainBoundedActionLinks(t *testing.T) {
	verification := VerificationMessage("https://app.puntazo.test", "user@example.com", "secret-token")
	if !strings.Contains(verification.Text, "https://app.puntazo.test/verify-email?token=secret-token") || !strings.Contains(verification.Text, "24 horas") {
		t.Fatalf("verification=%+v", verification)
	}
	reset := PasswordResetMessage("https://app.puntazo.test", "user@example.com", "secret-token")
	if !strings.Contains(reset.Text, "/reset-password?token=secret-token") || !strings.Contains(reset.Text, "1 hora") || !strings.Contains(reset.HTML, "Creá una contraseña nueva") || !strings.Contains(reset.HTML, "cid:"+mascotContentID) || strings.Contains(reset.HTML, "%!") {
		t.Fatalf("reset=%+v", reset)
	}
	invitation := BrandInvitationMessage("https://app.puntazo.test", "operator@example.com", "invite-token", BrandInvitationDetails{BrandName: "Café Aurora", Role: "OPERADOR", Branches: []string{"Belgrano", "Palermo"}, BrandLogoURL: "https://media.puntazo.test/logo.png"})
	for _, expected := range []string{"Café Aurora quiere sumarte", "Operador", "Belgrano, Palermo", "https://media.puntazo.test/logo.png", "/invitaciones/aceptar?token=invite-token", "todavía no se haya registrado"} {
		if !strings.Contains(invitation.HTML, expected) {
			t.Fatalf("invitation HTML missing %q", expected)
		}
	}
	if strings.Contains(invitation.HTML, "%!") {
		t.Fatalf("invitation HTML contains formatting error")
	}
}

func TestEncodeMessageEmbedsInlineImagesAsRelatedParts(t *testing.T) {
	encoded := string(encodeMessage("Puntazo", "hola@puntazo.pro", model.EmailMessage{
		To:      "user@example.com",
		Subject: "Prueba",
		Text:    "texto",
		HTML:    `<img src="cid:mr-puntazo@puntazo">`,
		InlineImages: []model.EmailInlineImage{{
			ContentID: "mr-puntazo@puntazo", ContentType: "image/png", Filename: "mr-puntazo.png", Data: []byte("png"),
		}},
	}))
	for _, expected := range []string{"multipart/related", "multipart/alternative", "Content-ID: <mr-puntazo@puntazo>", "Content-Disposition: inline", "cG5n"} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("encoded message missing %q", expected)
		}
	}
}

func TestApprovedTemplatesEscapeDynamicContent(t *testing.T) {
	reset := PasswordResetMessage("https://app.puntazo.test", `<script>@example.com`, `token&unsafe`)
	if strings.Contains(reset.HTML, `<script>`) || !strings.Contains(reset.HTML, `token%26unsafe`) {
		t.Fatalf("unsafe reset HTML=%s", reset.HTML)
	}
	invitation := BrandInvitationMessage("https://app.puntazo.test", "operator@example.com", "token", BrandInvitationDetails{BrandName: `<img src=x onerror=alert(1)>`, Role: "OPERADOR"})
	if strings.Contains(invitation.HTML, `<img src=x`) || !strings.Contains(invitation.HTML, `&lt;img`) {
		t.Fatalf("unsafe invitation HTML=%s", invitation.HTML)
	}
}
func TestSMTPRejectsHeaderInjectionBeforeDial(t *testing.T) {
	sender := NewSMTP(config.Config{})
	err := sender.Send(context.Background(), model.EmailMessage{To: "safe@example.com\r\nBcc: bad@example.com", Subject: "test"})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("error=%v", err)
	}
}

func TestVerificationCodeMessage(t *testing.T) {
	message := VerificationCodeMessage("user@example.test", "000001")
	if !strings.Contains(message.Text, "000001") || !strings.Contains(message.HTML, "000001") || strings.Contains(message.Subject, "000001") || strings.Contains(message.Text, "token=") || message.Token != "code:000001" {
		t.Fatal("verification code template")
	}
}

func TestVerificationCodeBrandingSurvivesMIMEEncoding(t *testing.T) {
	message := VerificationCodeMessage("user@example.test", "001234")
	for _, fragment := range []string{"cid:" + puntazoLogoContentID, "cid:" + mascotContentID, "Logo de Puntazo", "Mr. Puntazo feliz", "001234", "10 minutos"} {
		if !strings.Contains(message.HTML, fragment) {
			t.Fatalf("verification HTML missing %q", fragment)
		}
	}
	if strings.Contains(message.HTML, "%!") || strings.Contains(message.HTML, `src="https://`) {
		t.Fatal("broken or remotely loaded verification branding")
	}
	encoded := encodeMessage("Puntazo", "hola@puntazo.pro", message)
	envelope, err := mail.ReadMessage(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err := mime.ParseMediaType(envelope.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/related" {
		t.Fatal("missing related images envelope", err)
	}
	reader := multipart.NewReader(envelope.Body, params["boundary"])
	images := map[string]bool{}
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		id := part.Header.Get("Content-ID")
		if id == "" {
			continue
		}
		decoded, e := io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		if e != nil {
			t.Fatal(e)
		}
		image, e := png.DecodeConfig(bytes.NewReader(decoded))
		if e != nil || image.Width < 40 || image.Height < 40 {
			t.Fatal("invalid embedded PNG", id, e)
		}
		if images[id] {
			t.Fatal("duplicate CID", id)
		}
		images[id] = true
	}
	if len(images) != 2 || !images["<"+puntazoLogoContentID+">"] || !images["<"+mascotContentID+">"] {
		t.Fatalf("inline images=%v", images)
	}
	if !strings.Contains(message.Text, "001234") || strings.Contains(message.Subject, "001234") {
		t.Fatal("plaintext or subject regression")
	}
}

func TestVerificationCodeDeliveryHasActionAndNoAttachments(t *testing.T) {
	m := VerificationCodeDelivery("https://app.puntazo.test/", "user@example.test", "001234", "secret&token")
	if len(m.InlineImages) != 0 {
		t.Fatal("verification still attaches images")
	}
	for _, want := range []string{"https://app.puntazo.test/api/v1/email-assets/v1/puntazo-logo", "https://app.puntazo.test/api/v1/email-assets/v1/mr-puntazo", "Activar mi cuenta", "token=secret%26token", "001234", "10 minutos"} {
		if !strings.Contains(m.HTML, want) {
			t.Fatalf("missing %q", want)
		}
	}
	raw := string(encodeMessage("Puntazo", "hola@puntazo.pro", m))
	if strings.Contains(raw, "Content-Disposition:") || strings.Contains(m.HTML, "cid:") || strings.Contains(m.Subject, "secret") || !strings.Contains(m.Text, "token=secret%26token") {
		t.Fatal("delivery encoding or plaintext")
	}
	old := VerificationCodeDelivery("https://app.puntazo.test", "user@example.test", "001234", "")
	if strings.Contains(old.HTML, "Activar mi cuenta") {
		t.Fatal("old queued email has unissued link")
	}
}
