# PostgreSQL durability design

Version 0.1 uses an in-memory adapter so reviewers can inspect the financial
rules without infrastructure noise. This document defines the next durable
adapter rather than pretending persistence already exists.

The proposed schema is in `deploy/postgres/001_init.sql`.

## Transaction algorithm

For a transfer, the adapter should execute one database transaction:

1. Begin at `SERIALIZABLE`, or use `READ COMMITTED` with explicit row locks and
   a bounded retry policy.
2. Insert the journal transaction using a unique actor-and-idempotency-key pair.
3. If the key already exists, compare the stored request fingerprint. Return
   the original result only when the intent is byte-for-byte equivalent.
4. Lock both account rows in deterministic account-ID order to reduce deadlock
   risk.
5. Verify ownership-independent domain conditions: same currency, positive
   amount, sufficient source balance and transfer limits already authorised by
   the application layer.
6. Insert exactly two postings whose signed sum is zero.
7. Update the materialised balances with optimistic version checks.
8. Append the audit record and durable risk/outbox event.
9. Let the deferred database constraint recalculate the posting count and sum
   before commit; reject fewer than two postings or a non-zero sum.
10. Commit, then publish outbox events asynchronously.

## Why both journal and balance

The immutable postings are the accounting evidence. `accounts.balance_minor`
is a materialised view for fast reads. A reconciliation job must periodically
recompute balances from postings and alert on divergence.

## Failure cases that require tests

- two concurrent debits against the same source account;
- duplicate idempotency keys with different request bodies;
- deadlock retry exhaustion;
- process termination after commit but before event publication;
- migration rollback and mixed application versions;
- integer boundary conditions;
- replica lag affecting read-after-write behaviour;
- a posting inserted without a balancing counterpart.

## Security boundary

Database constraints are defence in depth, not a replacement for application
validation. The production database role should not own the schema, and normal
runtime credentials should not be permitted to update or delete historical
postings and audit records directly. The target migration also installs
append-only triggers; privilege separation remains required because a schema
owner can disable or replace them.
