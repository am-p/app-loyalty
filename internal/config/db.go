package config

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func (c Config) PoolConfig() (*pgxpool.Config, error) {
	return ParsePoolConfig(c.DatabaseURL)
}

// ParsePoolConfig is shared by the API and migrator. Keep the original pool
// budget until deployment measurements justify a different connection policy.
func ParsePoolConfig(databaseURL string) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pc.MaxConns = 4
	pc.MinConns = 0
	pc.MaxConnLifetime = time.Hour
	pc.MaxConnIdleTime = 30 * time.Minute
	pc.HealthCheckPeriod = time.Minute
	pc.ConnConfig.ConnectTimeout = 5 * time.Second
	return pc, nil
}
