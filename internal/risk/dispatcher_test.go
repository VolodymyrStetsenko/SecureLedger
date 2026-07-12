package risk

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestDispatcherIsBounded(t *testing.T) {
	t.Parallel()
	d := NewDispatcher(nil, 1)
	if !d.Submit(domain.RiskEvent{ID: "risk-1"}) {
		t.Fatal("first event should fit")
	}
	if d.Submit(domain.RiskEvent{ID: "risk-2"}) {
		t.Fatal("second event should be dropped when the queue is full")
	}
}

func TestDispatcherRunsUntilCancellation(t *testing.T) {
	t.Parallel()
	var output lockedBuffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	d := NewDispatcher(logger, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()

	d.Submit(domain.RiskEvent{ID: "risk-1", Type: "high_value_transfer", Severity: "medium", TransferID: "tx-1"})
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(output.String(), "risk event dispatched") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(output.String(), "risk_event_id=risk-1") {
		t.Fatalf("dispatch was not logged: %s", output.String())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not stop after cancellation")
	}
}
