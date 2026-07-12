package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/VolodymyrStetsenko/secureledger/internal/domain"
	"github.com/VolodymyrStetsenko/secureledger/internal/store"
)

const (
	defaultMaxRetries     = 4
	requiredSchemaVersion = 1
)

type Store struct {
	pool       *pgxpool.Pool
	maxRetries int
}

var _ store.Repository = (*Store)(nil)

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	if config.ConnConfig.RuntimeParams["application_name"] == "" {
		config.ConnConfig.RuntimeParams["application_name"] = "secureledger"
	}
	if config.ConnConfig.ConnectTimeout == 0 {
		config.ConnConfig.ConnectTimeout = 5 * time.Second
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	store := New(pool)
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("validate PostgreSQL dependency: %w", err)
	}
	return store, nil
}

func New(pool *pgxpool.Pool) *Store {
	if pool == nil {
		panic("postgres store requires a pool")
	}
	return &Store{pool: pool, maxRetries: defaultMaxRetries}
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return err
	}
	var applied bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE version = $1
		)`, requiredSchemaVersion).Scan(&applied)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if !applied {
		return fmt.Errorf("required schema migration %d is not applied", requiredSchemaVersion)
	}
	return nil
}

func (s *Store) CreateAccount(ctx context.Context, in store.CreateAccountInput) error {
	if err := validateCreateAccount(in); err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt < s.maxRetries; attempt++ {
		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return fmt.Errorf("begin account transaction: %w", err)
		}
		err = createAccountTx(ctx, tx, in)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == s.maxRetries-1 {
			return mapDatabaseError(err)
		}
		if err := waitForRetry(ctx, attempt); err != nil {
			return err
		}
	}
	return fmt.Errorf("account transaction retry exhausted: %w", lastErr)
}

func createAccountTx(ctx context.Context, tx pgx.Tx, in store.CreateAccountInput) error {
	now := in.Account.CreatedAt.UTC()
	_, err := tx.Exec(ctx, `
		INSERT INTO accounts (id, owner_id, currency, balance_minor, system, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, false, 0, $5, $5)`,
		in.Account.ID, in.Account.OwnerID, in.Account.Currency, in.Account.BalanceMinor, now,
	)
	if err != nil {
		return err
	}

	if in.Account.BalanceMinor > 0 {
		systemID := systemAccountID(in.Account.Currency)
		_, err = tx.Exec(ctx, `
			INSERT INTO accounts (id, owner_id, currency, balance_minor, system, version, created_at, updated_at)
			VALUES ($1, 'system', $2, 0, true, 0, $3, $3)
			ON CONFLICT (id) DO NOTHING`, systemID, in.Account.Currency, now)
		if err != nil {
			return err
		}

		var systemBalance int64
		var systemOwner, systemCurrency string
		var system bool
		err = tx.QueryRow(ctx, `
			SELECT owner_id, currency, balance_minor, system
			FROM accounts
			WHERE id = $1
			FOR UPDATE`, systemID).Scan(&systemOwner, &systemCurrency, &systemBalance, &system)
		if err != nil {
			return err
		}
		if !system || systemOwner != "system" || systemCurrency != in.Account.Currency {
			return fmt.Errorf("%w: reserved system account is invalid", domain.ErrInvalidInput)
		}
		if systemBalance < (-1<<63)+in.Account.BalanceMinor {
			return fmt.Errorf("%w: system account balance underflow", domain.ErrInvalidInput)
		}
		_, err = tx.Exec(ctx, `
			UPDATE accounts
			SET balance_minor = $2, version = version + 1, updated_at = $3
			WHERE id = $1`, systemID, systemBalance-in.Account.BalanceMinor, now)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO journal_transactions (id, kind, idempotency_key, actor_id, currency, description, created_at)
			VALUES ($1, 'opening', NULL, $2, $3, 'Opening balance', $4)`,
			in.OpeningTransactionID, in.Audit.ActorID, in.Account.Currency, now)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO postings (id, transaction_id, account_id, amount_minor, created_at)
			VALUES
				($1, $3, $4, $6, $8),
				($2, $3, $5, $7, $8)`,
			in.OpeningPostingIDs[0], in.OpeningPostingIDs[1], in.OpeningTransactionID,
			in.Account.ID, systemID, in.Account.BalanceMinor, -in.Account.BalanceMinor, now)
		if err != nil {
			return err
		}
	}

	return insertAudit(ctx, tx, in.Audit)
}

func (s *Store) GetAccount(ctx context.Context, id string) (domain.Account, error) {
	var account domain.Account
	err := s.pool.QueryRow(ctx, `
		SELECT id, owner_id, currency, balance_minor, system, created_at
		FROM accounts
		WHERE id = $1 AND system = false`, id).Scan(
		&account.ID, &account.OwnerID, &account.Currency, &account.BalanceMinor, &account.System, &account.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("get account: %w", err)
	}
	return account, nil
}

func (s *Store) ApplyTransfer(ctx context.Context, in store.ApplyTransferInput) (store.TransferResult, error) {
	if err := validateTransfer(in); err != nil {
		return store.TransferResult{}, err
	}

	var lastErr error
	for attempt := 0; attempt < s.maxRetries; attempt++ {
		tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return store.TransferResult{}, fmt.Errorf("begin transfer transaction: %w", err)
		}
		result, err := applyTransferTx(ctx, tx, in)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err == nil {
			return result, nil
		}
		lastErr = err
		if (!isRetryable(err) && !isIdempotencyRace(err)) || attempt == s.maxRetries-1 {
			return store.TransferResult{}, mapDatabaseError(err)
		}
		if err := waitForRetry(ctx, attempt); err != nil {
			return store.TransferResult{}, err
		}
	}
	return store.TransferResult{}, fmt.Errorf("transfer transaction retry exhausted: %w", lastErr)
}

func applyTransferTx(ctx context.Context, tx pgx.Tx, in store.ApplyTransferInput) (store.TransferResult, error) {
	existing, found, err := findIdempotentTransfer(ctx, tx, in.Transfer.ActorID, in.Transfer.IdempotencyKey)
	if err != nil {
		return store.TransferResult{}, err
	}
	if found {
		if !existing.Intent.Equal(in.Transfer.Intent) {
			return store.TransferResult{}, domain.ErrIdempotencyConflict
		}
		return store.TransferResult{Transfer: existing, Replayed: true}, nil
	}

	if in.Transfer.Intent.FromAccountID == in.Transfer.Intent.ToAccountID {
		return store.TransferResult{}, fmt.Errorf("%w: source and destination must differ", domain.ErrInvalidInput)
	}
	accounts, err := lockAccounts(ctx, tx, in.Transfer.Intent.FromAccountID, in.Transfer.Intent.ToAccountID)
	if err != nil {
		return store.TransferResult{}, err
	}
	from, fromOK := accounts[in.Transfer.Intent.FromAccountID]
	to, toOK := accounts[in.Transfer.Intent.ToAccountID]
	if !fromOK || !toOK {
		return store.TransferResult{}, domain.ErrNotFound
	}
	if from.Currency != to.Currency || from.Currency != in.Transfer.Currency {
		return store.TransferResult{}, domain.ErrCurrencyMismatch
	}
	if from.BalanceMinor < in.Transfer.Intent.AmountMinor {
		return store.TransferResult{}, domain.ErrInsufficientFunds
	}
	if to.BalanceMinor > (1<<63-1)-in.Transfer.Intent.AmountMinor {
		return store.TransferResult{}, fmt.Errorf("%w: destination balance overflow", domain.ErrInvalidInput)
	}

	now := in.Transfer.CreatedAt.UTC()
	_, err = tx.Exec(ctx, `
		INSERT INTO journal_transactions (id, kind, idempotency_key, actor_id, currency, description, created_at)
		VALUES ($1, 'transfer', $2, $3, $4, $5, $6)`,
		in.Transfer.ID, in.Transfer.IdempotencyKey, in.Transfer.ActorID, from.Currency,
		in.Transfer.Intent.Description, now)
	if err != nil {
		return store.TransferResult{}, err
	}

	fingerprint, err := transferFingerprint(in.Transfer)
	if err != nil {
		return store.TransferResult{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO transfer_intents
			(transaction_id, from_account_id, to_account_id, amount_minor, request_fingerprint)
		VALUES ($1, $2, $3, $4, $5)`,
		in.Transfer.ID, from.ID, to.ID, in.Transfer.Intent.AmountMinor, fingerprint)
	if err != nil {
		return store.TransferResult{}, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE accounts
		SET balance_minor = $2, version = version + 1, updated_at = $3
		WHERE id = $1`, from.ID, from.BalanceMinor-in.Transfer.Intent.AmountMinor, now)
	if err != nil {
		return store.TransferResult{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE accounts
		SET balance_minor = $2, version = version + 1, updated_at = $3
		WHERE id = $1`, to.ID, to.BalanceMinor+in.Transfer.Intent.AmountMinor, now)
	if err != nil {
		return store.TransferResult{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO postings (id, transaction_id, account_id, amount_minor, created_at)
		VALUES
			($1, $3, $4, $6, $8),
			($2, $3, $5, $7, $8)`,
		in.PostingIDs[0], in.PostingIDs[1], in.Transfer.ID, from.ID, to.ID,
		-in.Transfer.Intent.AmountMinor, in.Transfer.Intent.AmountMinor, now)
	if err != nil {
		return store.TransferResult{}, err
	}
	if err := insertAudit(ctx, tx, in.Audit); err != nil {
		return store.TransferResult{}, err
	}
	if in.Risk != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO risk_events
				(id, event_type, severity, transaction_id, reason, status, attempts, created_at, available_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', 0, $6, $6)`,
			in.Risk.ID, in.Risk.Type, in.Risk.Severity, in.Transfer.ID, in.Risk.Reason, in.Risk.CreatedAt.UTC())
		if err != nil {
			return store.TransferResult{}, err
		}
	}

	return store.TransferResult{Transfer: in.Transfer}, nil
}

func findIdempotentTransfer(ctx context.Context, tx pgx.Tx, actorID, key string) (domain.Transfer, bool, error) {
	var transfer domain.Transfer
	var storedFingerprint []byte
	transfer.IdempotencyKey = key
	err := tx.QueryRow(ctx, `
		SELECT jt.id, ti.from_account_id, ti.to_account_id, ti.amount_minor,
		       jt.description, jt.currency, jt.actor_id, jt.created_at, ti.request_fingerprint
		FROM journal_transactions jt
		JOIN transfer_intents ti ON ti.transaction_id = jt.id
		WHERE jt.kind = 'transfer' AND jt.actor_id = $1 AND jt.idempotency_key = $2`,
		actorID, key).Scan(
		&transfer.ID,
		&transfer.Intent.FromAccountID,
		&transfer.Intent.ToAccountID,
		&transfer.Intent.AmountMinor,
		&transfer.Intent.Description,
		&transfer.Currency,
		&transfer.ActorID,
		&transfer.CreatedAt,
		&storedFingerprint,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transfer{}, false, nil
	}
	if err != nil {
		return domain.Transfer{}, false, err
	}
	expectedFingerprint, err := transferFingerprint(transfer)
	if err != nil {
		return domain.Transfer{}, false, err
	}
	if !bytes.Equal(storedFingerprint, expectedFingerprint) {
		return domain.Transfer{}, false, fmt.Errorf("stored transfer fingerprint does not match its immutable intent")
	}
	return transfer, true, nil
}

func lockAccounts(ctx context.Context, tx pgx.Tx, firstID, secondID string) (map[string]domain.Account, error) {
	ids := []string{firstID, secondID}
	sort.Strings(ids)
	rows, err := tx.Query(ctx, `
		SELECT id, owner_id, currency, balance_minor, system, created_at
		FROM accounts
		WHERE id = ANY($1) AND system = false
		ORDER BY id
		FOR UPDATE`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make(map[string]domain.Account, 2)
	for rows.Next() {
		var account domain.Account
		if err := rows.Scan(
			&account.ID, &account.OwnerID, &account.Currency,
			&account.BalanceMinor, &account.System, &account.CreatedAt,
		); err != nil {
			return nil, err
		}
		accounts[account.ID] = account
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (s *Store) ListJournal(ctx context.Context, limit int) ([]domain.Posting, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, transaction_id, account_id, amount_minor, currency, sequence_no, created_at
		FROM (
			SELECT p.id, p.transaction_id, p.account_id, p.amount_minor,
			       jt.currency, p.sequence_no, p.created_at
			FROM postings p
			JOIN journal_transactions jt ON jt.id = p.transaction_id
			ORDER BY p.sequence_no DESC
			LIMIT $1
		) recent
		ORDER BY sequence_no`, normaliseLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list journal: %w", err)
	}
	defer rows.Close()
	var out []domain.Posting
	for rows.Next() {
		var posting domain.Posting
		if err := rows.Scan(
			&posting.ID, &posting.TransactionID, &posting.AccountID,
			&posting.AmountMinor, &posting.Currency, &posting.Sequence, &posting.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, posting)
	}
	return out, rows.Err()
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]domain.AuditRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, actor_id, action, resource_id, outcome, metadata, created_at
		FROM (
			SELECT sequence_no, id, actor_id, action, resource_id, outcome, metadata, created_at
			FROM audit_records
			ORDER BY sequence_no DESC
			LIMIT $1
		) recent
		ORDER BY sequence_no`, normaliseLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	var out []domain.AuditRecord
	for rows.Next() {
		var record domain.AuditRecord
		var metadata []byte
		if err := rows.Scan(
			&record.ID, &record.ActorID, &record.Action, &record.ResourceID,
			&record.Outcome, &metadata, &record.CreatedAt,
		); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &record.Metadata); err != nil {
				return nil, fmt.Errorf("decode audit metadata: %w", err)
			}
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) ListRiskEvents(ctx context.Context, limit int) ([]domain.RiskEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_type, severity, transaction_id, reason, created_at
		FROM (
			SELECT id, event_type, severity, transaction_id, reason, created_at
			FROM risk_events
			ORDER BY created_at DESC, id DESC
			LIMIT $1
		) recent
		ORDER BY created_at, id`, normaliseLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("list risk events: %w", err)
	}
	defer rows.Close()
	var out []domain.RiskEvent
	for rows.Next() {
		var event domain.RiskEvent
		if err := rows.Scan(
			&event.ID, &event.Type, &event.Severity, &event.TransferID, &event.Reason, &event.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func insertAudit(ctx context.Context, tx pgx.Tx, record domain.AuditRecord) error {
	metadataValue := record.Metadata
	if metadataValue == nil {
		metadataValue = map[string]any{}
	}
	metadata, err := json.Marshal(metadataValue)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_records
			(id, actor_id, action, resource_id, outcome, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		record.ID, record.ActorID, record.Action, record.ResourceID,
		record.Outcome, metadata, record.CreatedAt.UTC())
	return err
}

func validateCreateAccount(in store.CreateAccountInput) error {
	account := in.Account
	if account.ID == "" || len(account.ID) > 128 ||
		account.OwnerID == "" || len(account.OwnerID) > 128 || account.CreatedAt.IsZero() ||
		account.System || strings.HasPrefix(account.ID, "system:") || account.BalanceMinor < 0 {
		return fmt.Errorf("%w: invalid account", domain.ErrInvalidInput)
	}
	if err := domain.ValidateCurrency(account.Currency); err != nil {
		return err
	}
	if err := domain.ValidateAuditRecord(in.Audit, "", account.ID); err != nil {
		return err
	}
	if account.BalanceMinor > 0 {
		if in.OpeningTransactionID == "" || in.OpeningPostingIDs[0] == "" ||
			in.OpeningPostingIDs[1] == "" || in.OpeningPostingIDs[0] == in.OpeningPostingIDs[1] {
			return fmt.Errorf("%w: unique opening transaction and posting ids are required", domain.ErrInvalidInput)
		}
		postings := []domain.Posting{
			{ID: in.OpeningPostingIDs[0], TransactionID: in.OpeningTransactionID, AccountID: account.ID, AmountMinor: account.BalanceMinor, Currency: account.Currency},
			{ID: in.OpeningPostingIDs[1], TransactionID: in.OpeningTransactionID, AccountID: systemAccountID(account.Currency), AmountMinor: -account.BalanceMinor, Currency: account.Currency},
		}
		if err := domain.ValidateBalanced(postings); err != nil {
			return err
		}
	}
	return nil
}

func validateTransfer(in store.ApplyTransferInput) error {
	transfer := in.Transfer
	if transfer.ID == "" || len(transfer.ID) > 128 ||
		len(transfer.IdempotencyKey) < 8 || len(transfer.IdempotencyKey) > 128 ||
		transfer.ActorID == "" || len(transfer.ActorID) > 128 || transfer.CreatedAt.IsZero() ||
		transfer.Intent.FromAccountID == "" || len(transfer.Intent.FromAccountID) > 128 ||
		transfer.Intent.ToAccountID == "" || len(transfer.Intent.ToAccountID) > 128 ||
		transfer.Intent.AmountMinor <= 0 || utf8.RuneCountInString(transfer.Intent.Description) > 200 {
		return fmt.Errorf("%w: invalid transfer", domain.ErrInvalidInput)
	}
	if err := domain.ValidateCurrency(transfer.Currency); err != nil {
		return err
	}
	if in.PostingIDs[0] == "" || in.PostingIDs[1] == "" || in.PostingIDs[0] == in.PostingIDs[1] {
		return fmt.Errorf("%w: two unique posting ids are required", domain.ErrInvalidInput)
	}
	if err := domain.ValidateAuditRecord(in.Audit, transfer.ActorID, transfer.ID); err != nil {
		return err
	}
	if in.Risk != nil {
		if err := domain.ValidateRiskEvent(*in.Risk, transfer.ID); err != nil {
			return err
		}
	}
	return nil
}

func transferFingerprint(transfer domain.Transfer) ([]byte, error) {
	canonical := struct {
		ActorID string                `json:"actor_id"`
		Intent  domain.TransferIntent `json:"intent"`
	}{ActorID: transfer.ActorID, Intent: transfer.Intent}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("encode transfer fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return sum[:], nil
}

func systemAccountID(currency string) string {
	return "system:equity:" + currency
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

func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

func isIdempotencyRace(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		strings.Contains(pgErr.ConstraintName, "actor_id_idempotency_key")
}

func mapDatabaseError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrNotFound) ||
		errors.Is(err, domain.ErrCurrencyMismatch) || errors.Is(err, domain.ErrInsufficientFunds) ||
		errors.Is(err, domain.ErrIdempotencyConflict) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23505", "23514", "22003":
			return fmt.Errorf("%w: database constraint rejected the operation", domain.ErrInvalidInput)
		}
	}
	return fmt.Errorf("PostgreSQL operation failed: %w", err)
}

func waitForRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(1<<attempt) * 5 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
