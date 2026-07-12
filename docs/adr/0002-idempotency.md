# ADR 0002: Mandatory idempotency for transfers

- Status: Accepted
- Date: 2026-07-12
- Author: Volodymyr Stetsenko

## Context

Networks fail ambiguously. A client may not know whether a transfer committed
and can retry the request.

## Decision

Every transfer requires an idempotency key. The repository atomically binds the
actor-and-key pair to one immutable transfer intent. An identical retry returns
the original result. Reusing the pair for a different intent returns a conflict.
The PostgreSQL adapter stores a SHA-256 fingerprint of the actor and canonical
intent and verifies that fingerprint when reconstructing a replay.

## Consequences

- retrying after timeouts is safe;
- PostgreSQL enforces durable uniqueness for the actor-and-key pair;
- tenant scoping and retention must be designed before production;
- field comparison remains authoritative for the public conflict response;
- clients must retain and reuse keys after ambiguous network failures.
