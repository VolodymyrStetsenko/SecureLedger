package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
)

func TestTransferFingerprintIsDeterministic(t *testing.T) {
	t.Parallel()
	transfer := domain.Transfer{
		ActorID: "alice",
		Intent: domain.TransferIntent{
			FromAccountID: "a", ToAccountID: "b", AmountMinor: 100, Description: "invoice-42",
		},
	}
	first, err := transferFingerprint(transfer)
	if err != nil {
		t.Fatal(err)
	}
	second, err := transferFingerprint(transfer)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("equal transfer intents produced different fingerprints")
	}
	transfer.Intent.AmountMinor++
	changed, err := transferFingerprint(transfer)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == string(changed) {
		t.Fatal("different transfer intents produced the same fingerprint")
	}
}

func TestValidationRejectsMalformedRepositoryInputs(t *testing.T) {
	t.Parallel()
	if err := validateCreateAccount(store.CreateAccountInput{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("create validation error=%v", err)
	}
	if err := validateTransfer(store.ApplyTransferInput{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("transfer validation error=%v", err)
	}
}

func TestRetryClassification(t *testing.T) {
	t.Parallel()
	if !isRetryable(&pgconn.PgError{Code: "40001"}) {
		t.Fatal("serialization failure should be retryable")
	}
	if !isRetryable(&pgconn.PgError{Code: "40P01"}) {
		t.Fatal("deadlock should be retryable")
	}
	if isRetryable(&pgconn.PgError{Code: "23514"}) {
		t.Fatal("constraint violation should not be retryable")
	}
	if !isIdempotencyRace(&pgconn.PgError{Code: "23505", ConstraintName: "journal_transactions_actor_id_idempotency_key_key"}) {
		t.Fatal("idempotency uniqueness race was not recognised")
	}
}

func TestRepositoryListLimits(t *testing.T) {
	t.Parallel()
	if normaliseLimit(0) != 100 || normaliseLimit(1001) != 1000 || normaliseLimit(7) != 7 {
		t.Fatal("journal list limit normalisation is incorrect")
	}
	if normaliseOutboxLimit(0) != 32 || normaliseOutboxLimit(101) != 100 || normaliseOutboxLimit(7) != 7 {
		t.Fatal("outbox limit normalisation is incorrect")
	}
}

func TestValidateCreateAccountAcceptsBalancedOpening(t *testing.T) {
	t.Parallel()
	now := time.Unix(1, 0).UTC()
	err := validateCreateAccount(store.CreateAccountInput{
		Account:              domain.Account{ID: "a", OwnerID: "alice", Currency: "GBP", BalanceMinor: 100, CreatedAt: now},
		OpeningTransactionID: "open-a",
		OpeningPostingIDs:    [2]string{"p1", "p2"},
		Audit:                domain.AuditRecord{ID: "audit-a", ActorID: "operator", Action: "account.create", ResourceID: "a", Outcome: "success", CreatedAt: now},
	})
	if err != nil {
		t.Fatalf("valid opening rejected: %v", err)
	}
}
