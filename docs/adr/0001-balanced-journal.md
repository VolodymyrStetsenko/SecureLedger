# ADR 0001: Balanced journal as the financial source of truth

- Status: Accepted
- Date: 2026-07-12
- Author: Volodymyr Stetsenko

## Context

Directly mutating account balances makes it difficult to explain or reconstruct
money movement. Financial systems need an immutable record of why balances
changed.

## Decision

Every state-changing money operation creates one journal transaction with
multiple signed postings whose sum is zero. Transfers create exactly two
postings. Opening balances are offset against an internal system-equity account.

## Consequences

- transaction history is reconstructable;
- invariant tests are straightforward;
- materialised balances can be reconciled;
- storage and query complexity increase;
- account-type-specific debit/credit semantics remain a future refinement.
