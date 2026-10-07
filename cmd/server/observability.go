package main

import (
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/password"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

// Capacity metrics are bounded aggregate operational logs without actor/email/IP
// labels. They are never exposed on a public HTTP route.
func logCapacity(ctx context.Context, pool *pgxpool.Pool, uploads *middleware.UploadSemaphore, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := pool.Stat()
			active, rejected := password.Stats()
			uploadActive, uploadActors := uploads.Stats()
			logger.Info("capacity", "db_acquired", stats.AcquiredConns(), "db_idle", stats.IdleConns(), "db_total", stats.TotalConns(), "db_max", stats.MaxConns(), "db_acquire_count", stats.AcquireCount(), "db_acquire_duration_ms", stats.AcquireDuration().Milliseconds(), "db_empty_acquire", stats.EmptyAcquireCount(), "db_canceled_acquire", stats.CanceledAcquireCount(), "bcrypt_active", active, "bcrypt_rejected", rejected, "upload_active", uploadActive, "upload_actor_gates", uploadActors)
		}
	}
}
