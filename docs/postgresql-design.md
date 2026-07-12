# PostgreSQL durability design

## Purpose

The PostgreSQL adapter is the durable implementation of `store.Repository`.
Migration `deploy/postgres/001_init.sql` defines the storage invariants and the
adapter in `internal/store/postgres` implements their transaction protocol.

## Transfer transaction

Each transfer runs as one `SERIALIZABLE` database transaction:

1. Look up `(actor_id, idempotency_key)`.
2. For an existing key, reconstruct the immutable transfer, verify its stored
   SHA-256 fingerprint and return it only when the new intent matches.
3. Lock both non-system account rows in lexical account-ID order.
4. Recheck existence, currency, available funds and integer boundaries from the
   locked rows.
5. Insert the journal transaction and immutable transfer intent.
6. Update both materialised balances.
7. Insert the debit and credit postings.
8. Append the audit record and optional risk outbox row.
9. Commit. Deferred constraint triggers verify the exact posting count and
   zero-sum invariant against the final transaction state.

PostgreSQL can abort serializable transactions to preserve the isolation
guarantee. SQLSTATE `40001` (serialization failure) and `40P01` (deadlock) cause
a complete rollback and bounded retry. A unique-key race on the actor/key
constraint is also retried so the next attempt can return the winner's result.

## Account creation

A positive opening balance is represented by two postings:

- a positive posting to the customer account;
- an equal negative posting to `system:equity:<CURRENCY>`.

The system account may be negative; ordinary accounts may not. Creation of the
customer account, system-equity update, opening journal entry and audit record
uses one serializable transaction.

## Database constraints

The migration enforces:

- uppercase three-letter currency codes;
- non-negative ordinary account balances;
- reserved ownership and currency-derived IDs for system accounts;
- one actor/key idempotency mapping;
- positive transfer amounts and distinct accounts;
- non-zero postings and one posting per account per transaction;
- exactly `expected_postings` rows and a zero sum at commit;
- append-only journal transactions, postings, transfer intents and audit rows;
- immutable account ownership, currency, system flag and creation identity;
- valid outbox state and bounded stored error text.

Application validation provides precise errors. Database constraints are a
second line of defence against adapter defects or unsafe direct writes.

## Locking and deadlocks

The adapter sorts both account IDs before issuing `SELECT ... FOR UPDATE`.
Every transfer therefore requests the same pair in the same order. This reduces
cycles but does not remove every possible deadlock involving unrelated database
work; the retry path remains required.

No lock is held while publishing a risk event. The transfer commits the outbox
row and returns independently of downstream delivery.

## Idempotency and ambiguous commit results

The database unique constraint is authoritative. The stored fingerprint covers
the actor and complete transfer intent: source, destination, amount and
description. Currency is derived from immutable account identity at execution.

If the connection fails during commit, the caller cannot safely infer whether
the transaction committed. Retrying the same request with the same key resolves
that ambiguity. The adapter never treats a different request body as a replay.

## Outbox delivery

Workers claim eligible rows using a short `READ COMMITTED` transaction and
`FOR UPDATE SKIP LOCKED`. Claiming changes the row to `processing`, increments
the attempt count and records a lease timestamp. Successful publication marks
the event `published`; failure records a truncated reason and schedules bounded
exponential backoff.

A stale `processing` lease becomes claimable after one minute. Delivery is
therefore at least once: a worker can publish and fail before recording success.
External consumers must deduplicate by risk-event ID.

## Reconciliation

`Store.Reconcile` runs read-only at `REPEATABLE READ`. The report checks the
complete journal and compares each stored account balance with
`sum(postings.amount_minor)`. The command exits non-zero on any difference. It
does not mutate or auto-correct financial history.

## Migration and credentials

The Compose environment applies migration 1 only when creating a new PostgreSQL
volume. CI applies it to an empty database and runs executable negative checks.

For a real deployment, migrations should run as a separate controlled step.
Use distinct roles:

- a schema owner that is not used by the service;
- a migration role with time-bound deployment access;
- a runtime role limited to required tables and operations;
- a read-only operations role for reconciliation and investigation.

The current local Compose credentials intentionally do not model that
separation.

## Verified failure cases

- exact idempotent replay;
- idempotency-key conflict;
- concurrent overspend against one source balance;
- missing and cross-currency accounts;
- balance overflow and underflow validation;
- incomplete or unbalanced posting sets;
- a late posting appended to an already completed transaction;
- update/delete attempts against immutable journal history;
- account ownership mutation and account deletion attempts;
- outbox claim and publish lifecycle;
- reconciliation detection of materialised-balance drift.

## References

- [PostgreSQL transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html)
- [PostgreSQL explicit locking and deadlocks](https://www.postgresql.org/docs/current/explicit-locking.html)
