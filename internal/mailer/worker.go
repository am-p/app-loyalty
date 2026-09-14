package mailer

import (
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"context"
	"log/slog"
	"time"
)

type Worker struct {
	Repo     *repository.Repository
	Sender   Sender
	Logger   *slog.Logger
	Interval time.Duration
}

func (w Worker) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = 5 * time.Second
	}
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		w.flush(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w Worker) flush(ctx context.Context) {
	items, err := w.Repo.ClaimEmails(ctx, 20)
	if err != nil {
		w.Logger.Error("email outbox claim failed", "error", err)
		return
	}
	for _, item := range items {
		message := model.EmailMessage{To: item.To, Subject: item.Subject, Text: item.Text, HTML: item.HTML}
		if err = w.Sender.Send(ctx, message); err != nil {
			w.Logger.Warn("email delivery failed", "outbox_id", item.ID, "attempt", item.Attempts, "error", err)
			if markErr := w.Repo.MarkEmailFailed(ctx, item.ID, item.Attempts, err); markErr != nil {
				w.Logger.Error("email failure update failed", "outbox_id", item.ID, "error", markErr)
			}
			continue
		}
		if err = w.Repo.MarkEmailSent(ctx, item.ID); err != nil {
			w.Logger.Error("email sent update failed", "outbox_id", item.ID, "error", err)
		}
	}
}
