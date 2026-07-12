# Testing strategy

## Quality gates

| Gate | Command | Evidence |
|---|---|---|
| Formatting | `make check-fmt` | All Go files match `gofmt` |
| Static analysis | `make vet` | Standard Go correctness checks |
| Known vulnerabilities | CI and `govulncheck` | Reachable Go vulnerability analysis |
| Unit/HTTP | `make test` | Domain, service, adapters, worker and HTTP boundary |
| Race detection | `make test-race` | Concurrent memory operations and all ordinary tests |
| PostgreSQL integration | `make test-integration` | Real transactions, idempotency, outbox and overspend |
| Schema negative tests | `scripts/test-postgres-schema.sh` | Deferred balance and append-only triggers reject invalid writes |
| OpenAPI lint | CI | OpenAPI 3.1 structure and contract quality |
| Build | `make build` | Service and reconciliation binaries |
| Security analysis | CodeQL workflow | Go static security queries |

`make check` combines formatting, vet, race-enabled tests and both builds.

## Unit-test boundaries

### Domain

Tests cover balanced and unbalanced postings, too few postings, zero values,
mixed currencies, malformed identifiers and signed overflow. These tests are
storage-independent.

### Application service

Tests cover role policy, customer ownership, transfer limits, risk thresholds,
idempotent replay and conflict behaviour. The application tests use the memory
adapter to exercise complete use cases quickly.

### Memory repository

Tests verify atomic validation, duplicate identifiers, context cancellation,
balance results, journal ordering and concurrent overspend. `go test -race`
checks that the mutex-backed state has no data races.

### HTTP boundary

Tests use `httptest.Server` and assert status, headers and JSON bodies for:

- process health and repository readiness;
- valid account and transfer sequences;
- replay response status/header;
- malformed, duplicate and unknown JSON fields;
- incorrect media type and oversized body;
- invalid identity and list limits;
- stable public errors.

### Risk worker

Fake outbox and publisher implementations verify successful publication,
failure marking, retry time and empty batches without depending on PostgreSQL.

## PostgreSQL integration tests

Start the dependency and execute the tagged suite:

```bash
make test-integration
```

The suite resets only test ledger tables and verifies:

- durable account/transfer lifecycle;
- exact replay and conflicting replay;
- final balances and journal/audit/risk counts;
- outbox claim, failed-delivery delay, retry attempt count and published
  terminal state;
- a clean reconciliation report;
- detection of deliberately introduced balance drift;
- two simultaneous 80-unit transfers from a 100-unit source, where exactly one
  commits and the other receives insufficient funds.

Integration tests are behind the `integration` build tag. A normal `go test
./...` does not silently start infrastructure.

## Schema tests

The shell test applies the migration to an empty CI database and then proves
that:

1. a balanced opening transaction commits;
2. a posting cannot be updated;
3. a third late posting cannot be appended to a completed two-posting entry;
4. an incomplete/unbalanced transaction fails at commit.

The expected-failure `psql` commands print database errors in CI. The script
fails only if PostgreSQL incorrectly accepts the invalid transaction.

## Coverage policy

CI enforces two separate floors:

- 65% across ordinary service packages;
- 50% for the PostgreSQL adapter under integration tests.

A floor prevents accidental collapse; it is not a correctness score. Critical
financial and authorisation branches require explicit assertions even when
overall percentage already passes.

## Adding a financial operation

A new operation should include:

1. domain invariant tests;
2. permission and ownership tests;
3. repository atomicity tests for both adapters;
4. a same-key replay and changed-request conflict test where applicable;
5. a concurrent execution test;
6. PostgreSQL constraint evidence;
7. HTTP success and stable-error tests;
8. threat-model and ADR updates.

## Manual localhost verification

After automated checks, run the complete stack and exercise it through HTTP:

```bash
make compose-up
./scripts/demo.sh
make reconcile-postgres
make compose-down
```

The demo asserts account creation, transfer, exact replay and final balances.
Reconciliation should return JSON with zero differences and exit successfully.
