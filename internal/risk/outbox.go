package risk

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
)

type Delivery struct {
	Event    domain.RiskEvent
	Attempts int
}

type Outbox interface {
	ClaimRiskEvents(context.Context, int, time.Time) ([]Delivery, error)
	MarkRiskEventPublished(context.Context, string, time.Time) error
	MarkRiskEventFailed(context.Context, string, string, time.Time) error
}

type Publisher interface {
	Publish(context.Context, domain.RiskEvent) error
}

type LogPublisher struct {
	log *slog.Logger
}

func NewLogPublisher(log *slog.Logger) *LogPublisher {
	return &LogPublisher{log: log}
}

func (p *LogPublisher) Publish(_ context.Context, event domain.RiskEvent) error {
	p.log.Info("risk event published",
		"risk_event_id", event.ID,
		"type", event.Type,
		"severity", event.Severity,
		"transfer_id", event.TransferID,
	)
	return nil
}

type Worker struct {
	outbox    Outbox
	publisher Publisher
	log       *slog.Logger
	batchSize int
	interval  time.Duration
	clock     func() time.Time
}

func NewWorker(outbox Outbox, publisher Publisher, log *slog.Logger) *Worker {
	return &Worker{
		outbox: outbox, publisher: publisher, log: log,
		batchSize: 32, interval: time.Second, clock: time.Now,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			w.log.Warn("risk outbox cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOnce(ctx context.Context) (int, error) {
	now := w.clock().UTC()
	deliveries, err := w.outbox.ClaimRiskEvents(ctx, w.batchSize, now)
	if err != nil {
		return 0, fmt.Errorf("claim risk events: %w", err)
	}
	for _, delivery := range deliveries {
		if err := w.publisher.Publish(ctx, delivery.Event); err != nil {
			reason := truncateError(err)
			retryAt := now.Add(deliveryBackoff(delivery.Attempts))
			if markErr := w.outbox.MarkRiskEventFailed(ctx, delivery.Event.ID, reason, retryAt); markErr != nil {
				return 0, fmt.Errorf("mark risk event %s failed: %w", delivery.Event.ID, markErr)
			}
			continue
		}
		if err := w.outbox.MarkRiskEventPublished(ctx, delivery.Event.ID, now); err != nil {
			return 0, fmt.Errorf("mark risk event %s published: %w", delivery.Event.ID, err)
		}
	}
	return len(deliveries), nil
}

func deliveryBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 8 {
		attempts = 8
	}
	delay := time.Duration(1<<(attempts-1)) * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func truncateError(err error) string {
	runes := []rune(err.Error())
	if len(runes) > 500 {
		runes = runes[:500]
	}
	return string(runes)
}
