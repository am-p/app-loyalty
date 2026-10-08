// Package logging stores API logs in daily files with a bounded lifetime.
package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Daily struct {
	Directory string
	mu        sync.Mutex
	Now       func() time.Time
}

func New(directory string) (*Daily, error) {
	if !filepath.IsAbs(directory) {
		return nil, fmt.Errorf("LOG_DIRECTORY must be absolute")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	d := &Daily{Directory: directory, Now: time.Now}
	return d, d.Cleanup()
}
func (d *Daily) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	path := filepath.Join(d.Directory, "api-"+d.Now().UTC().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	n, err := f.Write(p)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return n, err
}
func (d *Daily) Cleanup() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	entries, err := os.ReadDir(d.Directory)
	if err != nil {
		return err
	}
	cutoff := d.Now().UTC().Add(-60 * 24 * time.Hour)
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "api-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day, err := time.Parse("2006-01-02", strings.TrimSuffix(strings.TrimPrefix(name, "api-"), ".jsonl"))
		if err != nil {
			continue
		}
		// Remove the whole oldest day: no message exceeds 60 days.
		if day.Before(cutoff) {
			if err = os.Remove(filepath.Join(d.Directory, name)); err != nil {
				return err
			}
		}
	}
	return nil
}
func (d *Daily) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = d.Cleanup()
		}
	}
}
