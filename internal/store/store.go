package store

import (
	"context"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
)

type CreateAccountInput struct {
	Account              domain.Account
	OpeningTransactionID string
	OpeningPostingIDs    [2]string
	Audit                domain.AuditRecord
}

type ApplyTransferInput struct {
	Transfer   domain.Transfer
	PostingIDs [2]string
	Audit      domain.AuditRecord
	Risk       *domain.RiskEvent
}

type TransferResult struct {
	Transfer domain.Transfer `json:"transfer"`
	Replayed bool            `json:"replayed"`
}

type Repository interface {
	CreateAccount(context.Context, CreateAccountInput) error
	GetAccount(context.Context, string) (domain.Account, error)
	ApplyTransfer(context.Context, ApplyTransferInput) (TransferResult, error)
	ListJournal(context.Context, int) ([]domain.Posting, error)
	ListAudit(context.Context, int) ([]domain.AuditRecord, error)
	ListRiskEvents(context.Context, int) ([]domain.RiskEvent, error)
}
