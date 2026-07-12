#!/usr/bin/env sh
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f deploy/postgres/001_init.sql

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
INSERT INTO accounts (id, owner_id, currency, balance_minor, system, created_at, updated_at)
VALUES
    ('system:equity:GBP', 'system', 'GBP', -100, true, now(), now()),
    ('account:test', 'alice', 'GBP', 100, false, now(), now());
INSERT INTO journal_transactions (id, kind, actor_id, currency, created_at)
VALUES ('transaction:opening', 'opening', 'operator:test', 'GBP', now());
INSERT INTO postings (id, transaction_id, account_id, amount_minor, created_at)
VALUES
    ('posting:opening:1', 'transaction:opening', 'account:test', 100, now()),
    ('posting:opening:2', 'transaction:opening', 'system:equity:GBP', -100, now());
COMMIT;

DO $$
BEGIN
    BEGIN
        UPDATE postings SET amount_minor = 99 WHERE id = 'posting:opening:1';
        RAISE EXCEPTION 'append-only trigger did not reject mutation';
    EXCEPTION
        WHEN SQLSTATE '55000' THEN NULL;
    END;
END;
$$;
SQL

if psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;
INSERT INTO journal_transactions (id, kind, idempotency_key, actor_id, currency, created_at)
VALUES ('transaction:unbalanced', 'transfer', 'schema-test-key', 'alice', 'GBP', now());
INSERT INTO postings (id, transaction_id, account_id, amount_minor, created_at)
VALUES ('posting:unbalanced', 'transaction:unbalanced', 'account:test', -1, now());
COMMIT;
SQL
then
  echo "unbalanced journal transaction was accepted" >&2
  exit 1
fi

echo "PostgreSQL schema checks passed"
