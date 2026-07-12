package domain

import (
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleCustomer Role = "customer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
	RoleAuditor  Role = "auditor"
)

type Principal struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
}

func (p Principal) Valid() bool {
	if strings.TrimSpace(p.ID) == "" {
		return false
	}
	switch p.Role {
	case RoleCustomer, RoleOperator, RoleAdmin, RoleAuditor:
		return true
	default:
		return false
	}
}

type Account struct {
	ID           string    `json:"id"`
	OwnerID      string    `json:"owner_id"`
	Currency     string    `json:"currency"`
	BalanceMinor int64     `json:"balance_minor"`
	System       bool      `json:"system"`
	CreatedAt    time.Time `json:"created_at"`
}

func ValidateCurrency(v string) error {
	if len(v) != 3 || strings.ToUpper(v) != v {
		return fmt.Errorf("%w: currency must be three uppercase letters", ErrInvalidInput)
	}
	for _, r := range v {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%w: currency must contain only A-Z", ErrInvalidInput)
		}
	}
	return nil
}

type TransferIntent struct {
	FromAccountID string `json:"from_account_id"`
	ToAccountID   string `json:"to_account_id"`
	AmountMinor   int64  `json:"amount_minor"`
	Description   string `json:"description"`
}

func (i TransferIntent) Equal(other TransferIntent) bool {
	return i.FromAccountID == other.FromAccountID &&
		i.ToAccountID == other.ToAccountID &&
		i.AmountMinor == other.AmountMinor &&
		i.Description == other.Description
}

type Transfer struct {
	ID             string         `json:"id"`
	IdempotencyKey string         `json:"idempotency_key"`
	Intent         TransferIntent `json:"intent"`
	Currency       string         `json:"currency"`
	ActorID        string         `json:"actor_id"`
	CreatedAt      time.Time      `json:"created_at"`
}

type Posting struct {
	ID            string    `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	Sequence      int64     `json:"sequence"`
	CreatedAt     time.Time `json:"created_at"`
}

func ValidateBalanced(postings []Posting) error {
	if len(postings) < 2 {
		return fmt.Errorf("%w: transaction requires at least two postings", ErrInvalidInput)
	}
	currency := postings[0].Currency
	if err := ValidateCurrency(currency); err != nil {
		return err
	}
	var sum int64
	for _, p := range postings {
		if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.TransactionID) == "" || strings.TrimSpace(p.AccountID) == "" {
			return fmt.Errorf("%w: posting identifiers are required", ErrInvalidInput)
		}
		if p.AmountMinor == 0 {
			return fmt.Errorf("%w: posting amount cannot be zero", ErrInvalidInput)
		}
		if p.Currency != currency {
			return ErrCurrencyMismatch
		}
		if (p.AmountMinor > 0 && sum > (1<<63-1)-p.AmountMinor) ||
			(p.AmountMinor < 0 && sum < (-1<<63)-p.AmountMinor) {
			return fmt.Errorf("%w: posting sum overflow", ErrInvalidInput)
		}
		sum += p.AmountMinor
	}
	if sum != 0 {
		return fmt.Errorf("%w: postings are not balanced", ErrInvalidInput)
	}
	return nil
}

type AuditRecord struct {
	ID         string         `json:"id"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	ResourceID string         `json:"resource_id"`
	Outcome    string         `json:"outcome"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type RiskEvent struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	TransferID string    `json:"transfer_id"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}
