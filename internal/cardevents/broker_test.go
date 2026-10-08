package cardevents

import (
	"github.com/jackc/pgx/v5"
	"io"
	"log/slog"
	"testing"
)

func TestAggregateStatsCountCoalescingAndBoundedStreams(t *testing.T) {
	b := New(&pgx.ConnConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.setReady()
	events, _, release, e := b.Subscribe(1)
	if e != nil {
		t.Fatal(e)
	}
	b.publish(1)
	b.publish(1)
	<-events
	b.RecordDelivery(true)
	b.RecordDelivery(false)
	var releases []func()
	for i := 0; i < 3; i++ {
		_, _, r, e := b.Subscribe(1)
		if e != nil {
			t.Fatal(e)
		}
		releases = append(releases, r)
	}
	if _, _, _, e := b.Subscribe(1); e != ErrCapacity {
		t.Fatal(e)
	}
	stats := b.Stats()
	if stats.Active != 4 || stats.Enqueued != 1 || stats.Coalesced != 1 || stats.Rejected != 1 || stats.Delivered != 1 || stats.WriteFailures != 1 {
		t.Fatalf("%+v", stats)
	}
	for _, r := range releases {
		r()
	}
	release()
	b.setDown()
	stats = b.Stats()
	if stats.Active != 0 || stats.Ready || stats.Disconnects != 1 {
		t.Fatalf("%+v", stats)
	}
}
