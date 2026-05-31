package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/dispatch"
	"github.com/accept-io/midas/internal/outbox"
)

func TestOutboxDispatcher_PostgresDrainsAndPublishesRows(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	const total = 25
	prefix := fmt.Sprintf("d40g-drain-%d", time.Now().UnixNano())
	events := make([]*outbox.OutboxEvent, 0, total)
	for i := 0; i < total; i++ {
		ev := mustNewOutboxEvent(t,
			outbox.EventDecisionOutcomeRecorded,
			"envelope",
			fmt.Sprintf("%s-%02d", prefix, i),
			"midas.decisions",
			fmt.Sprintf("d40g:%02d", i),
			json.RawMessage(`{"drill":"d40g"}`),
		)
		events = append(events, ev)
		t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })
	}
	if err := repo.AppendBatch(ctx, events); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	publisher := &recordingPublisher{}
	dispatcher, err := dispatch.NewDispatcher(repo, publisher, dispatch.DispatcherConfig{
		BatchSize:    7,
		PollInterval: 10 * time.Millisecond,
		MaxBackoff:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		dispatcher.Run(runCtx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	waitForOutboxPublished(t, db, prefix, total)
	if got := publisher.Count(); got != total {
		t.Fatalf("publisher count=%d, want %d", got, total)
	}
	stats, err := repo.BacklogStats(ctx)
	if err != nil {
		t.Fatalf("BacklogStats: %v", err)
	}
	if stats.UnpublishedCount != 0 {
		t.Fatalf("unpublished count=%d, want 0", stats.UnpublishedCount)
	}
}

func TestOutboxDispatcher_PostgresRestartDrainsRemainingRows(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewOutboxRepo(db)
	if err != nil {
		t.Fatalf("NewOutboxRepo: %v", err)
	}

	const total = 12
	prefix := fmt.Sprintf("d40g-restart-%d", time.Now().UnixNano())
	events := make([]*outbox.OutboxEvent, 0, total)
	for i := 0; i < total; i++ {
		ev := mustNewOutboxEvent(t,
			outbox.EventDecisionEnvelopeClosed,
			"envelope",
			fmt.Sprintf("%s-%02d", prefix, i),
			"midas.decisions",
			fmt.Sprintf("d40g-restart:%02d", i),
			json.RawMessage(`{"drill":"d40g-restart"}`),
		)
		events = append(events, ev)
		t.Cleanup(func() { cleanupOutboxEventByID(t, ev.ID) })
	}
	if err := repo.AppendBatch(ctx, events); err != nil {
		t.Fatalf("AppendBatch: %v", err)
	}

	firstPublisher := &recordingPublisher{maxSuccess: 4}
	first, err := dispatch.NewDispatcher(repo, firstPublisher, dispatch.DispatcherConfig{
		BatchSize:    3,
		PollInterval: 10 * time.Millisecond,
		MaxBackoff:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewDispatcher(first): %v", err)
	}

	firstCtx, firstCancel := context.WithCancel(ctx)
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		first.Run(firstCtx)
	}()
	waitForOutboxPublished(t, db, prefix, 4)
	firstCancel()
	<-firstDone

	if published := countPublishedOutboxByAggregatePrefix(t, db, prefix); published < 4 || published >= total {
		t.Fatalf("published after first dispatcher=%d, want at least 4 and less than %d", published, total)
	}

	secondPublisher := &recordingPublisher{}
	second, err := dispatch.NewDispatcher(repo, secondPublisher, dispatch.DispatcherConfig{
		BatchSize:    5,
		PollInterval: 10 * time.Millisecond,
		MaxBackoff:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewDispatcher(second): %v", err)
	}
	secondCtx, secondCancel := context.WithCancel(ctx)
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		second.Run(secondCtx)
	}()
	defer func() {
		secondCancel()
		<-secondDone
	}()

	waitForOutboxPublished(t, db, prefix, total)
	if got := firstPublisher.Count() + secondPublisher.Count(); got < total {
		t.Fatalf("combined publisher count=%d, want at least %d", got, total)
	}
}

type recordingPublisher struct {
	mu         sync.Mutex
	count      int
	maxSuccess int
}

func (p *recordingPublisher) Publish(ctx context.Context, _ dispatch.Message) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.maxSuccess > 0 && p.count >= p.maxSuccess {
		return fmt.Errorf("d40g test publisher paused after %d successes", p.maxSuccess)
	}
	p.count++
	return nil
}

func (p *recordingPublisher) PublishBatch(ctx context.Context, msgs []dispatch.Message) (dispatch.PublishBatchResult, error) {
	select {
	case <-ctx.Done():
		return dispatch.PublishBatchResult{}, ctx.Err()
	default:
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	result := dispatch.PublishBatchResult{
		Successful: make([]int, 0, len(msgs)),
	}
	var firstErr error
	for i := range msgs {
		if p.maxSuccess > 0 && p.count >= p.maxSuccess {
			err := fmt.Errorf("d40g test publisher paused after %d successes", p.maxSuccess)
			if firstErr == nil {
				firstErr = err
			}
			result.Failed = append(result.Failed, dispatch.PublishBatchFailure{Index: i, Err: err})
			continue
		}
		p.count++
		result.Successful = append(result.Successful, i)
	}
	return result, firstErr
}

func (p *recordingPublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

func waitForOutboxPublished(t *testing.T, db interface {
	QueryRow(query string, args ...any) *sql.Row
}, aggregatePrefix string, want int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := countPublishedOutboxByAggregatePrefix(t, db, aggregatePrefix); got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d published outbox rows, got %d", want, countPublishedOutboxByAggregatePrefix(t, db, aggregatePrefix))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func countPublishedOutboxByAggregatePrefix(t *testing.T, db interface {
	QueryRow(query string, args ...any) *sql.Row
}, aggregatePrefix string) int {
	t.Helper()

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM outbox_events WHERE aggregate_id LIKE $1 AND published_at IS NOT NULL`,
		aggregatePrefix+"%",
	).Scan(&count); err != nil {
		t.Fatalf("count published outbox rows: %v", err)
	}
	return count
}
