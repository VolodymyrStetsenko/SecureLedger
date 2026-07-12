package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPrincipalValidation(t *testing.T) {
	t.Parallel()
	valid := Principal{ID: "customer-1", Role: RoleCustomer}
	if !valid.Valid() {
		t.Fatal("valid principal was rejected")
	}
	invalid := []Principal{
		{ID: "", Role: RoleCustomer},
		{ID: strings.Repeat("a", 129), Role: RoleCustomer},
		{ID: "customer-1", Role: Role("owner")},
	}
	for _, principal := range invalid {
		if principal.Valid() {
			t.Fatalf("invalid principal was accepted: %+v", principal)
		}
	}
}

func TestValidateBalanced(t *testing.T) {
	t.Parallel()
	postings := []Posting{
		{ID: "p1", TransactionID: "t1", AccountID: "a", AmountMinor: -2500, Currency: "GBP"},
		{ID: "p2", TransactionID: "t1", AccountID: "b", AmountMinor: 2500, Currency: "GBP"},
	}
	if err := ValidateBalanced(postings); err != nil {
		t.Fatalf("expected balanced postings: %v", err)
	}
}

func TestValidateBalancedRejectsUnbalanced(t *testing.T) {
	t.Parallel()
	postings := []Posting{
		{ID: "p1", TransactionID: "t1", AccountID: "a", AmountMinor: -2500, Currency: "GBP"},
		{ID: "p2", TransactionID: "t1", AccountID: "b", AmountMinor: 2499, Currency: "GBP"},
	}
	if err := ValidateBalanced(postings); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestValidateBalancedRejectsZeroPosting(t *testing.T) {
	t.Parallel()
	postings := []Posting{
		{ID: "p1", TransactionID: "t1", AccountID: "a", AmountMinor: 0, Currency: "GBP"},
		{ID: "p2", TransactionID: "t1", AccountID: "b", AmountMinor: 0, Currency: "GBP"},
	}
	if err := ValidateBalanced(postings); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestValidateBalancedRejectsOverflow(t *testing.T) {
	t.Parallel()
	postings := []Posting{
		{ID: "p1", TransactionID: "t1", AccountID: "a", AmountMinor: 1<<63 - 1, Currency: "GBP"},
		{ID: "p2", TransactionID: "t1", AccountID: "b", AmountMinor: 1, Currency: "GBP"},
		{ID: "p3", TransactionID: "t1", AccountID: "c", AmountMinor: -1 << 63, Currency: "GBP"},
	}
	if err := ValidateBalanced(postings); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected overflow rejection, got %v", err)
	}
}

func TestValidateCurrency(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"GBP", true}, {"USD", true}, {"gbp", false}, {"EURO", false}, {"G1P", false},
	} {
		if got := ValidateCurrency(tc.value); (got == nil) != tc.ok {
			t.Fatalf("ValidateCurrency(%q) = %v, want ok=%v", tc.value, got, tc.ok)
		}
	}
}

func TestAuditAndRiskValidation(t *testing.T) {
	t.Parallel()
	now := time.Unix(1, 0).UTC()
	audit := AuditRecord{
		ID: "audit-1", ActorID: "alice", Action: "transfer.create",
		ResourceID: "transfer-1", Outcome: "success", CreatedAt: now,
	}
	if err := ValidateAuditRecord(audit, "alice", "transfer-1"); err != nil {
		t.Fatalf("valid audit rejected: %v", err)
	}
	audit.Outcome = ""
	if err := ValidateAuditRecord(audit, "alice", "transfer-1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid audit error=%v", err)
	}

	event := RiskEvent{
		ID: "risk-1", Type: "high_value_transfer", Severity: "medium",
		TransferID: "transfer-1", Reason: "threshold", CreatedAt: now,
	}
	if err := ValidateRiskEvent(event, "transfer-1"); err != nil {
		t.Fatalf("valid risk event rejected: %v", err)
	}
	event.Severity = "unknown"
	if err := ValidateRiskEvent(event, "transfer-1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid risk event error=%v", err)
	}
}
