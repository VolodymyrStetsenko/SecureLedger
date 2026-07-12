package risk

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
)

type fakeOutbox struct {
	deliveries []Delivery
	published  []string
	failed     []string
	retryAt    time.Time
}

func (f *fakeOutbox) ClaimRiskEvents(context.Context, int, time.Time) ([]Delivery, error) {
	return append([]Delivery(nil), f.deliveries...), nil
}

func (f *fakeOutbox) MarkRiskEventPublished(_ context.Context, id string, _ time.Time) error {
	f.published = append(f.published, id)
	return nil
}

func (f *fakeOutbox) MarkRiskEventFailed(_ context.Context, id, _ string, retryAt time.Time) error {
	f.failed = append(f.failed, id)
	f.retryAt = retryAt
	return nil
}

type fakePublisher struct {
	err error
}

func (p fakePublisher) Publish(context.Context, domain.RiskEvent) error {
	return p.err
}

func TestWorkerMarksSuccessfulDeliveryPublished(t *testing.T) {
	t.Parallel()
	outbox := &fakeOutbox{deliveries: []Delivery{{Event: domain.RiskEvent{ID: "risk-1"}, Attempts: 1}}}
	worker := NewWorker(outbox, fakePublisher{}, discardLogger())
	worker.clock = func() time.Time { return time.Unix(10, 0).UTC() }
	count, err := worker.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(outbox.published) != 1 || outbox.published[0] != "risk-1" {
		t.Fatalf("unexpected publish result: count=%d published=%v", count, outbox.published)
	}
}

func TestWorkerSchedulesFailedDelivery(t *testing.T) {
	t.Parallel()
	now := time.Unix(10, 0).UTC()
	outbox := &fakeOutbox{deliveries: []Delivery{{Event: domain.RiskEvent{ID: "risk-1"}, Attempts: 3}}}
	worker := NewWorker(outbox, fakePublisher{err: errors.New("publisher unavailable")}, discardLogger())
	worker.clock = func() time.Time { return now }
	if _, err := worker.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(outbox.failed) != 1 || outbox.retryAt != now.Add(4*time.Second) {
		t.Fatalf("unexpected retry: failed=%v retry_at=%s", outbox.failed, outbox.retryAt)
	}
}

func TestDeliveryBackoffIsBounded(t *testing.T) {
	t.Parallel()
	if got := deliveryBackoff(1); got != time.Second {
		t.Fatalf("first backoff=%s", got)
	}
	if got := deliveryBackoff(100); got > 5*time.Minute {
		t.Fatalf("backoff is not bounded: %s", got)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
