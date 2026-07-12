# Interview Discussion Guide

This document is for the maintainer's learning. Do not memorise answers; use the
questions to verify understanding.

## Ledger model

- Why are floating-point numbers unsafe for money?
- What does “balanced postings” mean?
- Why is an opening balance offset against a system account?
- Is the materialised balance or the journal the source of truth?
- How would reconciliation detect corruption?

## Concurrency

- What prevents two concurrent transfers from overspending?
- Why would a mutex stop working after horizontal scaling?
- Which PostgreSQL isolation level or locking strategy would you choose?
- How would you avoid deadlocks when locking two accounts?

## Idempotency

- Why can a successful HTTP request still look failed to the client?
- Why must one key be bound to one request intent?
- How long should keys be retained?
- Should keys be scoped by tenant or actor?

## Security

- Why are the identity headers unsafe?
- Which asset is most important: balances, journal or audit records?
- What is the difference between authentication and authorisation?
- What failures require an outbox pattern?

## Honest positioning

A strong answer includes current limitations. Never claim that version 0.1 is
ready for regulated production.
