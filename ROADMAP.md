# Roadmap

## 0.2 — Durable persistence

- PostgreSQL adapter with serialisable transactions;
- schema migrations and rollback tests;
- outbox pattern for risk-event delivery;
- deterministic reconciliation job.

## 0.3 — Production identity boundary

- OIDC validation;
- service-to-service identities;
- policy-based authorisation;
- key rotation and secrets management.

## 0.4 — Operational resilience

- OpenTelemetry traces and metrics;
- SLOs and error budgets;
- backup/restore drill;
- incident-response exercise;
- tamper-evident audit export.

## 0.5 — On-chain settlement laboratory

- optional Solidity settlement contract;
- confirmation-depth policy;
- chain reorganisation simulation;
- on-chain/off-chain reconciliation and invariant tests.

Each milestone should be implemented only after its design and threat model can
be explained without relying on generated text.
