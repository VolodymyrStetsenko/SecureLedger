# ADR 0003: Repository owns the atomicity boundary

- Status: Accepted
- Date: 2026-07-12
- Author: Volodymyr Stetsenko

## Context

If balance updates, journal postings, audit records and risk records are written
separately, partial failure can corrupt financial state.

## Decision

The repository exposes `ApplyTransfer` as one atomic operation. The application
service performs policy checks but does not orchestrate individual writes.

## Consequences

- consistency requirements are explicit;
- the memory implementation can use one lock;
- a PostgreSQL adapter must implement the same all-or-nothing contract;
- repository methods are domain-specific rather than generic CRUD.
