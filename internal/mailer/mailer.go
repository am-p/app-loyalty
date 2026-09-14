package mailer

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"
	"sync"

	"clientesFrecuentes/internal/model"
)

type Sender interface {
	Send(context.Context, model.EmailMessage) error
}

type MemorySender struct {
	mu       sync.Mutex
	messages []model.EmailMessage
	Err      error
}

func (m *MemorySender) Send(_ context.Context, message model.EmailMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.messages = append(m.messages, message)
	return nil
}
func (m *MemorySender) Messages() []model.EmailMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]model.EmailMessage(nil), m.messages...)
}

func VerificationMessage(appURL, to, token string) model.EmailMessage {
	link := actionURL(appURL, "/verify-email", token)
	return transactional("VERIFY_EMAIL", to, "Verificá tu correo en Puntazo", "Verificá tu correo para activar tu cuenta: "+link+"\n\nEl enlace vence en 24 horas.", "Verificá tu correo", "Activar mi cuenta", link, "Este enlace vence en 24 horas.")
}
func PasswordResetMessage(appURL, to, token string) model.EmailMessage {
	link := actionURL(appURL, "/reset-password", token)
	return transactional("RESET_PASSWORD", to, "Restablecé tu contraseña de Puntazo", "Usá este enlace para elegir una contraseña nueva: "+link+"\n\nEl enlace vence en 1 hora.", "Restablecé tu contraseña", "Elegir contraseña nueva", link, "Este enlace vence en 1 hora. Si no lo pediste, ignorá este correo.")
}
func actionURL(base, path, token string) string {
	return strings.TrimRight(base, "/") + path + "?token=" + url.QueryEscape(token)
}
func transactional(kind, to, subject, text, title, action, link, note string) model.EmailMessage {
	h := fmt.Sprintf(`<!doctype html><html lang="es"><body><h1>%s</h1><p>%s</p><p><a href="%s">%s</a></p><p>%s</p></body></html>`, html.EscapeString(title), html.EscapeString(note), html.EscapeString(link), html.EscapeString(action), html.EscapeString(note))
	return model.EmailMessage{Kind: kind, To: to, Subject: subject, Text: text, HTML: h}
}
