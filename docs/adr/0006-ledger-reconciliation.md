# ADR 0006: Reconciliation detects but does not repair drift

- Status: Accepted
- Date: 2026-07-12
- Author: Volodymyr Stetsenko

## Context

Materialised balances make account reads efficient, but any stored derived value
can diverge from its evidence because of defects, unsafe database access or an
incorrect migration.

## Decision

Provide a standalone PostgreSQL reconciliation command. It reads a
repeatable-read snapshot, verifies journal posting counts and sums, and compares
every stored account balance with the complete posting history. It returns a
JSON report and a non-zero exit when it finds a difference.

The command does not mutate balances or journal history.

## Consequences

- integrity checks are executable and automatable;
- concurrent transfers do not create a mixed-snapshot false positive;
- operators receive exact account differences for investigation;
- scheduling, alerting and report retention remain deployment responsibilities;
- correction requires an explicit reviewed accounting policy.
