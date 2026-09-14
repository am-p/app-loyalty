package mediaworker

import (
	"context"
	"log/slog"
	"time"

	"clientesFrecuentes/internal/repository"
)

type Store interface {
	Delete(context.Context, string) error
}
type Worker struct {
	Repo     *repository.Repository
	Store    Store
	Logger   *slog.Logger
	Interval time.Duration
}

func (w Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.run(ctx)
		}
	}
}
func (w Worker) run(ctx context.Context) {
	items, err := w.Repo.DueBrandMedia(ctx, 50)
	if err != nil {
		w.Logger.Error("media cleanup claim failed", "error", err)
		return
	}
	for _, item := range items {
		failure := ""
		if err = w.Store.Delete(ctx, item.ObjectKey); err != nil {
			failure = err.Error()
		}
		if err = w.Repo.CompleteBrandMediaDeletion(ctx, item.ID, failure); err != nil {
			w.Logger.Error("media cleanup completion failed", "image_id", item.ID, "error", err)
		}
	}
}
