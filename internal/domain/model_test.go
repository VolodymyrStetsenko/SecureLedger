package domain

import (
	"errors"
	"testing"
)

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
