package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	postgresstore "github.com/VolodymyrStetsenko/secureledger/internal/store/postgres"
)

type reconciler interface {
	Reconcile(context.Context, time.Time) (postgresstore.ReconciliationReport, error)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo, err := postgresstore.Open(ctx, os.Getenv("SECURELEDGER_DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("open PostgreSQL repository: %w", err)
	}
	defer repo.Close()

	return reconcile(ctx, repo, time.Now(), os.Stdout)
}

func reconcile(ctx context.Context, repo reconciler, checkedAt time.Time, output io.Writer) error {
	report, err := repo.Reconcile(ctx, checkedAt)
	if err != nil {
		return fmt.Errorf("reconcile ledger: %w", err)
	}
	if err := json.NewEncoder(output).Encode(report); err != nil {
		return fmt.Errorf("write reconciliation report: %w", err)
	}
	if !report.Clean() {
		return fmt.Errorf("ledger reconciliation found integrity differences")
	}
	return nil
}
