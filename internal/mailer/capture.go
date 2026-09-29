package mailer

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"clientesFrecuentes/internal/model"
	"github.com/google/uuid"
)

// CaptureSender is a development mailbox. It writes private MIME messages
// locally and never contacts a mail server. Config rejects it in production.
type CaptureSender struct{ Directory, FromName, FromAddress string }

func (s CaptureSender) Send(ctx context.Context, message model.EmailMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(s.Directory) {
		return errors.New("mail capture directory must be absolute")
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.Directory, ".mail-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(encodeMessage(s.FromName, s.FromAddress, message)); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(s.Directory, uuid.NewString()+".eml"))
}
