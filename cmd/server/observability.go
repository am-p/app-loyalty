package main

import (
	"clientesFrecuentes/internal/cardevents"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/password"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
)

// Capacity metrics are bounded aggregate operational logs without actor/email/IP
// labels. They are never exposed on a public HTTP route.
func logCapacity(ctx context.Context, pool *pgxpool.Pool, uploads *middleware.UploadSemaphore, limiter *middleware.RateLimiter, events *cardevents.Broker, logger *slog.Logger) {
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
			redisStats := limiter.RedisStats()
			sse := events.Stats()
			logger.Info("capacity", "db_acquired", stats.AcquiredConns(), "db_idle", stats.IdleConns(), "db_total", stats.TotalConns(), "db_max", stats.MaxConns(), "db_acquire_count", stats.AcquireCount(), "db_acquire_duration_ms", stats.AcquireDuration().Milliseconds(), "db_empty_acquire", stats.EmptyAcquireCount(), "db_canceled_acquire", stats.CanceledAcquireCount(), "bcrypt_active", active, "bcrypt_rejected", rejected, "upload_active", uploadActive, "upload_actor_gates", uploadActors, "redis_calls", redisStats.Calls, "redis_errors", redisStats.Errors, "redis_duration_ms", redisStats.Duration.Milliseconds(), "sse_active", sse.Active, "sse_ready", sse.Ready, "sse_enqueued", sse.Enqueued, "sse_coalesced", sse.Coalesced, "sse_rejected", sse.Rejected, "sse_disconnects", sse.Disconnects, "sse_delivered", sse.Delivered, "sse_write_failures", sse.WriteFailures)
		}
	}
}
