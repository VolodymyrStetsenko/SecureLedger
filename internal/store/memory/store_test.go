package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
)

func TestApplyTransferIsIdempotent(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	create(t, s, "a", "alice", 1000)
	create(t, s, "b", "bob", 0)

	in := transferInput("tx1", "key-12345678", "a", "b", 250, "alice")
	first, err := s.ApplyTransfer(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ApplyTransfer(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || !second.Replayed || first.Transfer.ID != second.Transfer.ID {
		t.Fatalf("unexpected replay results: first=%+v second=%+v", first, second)
	}
	assertBalance(t, s, "a", 750)
	assertBalance(t, s, "b", 250)
	journal, _ := s.ListJournal(ctx, 100)
	// 2 opening postings + 2 transfer postings.
	if len(journal) != 4 {
		t.Fatalf("got %d journal entries, want 4", len(journal))
	}
}

func TestIdempotencyConflict(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	create(t, s, "a", "alice", 1000)
	create(t, s, "b", "bob", 0)
	if _, err := s.ApplyTransfer(ctx, transferInput("tx1", "key-12345678", "a", "b", 250, "alice")); err != nil {
		t.Fatal(err)
	}
	changed := transferInput("tx2", "key-12345678", "a", "b", 251, "alice")
	if _, err := s.ApplyTransfer(ctx, changed); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestIdempotencyKeysAreScopedByActor(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	create(t, s, "a", "alice", 1000)
	create(t, s, "b", "bob", 0)

	first := transferInput("tx1", "shared-key", "a", "b", 100, "operator-1")
	if _, err := s.ApplyTransfer(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := transferInput("tx2", "shared-key", "a", "b", 100, "operator-2")
	if _, err := s.ApplyTransfer(ctx, second); err != nil {
		t.Fatalf("same key from a different actor should be independent: %v", err)
	}
	assertBalance(t, s, "a", 800)
	assertBalance(t, s, "b", 200)
}

func TestCreateAccountValidationIsAtomic(t *testing.T) {
	t.Parallel()
	s := New()
	now := time.Unix(1, 0).UTC()
	err := s.CreateAccount(context.Background(), store.CreateAccountInput{
		Account:              domain.Account{ID: "a", OwnerID: "alice", Currency: "GBP", BalanceMinor: 100, CreatedAt: now},
		OpeningTransactionID: "open-a",
		OpeningPostingIDs:    [2]string{"duplicate", "duplicate"},
		Audit:                domain.AuditRecord{ID: "audit-a", ActorID: "operator", Action: "account.create", ResourceID: "a", Outcome: "success", CreatedAt: now},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if len(s.accounts) != 0 || len(s.journal) != 0 || len(s.audit) != 0 || s.sequence != 0 {
		t.Fatalf("failed create changed state: accounts=%d journal=%d audit=%d sequence=%d", len(s.accounts), len(s.journal), len(s.audit), s.sequence)
	}
}

func TestRepositoryHonoursCancelledContext(t *testing.T) {
	t.Parallel()
	s := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetAccount(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestCreateAccountRejectsReservedSystemIdentity(t *testing.T) {
	t.Parallel()
	s := New()
	now := time.Unix(1, 0).UTC()
	err := s.CreateAccount(context.Background(), store.CreateAccountInput{
		Account: domain.Account{ID: "system:equity:GBP", OwnerID: "attacker", Currency: "GBP", CreatedAt: now},
		Audit:   domain.AuditRecord{ID: "audit-a", ActorID: "operator", Action: "account.create", ResourceID: "system:equity:GBP", Outcome: "success", CreatedAt: now},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected reserved identity rejection, got %v", err)
	}
}

func TestConcurrentTransfersCannotOverspend(t *testing.T) {
	t.Parallel()
	s := New()
	ctx := context.Background()
	create(t, s, "a", "alice", 100)
	create(t, s, "b", "bob", 0)

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := transferInput(
				fmt.Sprintf("tx-%d", i),
				fmt.Sprintf("key-%08d", i),
				"a", "b", 1, "alice",
			)
			if _, err := s.ApplyTransfer(ctx, in); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !errors.Is(err, domain.ErrInsufficientFunds) {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if successes != 100 {
		t.Fatalf("got %d successful transfers, want 100", successes)
	}
	assertBalance(t, s, "a", 0)
	assertBalance(t, s, "b", 100)
}

func create(t *testing.T, s *Store, id, owner string, balance int64) {
	t.Helper()
	now := time.Unix(1, 0).UTC()
	err := s.CreateAccount(context.Background(), store.CreateAccountInput{
		Account: domain.Account{
			ID: id, OwnerID: owner, Currency: "GBP", BalanceMinor: balance, CreatedAt: now,
		},
		OpeningTransactionID: "open-" + id,
		OpeningPostingIDs:    [2]string{"p1-" + id, "p2-" + id},
		Audit:                domain.AuditRecord{ID: "audit-" + id, ActorID: "operator", Action: "account.create", ResourceID: id, Outcome: "success", CreatedAt: now},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func transferInput(id, key, from, to string, amount int64, actor string) store.ApplyTransferInput {
	now := time.Unix(2, 0).UTC()
	return store.ApplyTransferInput{
		Transfer: domain.Transfer{
			ID: id, IdempotencyKey: key,
			Intent:   domain.TransferIntent{FromAccountID: from, ToAccountID: to, AmountMinor: amount},
			Currency: "GBP", ActorID: actor, CreatedAt: now,
		},
		PostingIDs: [2]string{"p1-" + id, "p2-" + id},
		Audit:      domain.AuditRecord{ID: "audit-" + id, ActorID: actor, Action: "transfer.create", ResourceID: id, Outcome: "success", CreatedAt: now},
	}
}

func assertBalance(t *testing.T, s *Store, id string, want int64) {
	t.Helper()
	account, err := s.GetAccount(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if account.BalanceMinor != want {
		t.Fatalf("account %s balance=%d, want %d", id, account.BalanceMinor, want)
	}
}
