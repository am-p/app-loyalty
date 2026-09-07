package config

import (
	"testing"
	"time"
)

func TestPoolPolicySharedByServerAndMigrator(t *testing.T) {
	const url = "postgresql://test@localhost/test?sslmode=disable"
	server, err := (Config{DatabaseURL: url}).PoolConfig()
	if err != nil {
		t.Fatal(err)
	}
	migrator, err := ParsePoolConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if server.MaxConns != 4 || server.MinConns != 0 || server.MaxConnIdleTime != 30*time.Minute {
		t.Fatalf("unexpected API connection budget: max=%d min=%d idle=%s", server.MaxConns, server.MinConns, server.MaxConnIdleTime)
	}
	if migrator.MaxConns != server.MaxConns || migrator.MinConns != server.MinConns || migrator.MaxConnIdleTime != server.MaxConnIdleTime {
		t.Fatal("API and migrator connection budgets differ")
	}
	if server.ConnConfig.ConnectTimeout != 5*time.Second || migrator.ConnConfig.ConnectTimeout != server.ConnConfig.ConnectTimeout {
		t.Fatal("connection timeout must remain bounded at five seconds")
	}
}

func TestPoolConfigReturnsInvalidURLError(t *testing.T) {
	if _, err := ParsePoolConfig("postgresql://%invalid"); err == nil {
		t.Fatal("expected invalid URL error")
	}
}
