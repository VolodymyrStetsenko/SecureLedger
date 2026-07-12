package postgres

import "testing"

func TestReconciliationReportClean(t *testing.T) {
	if !(ReconciliationReport{}).Clean() {
		t.Fatal("empty reconciliation report should be clean")
	}
	if (ReconciliationReport{UnbalancedTransactionCount: 1}).Clean() {
		t.Fatal("unbalanced transaction was treated as clean")
	}
	if (ReconciliationReport{BalanceDifferences: []BalanceDifference{{AccountID: "account"}}}).Clean() {
		t.Fatal("balance difference was treated as clean")
	}
}
