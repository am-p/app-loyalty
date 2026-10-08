package main

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreReplay(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "journal_" + time.Now().Format("150405000000000")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	dsn = u.String()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE schema_migrations(version CHAR(4) PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, filepath.Base(file)[:4]); err != nil {
			t.Fatal(err)
		}
	}
	var id int64
	if err = pool.QueryRow(ctx, `INSERT INTO usuarios(email,nombre,tipo_cuenta,qr_hash) VALUES('restore@example.test','Restored','CLIENTE_FINAL',decode(repeat('01',32),'hex')) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", dsn)
	path := filepath.Join(t.TempDir(), "journal.json")
	data, _ := json.Marshal(journal{Environment: "testing", UserIDs: []int64{id}, ObjectKeys: []string{"profiles/restored/avatar.jpg"}})
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-mode", "replay", "-environment", "testing", "-file", path}
	if err = run(args); err != nil {
		t.Fatal(err)
	}
	var active bool
	if err = pool.QueryRow(ctx, `SELECT activo FROM usuarios WHERE id=$1`, id).Scan(&active); err != nil || !active {
		t.Fatal("preview changed account", err)
	}
	if err = run(append(args, "-apply")); err != nil {
		t.Fatal(err)
	}
	if err = run(append(args, "-apply")); err != nil {
		t.Fatal("replay must be idempotent", err)
	}
	var queued int
	if err = pool.QueryRow(ctx, `SELECT activo,(SELECT count(*) FROM profile_media_deletions) FROM usuarios WHERE id=$1`, id).Scan(&active, &queued); err != nil || active || queued != 1 {
		t.Fatalf("restored active=%t queue=%d err=%v", active, queued, err)
	}
	if err = run([]string{"-mode", "export", "-environment", "testing", "-file", path}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var j journal
	if err = json.Unmarshal(data, &j); err != nil || len(j.UserIDs) != 1 {
		t.Fatal("export lost deletion", err)
	}
	if err = run([]string{"-mode", "replay", "-environment", "production", "-file", path}); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatal("environment mismatch was accepted", err)
	}
}
