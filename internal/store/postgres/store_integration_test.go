//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
)

func TestPostgresRepositoryLifecycle(t *testing.T) {
	s := openIntegrationStore(t)
	resetDatabase(t, s)
	ctx := context.Background()
	createIntegrationAccount(t, s, "a", "alice", 1000)
	createIntegrationAccount(t, s, "b", "bob", 0)

	input := integrationTransfer("tx-1", "key-lifecycle-1", "a", "b", 250, "alice")
	input.Risk = &domain.RiskEvent{
		ID: "risk-1", Type: "high_value_transfer", Severity: "medium",
		TransferID: "tx-1", Reason: "integration test", CreatedAt: input.Transfer.CreatedAt,
	}
	first, err := s.ApplyTransfer(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed {
		t.Fatal("first transfer was marked as replayed")
	}
	replay, err := s.ApplyTransfer(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Transfer.ID != first.Transfer.ID {
		t.Fatalf("unexpected replay: %+v", replay)
	}

	assertIntegrationBalance(t, s, "a", 750)
	assertIntegrationBalance(t, s, "b", 250)
	journal, err := s.ListJournal(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(journal) != 4 {
		t.Fatalf("journal entries=%d want=4", len(journal))
	}
	audit, err := s.ListAudit(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 3 {
		t.Fatalf("audit records=%d want=3", len(audit))
	}
	risks, err := s.ListRiskEvents(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(risks) != 1 || risks[0].TransferID != "tx-1" {
		t.Fatalf("risk events=%+v", risks)
	}
	deliveries, err := s.ClaimRiskEvents(ctx, 10, time.Unix(3, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].Event.ID != "risk-1" || deliveries[0].Attempts != 1 {
		t.Fatalf("outbox deliveries=%+v", deliveries)
	}
	if err := s.MarkRiskEventFailed(ctx, "risk-1", "publisher unavailable", time.Unix(5, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	deliveries, err = s.ClaimRiskEvents(ctx, 10, time.Unix(4, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("event was retried before available_at: %+v", deliveries)
	}
	deliveries, err = s.ClaimRiskEvents(ctx, 10, time.Unix(5, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].Attempts != 2 {
		t.Fatalf("failed event was not retried: %+v", deliveries)
	}
	if err := s.MarkRiskEventPublished(ctx, "risk-1", time.Unix(6, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	deliveries, err = s.ClaimRiskEvents(ctx, 10, time.Unix(7, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("published event was claimed again: %+v", deliveries)
	}
	report, err := s.Reconcile(ctx, time.Unix(8, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Clean() || report.AccountsChecked != 3 {
		t.Fatalf("unexpected reconciliation report: %+v", report)
	}

	conflict := integrationTransfer("tx-2", "key-lifecycle-1", "a", "b", 251, "alice")
	if _, err := s.ApplyTransfer(ctx, conflict); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error=%v", err)
	}
}

func TestPostgresReconciliationDetectsBalanceDrift(t *testing.T) {
	s := openIntegrationStore(t)
	resetDatabase(t, s)
	createIntegrationAccount(t, s, "account", "alice", 100)

	if _, err := s.pool.Exec(context.Background(), `
		UPDATE accounts SET balance_minor = balance_minor + 1 WHERE id = 'account'`); err != nil {
		t.Fatal(err)
	}
	report, err := s.Reconcile(context.Background(), time.Unix(7, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if report.Clean() || len(report.BalanceDifferences) != 1 {
		t.Fatalf("balance drift not detected: %+v", report)
	}
	difference := report.BalanceDifferences[0]
	if difference.AccountID != "account" || difference.StoredBalanceMinor != 101 || difference.JournalBalanceMinor != 100 {
		t.Fatalf("unexpected balance difference: %+v", difference)
	}
}

func TestConcurrentPostgresTransfersCannotOverspend(t *testing.T) {
	s := openIntegrationStore(t)
	resetDatabase(t, s)
	createIntegrationAccount(t, s, "source", "alice", 100)
	createIntegrationAccount(t, s, "destination-1", "bob", 0)
	createIntegrationAccount(t, s, "destination-2", "carol", 0)

	inputs := []store.ApplyTransferInput{
		integrationTransfer("tx-1", "key-concurrent-1", "source", "destination-1", 80, "alice"),
		integrationTransfer("tx-2", "key-concurrent-2", "source", "destination-2", 80, "alice"),
	}
	var wg sync.WaitGroup
	errorsByCall := make([]error, len(inputs))
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errorsByCall[i] = s.ApplyTransfer(context.Background(), inputs[i])
		}(i)
	}
	wg.Wait()

	var successes, insufficient int
	for _, err := range errorsByCall {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrInsufficientFunds):
			insufficient++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || insufficient != 1 {
		t.Fatalf("successes=%d insufficient=%d", successes, insufficient)
	}

	source, err := s.GetAccount(context.Background(), "source")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := s.GetAccount(context.Background(), "destination-1")
	second, _ := s.GetAccount(context.Background(), "destination-2")
	if source.BalanceMinor != 20 || first.BalanceMinor+second.BalanceMinor != 80 {
		t.Fatalf("unexpected balances: source=%d destinations=%d", source.BalanceMinor, first.BalanceMinor+second.BalanceMinor)
	}
}

func openIntegrationStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func resetDatabase(t *testing.T, s *Store) {
	t.Helper()
	_, err := s.pool.Exec(context.Background(), `
		TRUNCATE risk_events, audit_records, transfer_intents, postings,
		         journal_transactions, accounts RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
}

func createIntegrationAccount(t *testing.T, s *Store, id, owner string, balance int64) {
	t.Helper()
	now := time.Unix(1, 0).UTC()
	err := s.CreateAccount(context.Background(), store.CreateAccountInput{
		Account:              domain.Account{ID: id, OwnerID: owner, Currency: "GBP", BalanceMinor: balance, CreatedAt: now},
		OpeningTransactionID: "open-" + id,
		OpeningPostingIDs:    [2]string{"opening-1-" + id, "opening-2-" + id},
		Audit: domain.AuditRecord{
			ID: "audit-" + id, ActorID: "operator", Action: "account.create",
			ResourceID: id, Outcome: "success", CreatedAt: now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func integrationTransfer(id, key, from, to string, amount int64, actor string) store.ApplyTransferInput {
	now := time.Unix(2, 0).UTC()
	return store.ApplyTransferInput{
		Transfer: domain.Transfer{
			ID: id, IdempotencyKey: key, ActorID: actor, Currency: "GBP", CreatedAt: now,
			Intent: domain.TransferIntent{FromAccountID: from, ToAccountID: to, AmountMinor: amount},
		},
		PostingIDs: [2]string{"posting-1-" + id, "posting-2-" + id},
		Audit: domain.AuditRecord{
			ID: fmt.Sprintf("audit-%s", id), ActorID: actor, Action: "transfer.create",
			ResourceID: id, Outcome: "success", CreatedAt: now,
		},
	}
}

func assertIntegrationBalance(t *testing.T, s *Store, id string, want int64) {
	t.Helper()
	account, err := s.GetAccount(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if account.BalanceMinor != want {
		t.Fatalf("account %s balance=%d want=%d", id, account.BalanceMinor, want)
	}
}
