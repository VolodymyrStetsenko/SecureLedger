# Contributing

Thank you for helping improve SecureLedger.

## Development workflow

1. Create a focused branch.
2. Add or update tests before changing behaviour.
3. Run `make check` and `make test-race`.
4. Document security-relevant trade-offs in an ADR.
5. Keep pull requests small enough to review.

## Commit style

Use imperative, scoped messages, for example:

- `ledger: enforce balanced postings`
- `api: reject missing idempotency key`
- `docs: document replay threat`

## Security-sensitive changes

Changes involving authentication, authorisation, money movement, idempotency,
audit logging, or persistence require:

- a threat-model update;
- negative tests;
- a rollback or migration note;
- an explicit statement of assumptions.

AI-assisted contributions are welcome, but the contributor remains responsible
for understanding and validating every line.
