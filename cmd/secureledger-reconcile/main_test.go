package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	postgresstore "github.com/VolodymyrStetsenko/secureledger/internal/store/postgres"
)

type fakeReconciler struct {
	report postgresstore.ReconciliationReport
	err    error
}

func (f fakeReconciler) Reconcile(context.Context, time.Time) (postgresstore.ReconciliationReport, error) {
	return f.report, f.err
}

func TestReconcileWritesCleanReport(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	report := postgresstore.ReconciliationReport{
		CheckedAt:          time.Unix(1, 0).UTC(),
		AccountsChecked:    2,
		BalanceDifferences: []postgresstore.BalanceDifference{},
	}
	if err := reconcile(context.Background(), fakeReconciler{report: report}, report.CheckedAt, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"accounts_checked":2`) {
		t.Fatalf("unexpected report: %s", output.String())
	}
}

func TestReconcileReturnsErrorForDifference(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	report := postgresstore.ReconciliationReport{
		BalanceDifferences: []postgresstore.BalanceDifference{{AccountID: "account"}},
	}
	if err := reconcile(context.Background(), fakeReconciler{report: report}, time.Now(), &output); err == nil {
		t.Fatal("integrity difference returned success")
	}
	if output.Len() == 0 {
		t.Fatal("integrity report was not written")
	}
}

func TestReconcilePropagatesRepositoryError(t *testing.T) {
	t.Parallel()
	want := errors.New("database unavailable")
	if err := reconcile(context.Background(), fakeReconciler{err: want}, time.Now(), &bytes.Buffer{}); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("SECURELEDGER_DATABASE_URL", "")
	if err := run(); err == nil {
		t.Fatal("missing database URL was accepted")
	}
}
