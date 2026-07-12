# Threat Model

## Method

This model combines asset/trust-boundary analysis with STRIDE-style prompts.
It is deliberately specific to the current implementation.

## Assets

- account balances;
- journal integrity;
- idempotency mappings;
- actor identity and role;
- audit records;
- risk events;
- service availability;
- configuration values.

## Trust boundaries

1. Untrusted client to HTTP API.
2. HTTP identity headers to authorisation policy.
3. Application service to repository atomicity boundary.
4. Committed transaction to asynchronous risk dispatcher.
5. Process memory to host operating system.

## Primary threats and controls

| Threat | Example | Current control | Residual risk / next step |
|---|---|---|---|
| Spoofing | forge `X-Principal-ID` | explicitly local-only | replace with validated OIDC/JWT |
| Tampering | alter one side of a transfer | repository creates both postings atomically | durable DB constraints |
| Replay | resend a transfer after timeout | mandatory actor-scoped idempotency key | bind key to request hash durably |
| Elevation | customer moves another owner's funds | ownership-aware policy | external policy engine / tests |
| Race | concurrent transfers overspend | repository mutex and race tests | row locking/serialisable DB |
| Repudiation | actor denies transfer | append-only audit record | signed/tamper-evident export |
| Information disclosure | verbose internal errors | stable public error mapping | structured redaction policy |
| Denial of service | oversized JSON body | 1 MiB request limit, timeouts | rate limiting and quotas |
| Event loss | risk worker unavailable | event stored synchronously | transactional outbox |
| Integer error | floating-point rounding | `int64` minor units | currency-specific scale registry |

## Abuse cases

### Duplicate payment

A client times out after a successful commit and retries. The same idempotency
key must return the original transfer without adding postings.

### Horizontal privilege escalation

A customer knows another account ID and attempts to debit it. The service checks
that the actor owns the source account unless the role is operator or admin.

### Concurrent overspend

Multiple goroutines transfer from one account simultaneously. The repository
serialises the state transition and rejects operations after funds are
exhausted.

### Idempotency-key confusion

A client reuses a key for a different transfer intent. The repository compares
the original and new intent and returns a conflict instead of replaying an
unrelated transfer. The scope includes the actor identifier to avoid collisions
between independent clients; a production design should add tenant scope.

## Out of scope for version 0.1

- compromise of the host or Go runtime;
- cryptographic key management;
- malicious database administrator;
- multi-region failure;
- sanctions/AML decisions;
- card-data processing;
- public blockchain settlement;
- formal regulatory compliance.
