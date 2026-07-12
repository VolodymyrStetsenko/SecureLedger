# ADR 0004: Serializable PostgreSQL ledger transactions

- Status: Accepted
- Date: 2026-07-12
- Author: Volodymyr Stetsenko

## Context

Multiple service instances can attempt to debit the same account concurrently.
Read-then-write code at the default isolation level can admit lost updates or
make correctness depend on fragile statement ordering.

## Decision

Account creation and transfers execute in PostgreSQL `SERIALIZABLE`
transactions. Transfers lock both accounts in lexical ID order, then validate
funds from the locked rows. SQLSTATE `40001` and `40P01` trigger a bounded retry
of the complete operation. A concurrent unique-key race on idempotency also
retries from the beginning.

Database constraints independently enforce non-negative ordinary balances,
idempotency uniqueness and balanced journal completion.

## Consequences

- concurrent transfer histories are equivalent to a serial execution;
- every retry re-runs validation against current committed state;
- deterministic row order reduces but does not eliminate deadlocks;
- high contention can increase aborts and latency;
- callers still need idempotency for ambiguous connection failures;
- production operation should measure retry rate and exhaustion.
