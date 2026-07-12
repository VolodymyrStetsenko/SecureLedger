# Changelog

All notable changes will be documented here.

## [0.1.1] - 2026-07-12

### Security and correctness

- scoped idempotency keys by actor;
- made repository input validation and failed account creation atomic;
- rejected zero, malformed and overflowing posting sets;
- added strict media-type, request-size and list-limit handling;
- aligned JSON output with the documented snake-case API contract;
- added deferred PostgreSQL balance constraints and append-only triggers;
- upgraded CI to Go 1.26 and CodeQL Action v4.

### Documentation and tests

- completed OpenAPI request, response and error schemas;
- expanded negative, concurrency, context and HTTP-boundary tests;
- added an executable PostgreSQL schema verification script;
- corrected the demo script's JSON quoting.
- anchored the generated-binary ignore rule so `cmd/secureledger` remains tracked.

## [0.1.0] - 2026-07-12

### Added

- executable Go reference service;
- balanced two-posting journal transactions;
- idempotent transfers;
- role- and ownership-aware authorisation;
- transaction limits and risk events;
- append-only audit records;
- race-safe in-memory repository;
- HTTP API, OpenAPI contract, tests, threat model and ADRs.
