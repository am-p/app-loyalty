package mailer

import (
	"context"
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
	if !strings.Contains(reset.Text, "/reset-password?token=secret-token") || !strings.Contains(reset.Text, "1 hora") {
		t.Fatalf("reset=%+v", reset)
	}
}
func TestSMTPRejectsHeaderInjectionBeforeDial(t *testing.T) {
	sender := NewSMTP(config.Config{})
	err := sender.Send(context.Background(), model.EmailMessage{To: "safe@example.com\r\nBcc: bad@example.com", Subject: "test"})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("error=%v", err)
	}
}
