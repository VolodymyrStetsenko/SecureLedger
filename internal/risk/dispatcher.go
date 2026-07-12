package risk

import (
	"context"
	"io"
	"log/slog"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
)

type Dispatcher struct {
	queue chan domain.RiskEvent
	log   *slog.Logger
}

func NewDispatcher(log *slog.Logger, capacity int) *Dispatcher {
	if capacity <= 0 {
		capacity = 128
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Dispatcher{queue: make(chan domain.RiskEvent, capacity), log: log}
}

func (d *Dispatcher) Submit(event domain.RiskEvent) bool {
	select {
	case d.queue <- event:
		return true
	default:
		d.log.Warn("risk notification dropped", "risk_event_id", event.ID, "transfer_id", event.TransferID)
		return false
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-d.queue:
			d.log.Info("risk event dispatched",
				"risk_event_id", event.ID,
				"type", event.Type,
				"severity", event.Severity,
				"transfer_id", event.TransferID,
			)
		}
	}
}
