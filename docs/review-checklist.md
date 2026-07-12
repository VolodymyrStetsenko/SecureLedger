# Engineering review checklist

Use this checklist for changes to money movement, identity, persistence, audit
or risk delivery. A checked item should point to code, a test, or a documented
decision rather than relying on intention.

## Financial model

- Are all amounts integer minor units with explicit sign semantics?
- Does every new transaction type define its expected postings?
- Can the posting count or sum overflow?
- Does the operation preserve the zero-sum invariant?
- Is every materialised balance change reconstructable from postings?
- Are reversal or correction semantics append-only?

## Atomicity and concurrency

- What is the single commit boundary?
- Can any audit, risk or journal write succeed without the matching balance
  change?
- Which rows are locked, and is lock order deterministic?
- Which database errors require a full retry?
- Can retry duplicate money movement or external side effects?
- Is there a concurrent test that asserts final state, not only responses?

## Idempotency

- What is the idempotency scope: actor, tenant and operation?
- Which request fields are bound to the key?
- Does an exact retry return the original result?
- Does a changed request fail with a conflict?
- What happens when two first attempts race?
- How does the client recover from an ambiguous commit result?

## Identity and authorisation

- Where is identity authenticated, and where is permission decided?
- Does knowledge of a resource ID grant any authority accidentally?
- Are customer, operator, administrator and auditor behaviours tested?
- Can a caller bypass a trusted ingress or inject identity headers?
- Does the change require tenant scoping or workload identity?

## API boundary

- Are body size, content type, JSON shape and list limits bounded?
- Are unknown fields rejected?
- Are domain errors mapped consistently without internal detail?
- Does OpenAPI describe every success and error response?
- Are secrets, raw bodies and idempotency keys excluded from logs?

## Persistence and operations

- Does the migration enforce the most important invariant independently?
- Is history protected from update/delete?
- Can old and new application versions coexist during rollout?
- Can reconciliation detect drift introduced by this change?
- Are health/readiness semantics still accurate?
- What alert, backup, restore or incident procedure changes?

## Risk delivery

- Is the event stored in the same transaction as the financial operation?
- Is delivery at-most-once, at-least-once or exactly-once in effect?
- Can a stale claim be recovered?
- Can consumers deduplicate by stable event ID?
- Is retry bounded and observable?

## Evidence before merge

- `make check` passes.
- PostgreSQL integration and schema tests pass when persistence changed.
- OpenAPI lint passes when HTTP behaviour changed.
- The threat model and relevant ADR are current.
- Documentation states any new limitation without overstating assurance.
