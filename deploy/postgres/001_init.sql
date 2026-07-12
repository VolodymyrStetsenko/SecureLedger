-- SecureLedger PostgreSQL schema, migration 1.
-- Deployment applies this migration before starting the PostgreSQL adapter.

BEGIN;

CREATE TABLE schema_migrations (
    version             bigint PRIMARY KEY,
    name                text NOT NULL CHECK (length(name) BETWEEN 1 AND 128),
    applied_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE accounts (
    id                  text PRIMARY KEY,
    owner_id            text NOT NULL,
    currency            char(3) NOT NULL CHECK (currency::text ~ '^[A-Z]{3}$'),
    balance_minor       bigint NOT NULL,
    system              boolean NOT NULL DEFAULT false,
    version             bigint NOT NULL DEFAULT 0 CHECK (version >= 0),
    created_at          timestamptz NOT NULL,
    updated_at          timestamptz NOT NULL,
    CHECK (length(id) BETWEEN 1 AND 128),
    CHECK (length(owner_id) BETWEEN 1 AND 128),
    CHECK (system OR balance_minor >= 0),
    CHECK (NOT system OR owner_id = 'system'),
    CHECK ((system AND id = 'system:equity:' || btrim(currency::text)) OR
           (NOT system AND id NOT LIKE 'system:%'))
);

CREATE INDEX accounts_owner_idx ON accounts (owner_id, created_at DESC);

CREATE TABLE journal_transactions (
    id                  text PRIMARY KEY,
    kind                text NOT NULL CHECK (kind IN ('opening', 'transfer')),
    idempotency_key     text,
    actor_id            text NOT NULL,
    currency            char(3) NOT NULL CHECK (currency::text ~ '^[A-Z]{3}$'),
    description         text NOT NULL DEFAULT '' CHECK (length(description) <= 200),
    expected_postings   smallint NOT NULL DEFAULT 2 CHECK (expected_postings >= 2),
    created_at          timestamptz NOT NULL,
    CHECK (length(id) BETWEEN 1 AND 128),
    CHECK (length(actor_id) BETWEEN 1 AND 128),
    UNIQUE (actor_id, idempotency_key),
    CHECK ((kind = 'opening' AND idempotency_key IS NULL) OR
           (kind = 'transfer' AND length(idempotency_key) BETWEEN 8 AND 128))
);

CREATE TABLE postings (
    id                  text PRIMARY KEY,
    transaction_id      text NOT NULL REFERENCES journal_transactions(id) ON DELETE RESTRICT,
    account_id          text NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount_minor        bigint NOT NULL CHECK (amount_minor <> 0),
    sequence_no         bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    created_at          timestamptz NOT NULL,
    CHECK (length(id) BETWEEN 1 AND 128),
    UNIQUE (transaction_id, account_id)
);

CREATE INDEX postings_transaction_idx ON postings (transaction_id, id);
CREATE INDEX postings_account_idx ON postings (account_id, created_at DESC, id);
CREATE INDEX postings_sequence_idx ON postings (sequence_no);

CREATE TABLE transfer_intents (
    transaction_id      text PRIMARY KEY REFERENCES journal_transactions(id) ON DELETE RESTRICT,
    from_account_id     text NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    to_account_id       text NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    amount_minor        bigint NOT NULL CHECK (amount_minor > 0),
    request_fingerprint bytea NOT NULL,
    CHECK (octet_length(request_fingerprint) = 32),
    CHECK (from_account_id <> to_account_id)
);

CREATE TABLE audit_records (
    sequence_no         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id                  text NOT NULL UNIQUE,
    actor_id            text NOT NULL,
    action              text NOT NULL,
    resource_id         text NOT NULL,
    outcome             text NOT NULL,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL,
    CHECK (length(id) BETWEEN 1 AND 128),
    CHECK (length(actor_id) BETWEEN 1 AND 128),
    CHECK (length(action) BETWEEN 1 AND 128),
    CHECK (length(resource_id) BETWEEN 1 AND 128),
    CHECK (length(outcome) BETWEEN 1 AND 64),
    CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX audit_records_resource_idx ON audit_records (resource_id, sequence_no DESC);

CREATE TABLE risk_events (
    id                  text PRIMARY KEY,
    event_type          text NOT NULL,
    severity            text NOT NULL CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    transaction_id      text NOT NULL REFERENCES journal_transactions(id) ON DELETE RESTRICT,
    reason              text NOT NULL,
    status              text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'published', 'failed')),
    attempts            integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    created_at          timestamptz NOT NULL,
    available_at        timestamptz NOT NULL,
    locked_at           timestamptz,
    published_at        timestamptz,
    last_error          text CHECK (last_error IS NULL OR length(last_error) <= 500),
    CHECK (length(id) BETWEEN 1 AND 128),
    CHECK (length(event_type) BETWEEN 1 AND 64),
    CHECK (length(reason) BETWEEN 1 AND 500)
);

CREATE INDEX risk_events_delivery_idx ON risk_events (status, available_at, created_at)
    WHERE status IN ('pending', 'processing', 'failed');

CREATE FUNCTION assert_balanced_journal_transaction()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
    target_transaction_id text;
    posting_count bigint;
    posting_sum numeric;
    expected_count smallint;
BEGIN
    IF TG_TABLE_NAME = 'journal_transactions' THEN
        target_transaction_id := NEW.id;
    ELSE
        target_transaction_id := NEW.transaction_id;
    END IF;

    SELECT count(*), COALESCE(sum(amount_minor), 0)
      INTO posting_count, posting_sum
      FROM public.postings
     WHERE transaction_id = target_transaction_id;

    SELECT expected_postings
      INTO expected_count
      FROM public.journal_transactions
     WHERE id = target_transaction_id;

    IF posting_count <> expected_count OR posting_sum <> 0 THEN
        RAISE EXCEPTION 'journal transaction % is not balanced', target_transaction_id
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE CONSTRAINT TRIGGER journal_transaction_balance_on_transaction
AFTER INSERT ON journal_transactions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION assert_balanced_journal_transaction();

CREATE CONSTRAINT TRIGGER journal_transaction_balance_on_posting
AFTER INSERT ON postings
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION assert_balanced_journal_transaction();

CREATE FUNCTION reject_ledger_history_mutation()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME
        USING ERRCODE = '55000';
END;
$$;

CREATE FUNCTION protect_account_identity()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'accounts cannot be deleted'
            USING ERRCODE = '55000';
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
       OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.system IS DISTINCT FROM OLD.system
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'account identity fields are immutable'
            USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER accounts_identity_immutable
BEFORE UPDATE OR DELETE ON accounts
FOR EACH ROW EXECUTE FUNCTION protect_account_identity();

CREATE TRIGGER journal_transactions_append_only
BEFORE UPDATE OR DELETE ON journal_transactions
FOR EACH ROW EXECUTE FUNCTION reject_ledger_history_mutation();

CREATE TRIGGER postings_append_only
BEFORE UPDATE OR DELETE ON postings
FOR EACH ROW EXECUTE FUNCTION reject_ledger_history_mutation();

CREATE TRIGGER transfer_intents_append_only
BEFORE UPDATE OR DELETE ON transfer_intents
FOR EACH ROW EXECUTE FUNCTION reject_ledger_history_mutation();

CREATE TRIGGER audit_records_append_only
BEFORE UPDATE OR DELETE ON audit_records
FOR EACH ROW EXECUTE FUNCTION reject_ledger_history_mutation();

INSERT INTO schema_migrations (version, name)
VALUES (1, 'initial ledger schema');

COMMIT;
