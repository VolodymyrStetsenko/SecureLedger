# ADR 0005: Transactional outbox for risk events

- Status: Accepted
- Date: 2026-07-12
- Author: Volodymyr Stetsenko

## Context

Publishing a risk event directly after a transfer commit creates a failure
window: the financial transaction can succeed and the process can terminate
before delivery. Publishing before commit can emit an event for a transfer that
later rolls back.

## Decision

PostgreSQL mode inserts the risk event in the same transaction as the transfer.
A background worker claims committed rows with `FOR UPDATE SKIP LOCKED`, records
a lease and publishes outside the financial transaction. It marks success or
schedules bounded retry. A stale processing lease can be reclaimed.

Delivery is explicitly at least once. Event IDs are stable so consumers can
deduplicate.

## Consequences

- a committed qualifying transfer always has a durable risk row;
- request latency does not depend on the external publisher;
- publication can occur more than once;
- consumers must be idempotent;
- outbox backlog and exhausted attempts require monitoring;
- the memory adapter retains its simpler best-effort dispatcher.
