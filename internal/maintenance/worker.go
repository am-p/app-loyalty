package maintenance

import (
	"context"
	"log/slog"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/repository"
)

type Store interface {
	Delete(context.Context, string) error
}

type Observer interface {
	ObserveRetention(kind string, count int64, elapsed time.Duration, err error)
}

type Worker struct {
	Repo     *repository.Repository
	Store    Store
	Logger   *slog.Logger
	Config   config.Config
	Observer Observer
	Now      func() time.Time
}

func (w Worker) Run(ctx context.Context) {
	retentionTicker := time.NewTicker(w.Config.RetentionInterval)
	defer retentionTicker.Stop()
	mediaTicker := time.NewTicker(w.Config.MediaCleanupInterval)
	defer mediaTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-retentionTicker.C:
			w.RunRetention(ctx)
		case <-mediaTicker.C:
			w.RunMediaCleanup(ctx)
		}
	}
}

func (w Worker) RunRetention(ctx context.Context) {
	started := time.Now()
	now := time.Now()
	if w.Now != nil {
		now = w.Now()
	}
	stats, err := w.Repo.ApplyRetention(ctx, repository.RetentionPolicy{Now: now, BatchSize: w.Config.RetentionBatchSize, PreviewRetention: w.Config.PreviewRetention, IdempotencyRetention: w.Config.IdempotencyRetention, SessionRetention: w.Config.SessionRetention, IdentityRetention: w.Config.IdentityTokenRetention, OutboxRedactAfter: w.Config.OutboxRedactAfter, OutboxRetention: w.Config.OutboxRetention})
	if err != nil {
		w.Logger.Error("retention batch failed", "error", err)
	} else {
		w.Logger.Info("retention batch completed", "previews", stats.PreviewsDeleted, "idempotencies", stats.IdempotenciesDeleted, "sessions", stats.SessionsDeleted, "identity_tokens", stats.IdentityTokensDeleted, "outbox_redacted", stats.OutboxRedacted, "outbox_deleted", stats.OutboxDeleted)
	}
	if w.Observer != nil {
		w.Observer.ObserveRetention("database", stats.PreviewsDeleted+stats.IdempotenciesDeleted+stats.SessionsDeleted+stats.IdentityTokensDeleted+stats.OutboxRedacted+stats.OutboxDeleted, time.Since(started), err)
	}
}

func (w Worker) RunMediaCleanup(ctx context.Context) {
	if w.Store == nil {
		return
	}
	started := time.Now()
	items, err := w.Repo.DueBrandMedia(ctx, w.Config.RetentionBatchSize)
	if err != nil {
		w.Logger.Error("media cleanup claim failed", "error", err)
		if w.Observer != nil {
			w.Observer.ObserveRetention("media", 0, time.Since(started), err)
		}
		return
	}
	var completed int64
	for _, item := range items {
		failure := ""
		if err = w.Store.Delete(ctx, item.ObjectKey); err != nil {
			failure = err.Error()
		}
		if completionErr := w.Repo.CompleteBrandMediaDeletion(ctx, item.ID, failure); completionErr != nil {
			w.Logger.Error("media cleanup completion failed", "image_id", item.ID, "error", completionErr)
			err = completionErr
		} else if failure == "" {
			completed++
		}
	}
	if w.Observer != nil {
		w.Observer.ObserveRetention("media", completed, time.Since(started), err)
	}
}
