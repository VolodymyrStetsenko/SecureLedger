package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
)

type RiskNotifier interface {
	Submit(domain.RiskEvent) bool
}

type Config struct {
	MaxTransferMinor   int64
	RiskThresholdMinor int64
}

type Service struct {
	repo     store.Repository
	notifier RiskNotifier
	cfg      Config
	clock    func() time.Time
	newID    func(string) string
}

func New(repo store.Repository, notifier RiskNotifier, cfg Config) *Service {
	if cfg.MaxTransferMinor <= 0 {
		cfg.MaxTransferMinor = 100_000_000
	}
	if cfg.RiskThresholdMinor <= 0 {
		cfg.RiskThresholdMinor = 1_000_000
	}
	return &Service{
		repo: repo, notifier: notifier, cfg: cfg,
		clock: time.Now, newID: randomID,
	}
}

type CreateAccountCommand struct {
	OwnerID             string `json:"owner_id"`
	Currency            string `json:"currency"`
	OpeningBalanceMinor int64  `json:"opening_balance_minor"`
}

func (s *Service) CreateAccount(ctx context.Context, actor domain.Principal, cmd CreateAccountCommand) (domain.Account, error) {
	if !actor.Valid() || (actor.Role != domain.RoleOperator && actor.Role != domain.RoleAdmin) {
		return domain.Account{}, domain.ErrForbidden
	}
	cmd.OwnerID = strings.TrimSpace(cmd.OwnerID)
	cmd.Currency = strings.TrimSpace(cmd.Currency)
	if cmd.OwnerID == "" || len(cmd.OwnerID) > 128 || cmd.OpeningBalanceMinor < 0 {
		return domain.Account{}, domain.ErrInvalidInput
	}
	if err := domain.ValidateCurrency(cmd.Currency); err != nil {
		return domain.Account{}, err
	}
	now := s.clock().UTC()
	account := domain.Account{
		ID: s.newID("acct"), OwnerID: cmd.OwnerID, Currency: cmd.Currency,
		BalanceMinor: cmd.OpeningBalanceMinor, CreatedAt: now,
	}
	in := store.CreateAccountInput{
		Account:              account,
		OpeningTransactionID: s.newID("open"),
		OpeningPostingIDs:    [2]string{s.newID("post"), s.newID("post")},
		Audit: domain.AuditRecord{
			ID: s.newID("audit"), ActorID: actor.ID, Action: "account.create",
			ResourceID: account.ID, Outcome: "success", CreatedAt: now,
			Metadata: map[string]any{"owner_id": account.OwnerID, "currency": account.Currency},
		},
	}
	if err := s.repo.CreateAccount(ctx, in); err != nil {
		return domain.Account{}, err
	}
	return account, nil
}

func (s *Service) GetAccount(ctx context.Context, actor domain.Principal, id string) (domain.Account, error) {
	if !actor.Valid() {
		return domain.Account{}, domain.ErrForbidden
	}
	account, err := s.repo.GetAccount(ctx, id)
	if err != nil {
		return domain.Account{}, err
	}
	if actor.Role == domain.RoleCustomer && account.OwnerID != actor.ID {
		return domain.Account{}, domain.ErrForbidden
	}
	return account, nil
}

type TransferCommand struct {
	IdempotencyKey string `json:"-"`
	FromAccountID  string `json:"from_account_id"`
	ToAccountID    string `json:"to_account_id"`
	AmountMinor    int64  `json:"amount_minor"`
	Description    string `json:"description"`
}

func (s *Service) Transfer(ctx context.Context, actor domain.Principal, cmd TransferCommand) (store.TransferResult, error) {
	if !actor.Valid() || actor.Role == domain.RoleAuditor {
		return store.TransferResult{}, domain.ErrForbidden
	}
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	cmd.FromAccountID = strings.TrimSpace(cmd.FromAccountID)
	cmd.ToAccountID = strings.TrimSpace(cmd.ToAccountID)
	cmd.Description = strings.TrimSpace(cmd.Description)
	if len(cmd.IdempotencyKey) < 8 || len(cmd.IdempotencyKey) > 128 ||
		cmd.FromAccountID == "" || cmd.ToAccountID == "" ||
		cmd.AmountMinor <= 0 || utf8.RuneCountInString(cmd.Description) > 200 {
		return store.TransferResult{}, domain.ErrInvalidInput
	}
	if cmd.AmountMinor > s.cfg.MaxTransferMinor {
		return store.TransferResult{}, domain.ErrTransferLimit
	}

	from, err := s.repo.GetAccount(ctx, cmd.FromAccountID)
	if err != nil {
		return store.TransferResult{}, err
	}
	if actor.Role == domain.RoleCustomer && from.OwnerID != actor.ID {
		return store.TransferResult{}, domain.ErrForbidden
	}
	to, err := s.repo.GetAccount(ctx, cmd.ToAccountID)
	if err != nil {
		return store.TransferResult{}, err
	}
	if from.Currency != to.Currency {
		return store.TransferResult{}, domain.ErrCurrencyMismatch
	}

	now := s.clock().UTC()
	transfer := domain.Transfer{
		ID: s.newID("txn"), IdempotencyKey: cmd.IdempotencyKey,
		Intent: domain.TransferIntent{
			FromAccountID: cmd.FromAccountID, ToAccountID: cmd.ToAccountID,
			AmountMinor: cmd.AmountMinor, Description: cmd.Description,
		},
		Currency: from.Currency, ActorID: actor.ID, CreatedAt: now,
	}
	var riskEvent *domain.RiskEvent
	if cmd.AmountMinor >= s.cfg.RiskThresholdMinor {
		riskEvent = &domain.RiskEvent{
			ID: s.newID("risk"), Type: "high_value_transfer", Severity: "medium",
			TransferID: transfer.ID, Reason: fmt.Sprintf("amount_minor >= %d", s.cfg.RiskThresholdMinor),
			CreatedAt: now,
		}
	}
	result, err := s.repo.ApplyTransfer(ctx, store.ApplyTransferInput{
		Transfer:   transfer,
		PostingIDs: [2]string{s.newID("post"), s.newID("post")},
		Audit: domain.AuditRecord{
			ID: s.newID("audit"), ActorID: actor.ID, Action: "transfer.create",
			ResourceID: transfer.ID, Outcome: "success", CreatedAt: now,
			Metadata: map[string]any{
				"from_account_id": cmd.FromAccountID,
				"to_account_id":   cmd.ToAccountID,
				"amount_minor":    cmd.AmountMinor,
				"currency":        from.Currency,
			},
		},
		Risk: riskEvent,
	})
	if err != nil {
		return store.TransferResult{}, err
	}
	if riskEvent != nil && !result.Replayed && s.notifier != nil {
		s.notifier.Submit(*riskEvent)
	}
	return result, nil
}

func (s *Service) ListJournal(ctx context.Context, actor domain.Principal, limit int) ([]domain.Posting, error) {
	if !canInspect(actor) {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListJournal(ctx, limit)
}

func (s *Service) ListAudit(ctx context.Context, actor domain.Principal, limit int) ([]domain.AuditRecord, error) {
	if !canInspect(actor) {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListAudit(ctx, limit)
}

func (s *Service) ListRiskEvents(ctx context.Context, actor domain.Principal, limit int) ([]domain.RiskEvent, error) {
	if !canInspect(actor) {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListRiskEvents(ctx, limit)
}

func canInspect(actor domain.Principal) bool {
	return actor.Valid() && (actor.Role == domain.RoleOperator || actor.Role == domain.RoleAdmin || actor.Role == domain.RoleAuditor)
}

func randomID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
