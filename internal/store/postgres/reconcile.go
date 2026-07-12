package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type BalanceDifference struct {
	AccountID           string `json:"account_id"`
	Currency            string `json:"currency"`
	StoredBalanceMinor  int64  `json:"stored_balance_minor"`
	JournalBalanceMinor int64  `json:"journal_balance_minor"`
}

type ReconciliationReport struct {
	CheckedAt                  time.Time           `json:"checked_at"`
	AccountsChecked            int64               `json:"accounts_checked"`
	UnbalancedTransactionCount int64               `json:"unbalanced_transaction_count"`
	BalanceDifferences         []BalanceDifference `json:"balance_differences"`
}

func (r ReconciliationReport) Clean() bool {
	return r.UnbalancedTransactionCount == 0 && len(r.BalanceDifferences) == 0
}

// Reconcile compares the materialised account balances with the complete
// posting history. A repeatable-read, read-only transaction keeps every query
// on the same database snapshot while transfers continue concurrently.
func (s *Store) Reconcile(ctx context.Context, checkedAt time.Time) (ReconciliationReport, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("begin reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	report := ReconciliationReport{
		CheckedAt:          checkedAt.UTC(),
		BalanceDifferences: make([]BalanceDifference, 0),
	}
	if report.CheckedAt.IsZero() {
		report.CheckedAt = time.Now().UTC()
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM accounts`).Scan(&report.AccountsChecked); err != nil {
		return ReconciliationReport{}, fmt.Errorf("count accounts: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM (
			SELECT jt.id
			FROM journal_transactions jt
			LEFT JOIN postings p ON p.transaction_id = jt.id
			GROUP BY jt.id, jt.expected_postings
			HAVING count(p.id) <> jt.expected_postings
			    OR COALESCE(sum(p.amount_minor), 0) <> 0
		) defects`).Scan(&report.UnbalancedTransactionCount); err != nil {
		return ReconciliationReport{}, fmt.Errorf("check journal balance: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT a.id, a.currency, a.balance_minor,
		       COALESCE(sum(p.amount_minor), 0)::bigint AS journal_balance_minor
		FROM accounts a
		LEFT JOIN postings p ON p.account_id = a.id
		GROUP BY a.id, a.currency, a.balance_minor
		HAVING a.balance_minor <> COALESCE(sum(p.amount_minor), 0)
		ORDER BY a.id`)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("compare account balances: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var difference BalanceDifference
		if err := rows.Scan(
			&difference.AccountID,
			&difference.Currency,
			&difference.StoredBalanceMinor,
			&difference.JournalBalanceMinor,
		); err != nil {
			return ReconciliationReport{}, fmt.Errorf("scan balance difference: %w", err)
		}
		report.BalanceDifferences = append(report.BalanceDifferences, difference)
	}
	if err := rows.Err(); err != nil {
		return ReconciliationReport{}, fmt.Errorf("read balance differences: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return ReconciliationReport{}, fmt.Errorf("commit reconciliation snapshot: %w", err)
	}
	return report, nil
}
