// deletion-journal keeps the minimum account-deletion record outside database
// backups. Run replay with the API stopped before opening a restored database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"clientesFrecuentes/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type journal struct {
	Environment string    `json:"environment"`
	ExportedAt  time.Time `json:"exported_at"`
	UserIDs     []int64   `json:"user_ids"`
	ObjectKeys  []string  `json:"profile_object_keys"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	flags := flag.NewFlagSet("deletion-journal", flag.ContinueOnError)
	mode := flags.String("mode", "export", "export or replay")
	path := flags.String("file", "", "absolute external journal path")
	environment := flags.String("environment", "", "testing or production; must match journal")
	apply := flags.Bool("apply", false, "actually reapply deletions; default is preview")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !filepath.IsAbs(*path) || (*environment != "testing" && *environment != "production") {
		return errors.New("absolute file and explicit testing/production environment required")
	}
	if *mode != "export" && *mode != "replay" {
		return errors.New("invalid mode")
	}
	j := journal{Environment: *environment}
	data, err := os.ReadFile(*path)
	if err == nil {
		if err = json.Unmarshal(data, &j); err != nil {
			return err
		}
		if j.Environment != *environment {
			return errors.New("journal environment mismatch")
		}
	} else if !os.IsNotExist(err) || *mode == "replay" {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if *mode == "export" {
		ids := map[int64]bool{}
		for _, id := range j.UserIDs {
			if id <= 0 {
				return errors.New("invalid user ID")
			}
			ids[id] = true
		}
		rows, err := pool.Query(ctx, `SELECT user_id FROM account_deletion_journal ORDER BY user_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				return err
			}
			ids[id] = true
		}
		if err = rows.Err(); err != nil {
			return err
		}
		keys := map[string]bool{}
		for _, key := range j.ObjectKeys {
			keys[key] = true
		}
		media, err := pool.Query(ctx, `SELECT object_key FROM profile_media_deletions`)
		if err != nil {
			return err
		}
		defer media.Close()
		for media.Next() {
			var key string
			if err = media.Scan(&key); err != nil {
				return err
			}
			keys[key] = true
		}
		if err = media.Err(); err != nil {
			return err
		}
		j.ObjectKeys = make([]string, 0, len(keys))
		for key := range keys {
			j.ObjectKeys = append(j.ObjectKeys, key)
		}
		sort.Strings(j.ObjectKeys)
		j.UserIDs = make([]int64, 0, len(ids))
		for id := range ids {
			j.UserIDs = append(j.UserIDs, id)
		}
		sort.Slice(j.UserIDs, func(a, b int) bool { return j.UserIDs[a] < j.UserIDs[b] })
		j.ExportedAt = time.Now().UTC()
		data, err = json.Marshal(j)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(*path), 0700); err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(*path), ".deletions-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if err = f.Chmod(0600); err != nil {
			f.Close()
			return err
		}
		if _, err = f.Write(data); err != nil {
			f.Close()
			return err
		}
		if err = f.Sync(); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = os.Rename(f.Name(), *path); err != nil {
			return err
		}
		directory, err := os.Open(filepath.Dir(*path))
		if err != nil {
			return err
		}
		defer directory.Close()
		return directory.Sync()
	}
	repo := repository.New(pool)
	if *apply {
		for _, key := range j.ObjectKeys {
			if _, err = pool.Exec(ctx, `INSERT INTO profile_media_deletions(object_key) VALUES($1) ON CONFLICT DO NOTHING`, key); err != nil {
				return fmt.Errorf("restore media queue: %w", err)
			}
		}
	}
	for _, id := range j.UserIDs {
		if id <= 0 {
			return errors.New("invalid user ID")
		}
		var version int
		err = pool.QueryRow(ctx, `SELECT version FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL`, id).Scan(&version)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if !*apply {
			fmt.Printf("would delete restored account %d\n", id)
			continue
		}
		if _, err = repo.AnonymizeAccount(ctx, id, version); err != nil {
			return fmt.Errorf("restore remains blocked: deletion %d failed: %w", id, err)
		}
	}
	return nil
}
