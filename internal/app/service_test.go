package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store/memory"
)

func TestCustomerCannotDebitAnotherOwnersAccount(t *testing.T) {
	t.Parallel()
	svc := New(memory.New(), nil, Config{})
	ctx := context.Background()
	operator := domain.Principal{ID: "op", Role: domain.RoleOperator}
	alice, err := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "alice", Currency: "GBP", OpeningBalanceMinor: 1000})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "bob", Currency: "GBP"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Transfer(ctx, domain.Principal{ID: "mallory", Role: domain.RoleCustomer}, TransferCommand{
		IdempotencyKey: "key-12345678", FromAccountID: alice.ID, ToAccountID: bob.ID, AmountMinor: 100,
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestAccountReadAuthorisation(t *testing.T) {
	t.Parallel()
	svc := New(memory.New(), nil, Config{})
	ctx := context.Background()
	operator := domain.Principal{ID: "operator", Role: domain.RoleOperator}
	account, err := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "alice", Currency: "GBP"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetAccount(ctx, domain.Principal{ID: "alice", Role: domain.RoleCustomer}, account.ID); err != nil {
		t.Fatalf("owner read failed: %v", err)
	}
	if _, err := svc.GetAccount(ctx, domain.Principal{ID: "bob", Role: domain.RoleCustomer}, account.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-owner read error=%v", err)
	}
	if _, err := svc.GetAccount(ctx, domain.Principal{ID: "audit", Role: domain.RoleAuditor}, account.ID); err != nil {
		t.Fatalf("auditor read failed: %v", err)
	}
	if _, err := svc.GetAccount(ctx, domain.Principal{}, account.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("invalid principal error=%v", err)
	}
}

func TestTransferValidationAndPolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	operator := domain.Principal{ID: "operator", Role: domain.RoleOperator}

	newService := func() (*Service, domain.Account, domain.Account) {
		svc := New(memory.New(), nil, Config{MaxTransferMinor: 100})
		from, err := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "alice", Currency: "GBP", OpeningBalanceMinor: 100})
		if err != nil {
			t.Fatal(err)
		}
		to, err := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "bob", Currency: "GBP"})
		if err != nil {
			t.Fatal(err)
		}
		return svc, from, to
	}

	t.Run("auditor cannot transfer", func(t *testing.T) {
		svc, from, to := newService()
		_, err := svc.Transfer(ctx, domain.Principal{ID: "audit", Role: domain.RoleAuditor}, TransferCommand{
			IdempotencyKey: "key-12345678", FromAccountID: from.ID, ToAccountID: to.ID, AmountMinor: 1,
		})
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("configured limit", func(t *testing.T) {
		svc, from, to := newService()
		_, err := svc.Transfer(ctx, operator, TransferCommand{
			IdempotencyKey: "key-12345678", FromAccountID: from.ID, ToAccountID: to.ID, AmountMinor: 101,
		})
		if !errors.Is(err, domain.ErrTransferLimit) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		svc, from, to := newService()
		_, err := svc.Transfer(ctx, operator, TransferCommand{
			IdempotencyKey: "key-12345678", FromAccountID: from.ID, ToAccountID: to.ID, AmountMinor: 100,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.Transfer(ctx, operator, TransferCommand{
			IdempotencyKey: "key-87654321", FromAccountID: from.ID, ToAccountID: to.ID, AmountMinor: 1,
		})
		if !errors.Is(err, domain.ErrInsufficientFunds) {
			t.Fatalf("error=%v", err)
		}
	})

	t.Run("description rune limit", func(t *testing.T) {
		svc, from, to := newService()
		_, err := svc.Transfer(ctx, operator, TransferCommand{
			IdempotencyKey: "key-12345678", FromAccountID: from.ID, ToAccountID: to.ID,
			AmountMinor: 1, Description: strings.Repeat("€", 201),
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("error=%v", err)
		}
	})
}

func TestOversightEndpointsRequireInspectionRole(t *testing.T) {
	t.Parallel()
	svc := New(memory.New(), nil, Config{})
	ctx := context.Background()
	customer := domain.Principal{ID: "alice", Role: domain.RoleCustomer}
	if _, err := svc.ListJournal(ctx, customer, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("journal error=%v", err)
	}
	if _, err := svc.ListAudit(ctx, customer, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("audit error=%v", err)
	}
	if _, err := svc.ListRiskEvents(ctx, customer, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("risk error=%v", err)
	}

	auditor := domain.Principal{ID: "audit", Role: domain.RoleAuditor}
	if _, err := svc.ListJournal(ctx, auditor, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListAudit(ctx, auditor, 10); err != nil {
		t.Fatal(err)
	}
}

func TestHighValueTransferCreatesRiskEvent(t *testing.T) {
	t.Parallel()
	repo := memory.New()
	svc := New(repo, nil, Config{RiskThresholdMinor: 500, MaxTransferMinor: 10_000})
	ctx := context.Background()
	operator := domain.Principal{ID: "op", Role: domain.RoleOperator}
	alice, _ := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "alice", Currency: "GBP", OpeningBalanceMinor: 1000})
	bob, _ := svc.CreateAccount(ctx, operator, CreateAccountCommand{OwnerID: "bob", Currency: "GBP"})

	result, err := svc.Transfer(ctx, domain.Principal{ID: "alice", Role: domain.RoleCustomer}, TransferCommand{
		IdempotencyKey: "risk-12345678", FromAccountID: alice.ID, ToAccountID: bob.ID, AmountMinor: 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := svc.ListRiskEvents(ctx, operator, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].TransferID != result.Transfer.ID {
		t.Fatalf("unexpected risk events: %+v", events)
	}
}
