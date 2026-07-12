# Roadmap

## Completed in 0.2 — durable ledger foundation

- PostgreSQL repository adapter with serializable transaction retry;
- deterministic account row locking and concurrent overspend tests;
- durable actor-scoped idempotency and request fingerprints;
- deferred database enforcement of posting count and zero sum;
- append-only journal and audit triggers;
- transactional risk outbox with leased claims and retry;
- executable balance reconciliation;
- Compose-based PostgreSQL environment and integration CI.

## 0.3 — production identity and tenancy

- validated OIDC and service-to-service identities;
- trusted-ingress and token-verification tests;
- tenant-aware account and idempotency scope;
- explicit policy model for operator privileges;
- secrets rotation and separate migration/runtime database roles.

Acceptance requires removing development identity headers from the deployable
configuration and proving cross-tenant isolation with negative tests.

## 0.4 — operational resilience

- OpenTelemetry traces, metrics and structured correlation IDs;
- service-level indicators, objectives and alert thresholds;
- reconciliation schedule and retained reports;
- outbox backlog/dead-letter operations;
- backup automation and a documented restore drill;
- load, fault-injection and graceful-degradation tests.

## 0.5 — accounting breadth

- currency exponent registry;
- holds and available/ledger balance separation;
- explicit reversal and compensating-entry workflows;
- fee and multi-posting transaction types;
- statement pagination and stable cursor semantics;
- account lifecycle and closure rules.

## Later research

- tamper-evident audit export;
- multi-region consistency experiments;
- optional external settlement adapters;
- formal modelling of selected ledger invariants.

Each milestone starts with a failure model, an ADR where the decision is
material and measurable acceptance criteria.
