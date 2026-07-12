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

## Consequences

- retrying after timeouts is safe;
- keys require durable uniqueness in a production adapter;
- tenant scoping and retention must be designed before production;
- request hashing may replace field-by-field comparison later.
