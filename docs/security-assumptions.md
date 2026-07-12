# Security Assumptions and Deployment Warning

## Intended use

This version is for local learning, code review, demonstrations and authorised
testing.

## Authentication

The service accepts `X-Principal-ID` and `X-Principal-Role` headers. This makes
local API exploration easy, but it is **not authentication**. Any client can
forge them.

Do not expose the service to the internet. A production boundary must validate
tokens issued by a trusted identity provider, enforce audience/issuer/expiry,
rotate keys and distinguish human from workload identities.

## Persistence

The default repository is in memory. Process termination destroys all state.
The implementation demonstrates atomic business logic, not durability.

## Money representation

Amounts use signed 64-bit integers in minor units. The current currency
validation accepts three uppercase letters and assumes two-decimal currencies
for human interpretation. Production code needs a currency metadata registry
and overflow analysis.

## Availability

The service sets HTTP timeouts and limits request bodies. It does not implement
rate limiting, admission control, circuit breakers or load shedding.

## Audit evidence

Audit entries are append-only through the public interface, but a process or
host administrator can alter memory. Production evidence needs durable,
access-controlled, tamper-evident storage and retention rules.

## Risk processing

Risk events are written synchronously to the repository, then sent to a
best-effort background dispatcher. The dispatcher may drop alerts when its
buffer is full. A transactional outbox is required for reliable delivery.
