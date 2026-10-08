package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetention(t *testing.T) {
	dir := t.TempDir()
	d, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	d.Now = func() time.Time { return now }
	for _, name := range []string{"api-2026-07-01.jsonl", "api-2026-10-03.jsonl", "unrelated.log"} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err = d.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "api-2026-07-01.jsonl")); !os.IsNotExist(err) {
		t.Fatal("old log remains")
	}
	for _, name := range []string{"api-2026-10-03.jsonl", "unrelated.log"} {
		if _, err = os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = d.Write([]byte("{}\n")); err != nil {
		t.Fatal(err)
	}
}
