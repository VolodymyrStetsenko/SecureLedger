package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
)

type idempotencyRecord struct {
	transfer domain.Transfer
}

type idempotencyScope struct {
	actorID string
	key     string
}

type Store struct {
	mu           sync.RWMutex
	accounts     map[string]domain.Account
	idempotency  map[idempotencyScope]idempotencyRecord
	transactions map[string]struct{}
	postings     map[string]struct{}
	audits       map[string]struct{}
	risks        map[string]struct{}
	journal      []domain.Posting
	audit        []domain.AuditRecord
	risk         []domain.RiskEvent
	sequence     int64
}

func New() *Store {
	return &Store{
		accounts:     make(map[string]domain.Account),
		idempotency:  make(map[idempotencyScope]idempotencyRecord),
		transactions: make(map[string]struct{}),
		postings:     make(map[string]struct{}),
		audits:       make(map[string]struct{}),
		risks:        make(map[string]struct{}),
	}
}

func systemAccountID(currency string) string {
	return "system:equity:" + currency
}

func (s *Store) CreateAccount(ctx context.Context, in store.CreateAccountInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if in.Account.ID == "" || in.Account.OwnerID == "" || in.Account.CreatedAt.IsZero() ||
		in.Account.System || strings.HasPrefix(in.Account.ID, "system:") {
		return fmt.Errorf("%w: account id, owner and creation time are required", domain.ErrInvalidInput)
	}
	if _, exists := s.accounts[in.Account.ID]; exists {
		return fmt.Errorf("%w: account already exists", domain.ErrInvalidInput)
	}
	if err := domain.ValidateCurrency(in.Account.Currency); err != nil {
		return err
	}
	if in.Account.BalanceMinor < 0 {
		return fmt.Errorf("%w: opening balance cannot be negative", domain.ErrInvalidInput)
	}
	if in.Audit.ID == "" || in.Audit.ActorID == "" || in.Audit.Action == "" || in.Audit.ResourceID != in.Account.ID || in.Audit.CreatedAt.IsZero() {
		return fmt.Errorf("%w: valid account audit record is required", domain.ErrInvalidInput)
	}
	if _, exists := s.audits[in.Audit.ID]; exists {
		return fmt.Errorf("%w: duplicate audit id", domain.ErrInvalidInput)
	}

	if in.Account.BalanceMinor > 0 {
		if in.OpeningTransactionID == "" || in.OpeningPostingIDs[0] == "" || in.OpeningPostingIDs[1] == "" || in.OpeningPostingIDs[0] == in.OpeningPostingIDs[1] {
			return fmt.Errorf("%w: unique opening transaction and posting ids are required", domain.ErrInvalidInput)
		}
		if _, exists := s.transactions[in.OpeningTransactionID]; exists {
			return fmt.Errorf("%w: duplicate opening transaction id", domain.ErrInvalidInput)
		}
		for _, id := range in.OpeningPostingIDs {
			if _, exists := s.postings[id]; exists {
				return fmt.Errorf("%w: duplicate posting id", domain.ErrInvalidInput)
			}
		}
		systemID := systemAccountID(in.Account.Currency)
		systemAccount, ok := s.accounts[systemID]
		if !ok {
			systemAccount = domain.Account{
				ID: systemID, OwnerID: "system", Currency: in.Account.Currency,
				System: true, CreatedAt: in.Account.CreatedAt,
			}
		} else if !systemAccount.System || systemAccount.OwnerID != "system" || systemAccount.Currency != in.Account.Currency {
			return fmt.Errorf("%w: reserved system account is invalid", domain.ErrInvalidInput)
		}
		if systemAccount.BalanceMinor < (-1<<63)+in.Account.BalanceMinor {
			return fmt.Errorf("%w: system account balance underflow", domain.ErrInvalidInput)
		}
		postings := []domain.Posting{
			{
				ID: in.OpeningPostingIDs[0], TransactionID: in.OpeningTransactionID,
				AccountID: in.Account.ID, AmountMinor: in.Account.BalanceMinor,
				Currency: in.Account.Currency, Sequence: s.sequence + 1, CreatedAt: in.Account.CreatedAt,
			},
			{
				ID: in.OpeningPostingIDs[1], TransactionID: in.OpeningTransactionID,
				AccountID: systemID, AmountMinor: -in.Account.BalanceMinor,
				Currency: in.Account.Currency, Sequence: s.sequence + 2, CreatedAt: in.Account.CreatedAt,
			},
		}
		if err := domain.ValidateBalanced(postings); err != nil {
			return err
		}
		systemAccount.BalanceMinor -= in.Account.BalanceMinor
		s.accounts[systemID] = systemAccount
		s.sequence += 2
		s.transactions[in.OpeningTransactionID] = struct{}{}
		for _, id := range in.OpeningPostingIDs {
			s.postings[id] = struct{}{}
		}
		s.journal = append(s.journal, postings...)
	}
	s.accounts[in.Account.ID] = in.Account
	s.audits[in.Audit.ID] = struct{}{}
	s.audit = append(s.audit, cloneAudit(in.Audit))
	return nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (domain.Account, error) {
	if err := ctx.Err(); err != nil {
		return domain.Account{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.accounts[id]
	if !ok || account.System {
		return domain.Account{}, domain.ErrNotFound
	}
	return account, nil
}

func (s *Store) ApplyTransfer(ctx context.Context, in store.ApplyTransferInput) (store.TransferResult, error) {
	if err := ctx.Err(); err != nil {
		return store.TransferResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	scope := idempotencyScope{actorID: in.Transfer.ActorID, key: in.Transfer.IdempotencyKey}
	if existing, ok := s.idempotency[scope]; ok {
		if !existing.transfer.Intent.Equal(in.Transfer.Intent) {
			return store.TransferResult{}, domain.ErrIdempotencyConflict
		}
		return store.TransferResult{Transfer: existing.transfer, Replayed: true}, nil
	}
	if in.Transfer.ID == "" || in.Transfer.IdempotencyKey == "" || in.Transfer.ActorID == "" || in.Transfer.CreatedAt.IsZero() {
		return store.TransferResult{}, fmt.Errorf("%w: transfer identity fields are required", domain.ErrInvalidInput)
	}
	if _, exists := s.transactions[in.Transfer.ID]; exists {
		return store.TransferResult{}, fmt.Errorf("%w: duplicate transaction id", domain.ErrInvalidInput)
	}
	if in.PostingIDs[0] == "" || in.PostingIDs[1] == "" || in.PostingIDs[0] == in.PostingIDs[1] {
		return store.TransferResult{}, fmt.Errorf("%w: two unique posting ids are required", domain.ErrInvalidInput)
	}
	for _, id := range in.PostingIDs {
		if _, exists := s.postings[id]; exists {
			return store.TransferResult{}, fmt.Errorf("%w: duplicate posting id", domain.ErrInvalidInput)
		}
	}
	if in.Audit.ID == "" || in.Audit.ActorID != in.Transfer.ActorID || in.Audit.ResourceID != in.Transfer.ID || in.Audit.CreatedAt.IsZero() {
		return store.TransferResult{}, fmt.Errorf("%w: valid transfer audit record is required", domain.ErrInvalidInput)
	}
	if _, exists := s.audits[in.Audit.ID]; exists {
		return store.TransferResult{}, fmt.Errorf("%w: duplicate audit id", domain.ErrInvalidInput)
	}
	if in.Risk != nil {
		if in.Risk.ID == "" || in.Risk.TransferID != in.Transfer.ID || in.Risk.CreatedAt.IsZero() {
			return store.TransferResult{}, fmt.Errorf("%w: valid risk event is required", domain.ErrInvalidInput)
		}
		if _, exists := s.risks[in.Risk.ID]; exists {
			return store.TransferResult{}, fmt.Errorf("%w: duplicate risk event id", domain.ErrInvalidInput)
		}
	}

	from, ok := s.accounts[in.Transfer.Intent.FromAccountID]
	if !ok || from.System {
		return store.TransferResult{}, domain.ErrNotFound
	}
	to, ok := s.accounts[in.Transfer.Intent.ToAccountID]
	if !ok || to.System {
		return store.TransferResult{}, domain.ErrNotFound
	}
	if from.ID == to.ID {
		return store.TransferResult{}, fmt.Errorf("%w: source and destination must differ", domain.ErrInvalidInput)
	}
	if from.Currency != to.Currency || from.Currency != in.Transfer.Currency {
		return store.TransferResult{}, domain.ErrCurrencyMismatch
	}
	if in.Transfer.Intent.AmountMinor <= 0 {
		return store.TransferResult{}, fmt.Errorf("%w: amount must be positive", domain.ErrInvalidInput)
	}
	if from.BalanceMinor < in.Transfer.Intent.AmountMinor {
		return store.TransferResult{}, domain.ErrInsufficientFunds
	}
	if to.BalanceMinor > (1<<63-1)-in.Transfer.Intent.AmountMinor {
		return store.TransferResult{}, fmt.Errorf("%w: destination balance overflow", domain.ErrInvalidInput)
	}

	postings := []domain.Posting{
		{
			ID: in.PostingIDs[0], TransactionID: in.Transfer.ID,
			AccountID: from.ID, AmountMinor: -in.Transfer.Intent.AmountMinor,
			Currency: from.Currency, Sequence: s.sequence + 1, CreatedAt: in.Transfer.CreatedAt,
		},
		{
			ID: in.PostingIDs[1], TransactionID: in.Transfer.ID,
			AccountID: to.ID, AmountMinor: in.Transfer.Intent.AmountMinor,
			Currency: to.Currency, Sequence: s.sequence + 2, CreatedAt: in.Transfer.CreatedAt,
		},
	}
	if err := domain.ValidateBalanced(postings); err != nil {
		return store.TransferResult{}, err
	}

	from.BalanceMinor -= in.Transfer.Intent.AmountMinor
	to.BalanceMinor += in.Transfer.Intent.AmountMinor
	s.accounts[from.ID] = from
	s.accounts[to.ID] = to
	s.sequence += 2
	s.transactions[in.Transfer.ID] = struct{}{}
	for _, id := range in.PostingIDs {
		s.postings[id] = struct{}{}
	}
	s.audits[in.Audit.ID] = struct{}{}
	s.journal = append(s.journal, postings...)
	s.audit = append(s.audit, cloneAudit(in.Audit))
	if in.Risk != nil {
		s.risk = append(s.risk, *in.Risk)
		s.risks[in.Risk.ID] = struct{}{}
	}
	s.idempotency[scope] = idempotencyRecord{transfer: in.Transfer}

	return store.TransferResult{Transfer: in.Transfer}, nil
}

func (s *Store) ListJournal(ctx context.Context, limit int) ([]domain.Posting, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return tailPostings(s.journal, normaliseLimit(limit)), nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]domain.AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	start := max(0, len(s.audit)-normaliseLimit(limit))
	out := make([]domain.AuditRecord, 0, len(s.audit)-start)
	for _, record := range s.audit[start:] {
		out = append(out, cloneAudit(record))
	}
	return out, nil
}

func (s *Store) ListRiskEvents(ctx context.Context, limit int) ([]domain.RiskEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	start := max(0, len(s.risk)-normaliseLimit(limit))
	out := append([]domain.RiskEvent(nil), s.risk[start:]...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func normaliseLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func tailPostings(in []domain.Posting, limit int) []domain.Posting {
	start := max(0, len(in)-limit)
	return append([]domain.Posting(nil), in[start:]...)
}

func cloneAudit(in domain.AuditRecord) domain.AuditRecord {
	out := in
	if in.Metadata != nil {
		out.Metadata = make(map[string]any, len(in.Metadata))
		for k, v := range in.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}
