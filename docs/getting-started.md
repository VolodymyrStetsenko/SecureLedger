# Getting started

## Choose an execution mode

| Mode | Use it for | Persistence | Requirements |
|---|---|---|---|
| Memory | Fast development, API exploration and unit tests | Lost at process exit | Go 1.26+ |
| PostgreSQL | Durable local execution and concurrency verification | Docker volume | Docker Engine + Compose |

Both modes expose the same HTTP contract. PostgreSQL mode additionally enables
the durable risk outbox and reconciliation command.

## Clone and inspect commands

```bash
git clone https://github.com/VolodymyrStetsenko/SecureLedger.git
cd SecureLedger
make help
```

`make help` is generated from the Makefile target descriptions, so it stays in
sync with available commands.

## Run in memory

```bash
make check
make run
```

In another terminal:

```bash
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/readyz
./scripts/demo.sh
```

Expected health responses are `{"status":"ok"}` and
`{"status":"ready"}`. Stop with `Ctrl-C`; the server allows up to ten
seconds for graceful HTTP shutdown.

## Run with PostgreSQL

Start the complete stack:

```bash
make compose-up
docker compose ps
curl --fail http://localhost:8080/readyz
./scripts/demo.sh
```

On the first start, the official PostgreSQL image applies
`deploy/postgres/001_init.sql` to the new named volume. Later starts reuse that
volume and do not re-run the initialization script.

Stop containers while preserving data:

```bash
make compose-down
```

To remove the local database deliberately, first stop the stack and then run:

```bash
docker compose down --volumes
```

That command irreversibly deletes the local Compose data volume. It is not part
of an ordinary restart.

## Run only the database

This is useful when running the Go process directly:

```bash
make postgres-up
make run-postgres
```

The default development connection URI is:

```text
postgresql://secureledger:secureledger@localhost:5432/secureledger?sslmode=disable
```

It is intentionally local and must not be reused as a production secret.

## Configuration

Copy `.env.example` only when a local launcher needs an environment file. Docker
Compose already declares its own local values.

```bash
export SECURELEDGER_ADDR=:8080
export SECURELEDGER_STORE=memory
export SECURELEDGER_LOG_LEVEL=debug
go run ./cmd/secureledger
```

For PostgreSQL:

```bash
export SECURELEDGER_STORE=postgres
export SECURELEDGER_DATABASE_URL='postgresql://secureledger:secureledger@localhost:5432/secureledger?sslmode=disable'
go run ./cmd/secureledger
```

The process fails closed at startup when the database URI is missing,
malformed, unavailable or not migrated.

## Verify persistence

1. Start the Compose stack.
2. Run `./scripts/demo.sh` and retain an account ID.
3. Run `docker compose restart app`.
4. Read the account again using the same development identity headers.

The account remains because only the application process restarted. Memory mode
cannot provide this property.

## Common failures

### Port already in use

Change `SECURELEDGER_ADDR` for a direct Go run, or stop the local process using
port 8080. Compose intentionally publishes only `127.0.0.1:8080`.

### Database reports missing tables

The database was created without migration 1, or an old volume predates the
schema. For disposable local data, recreate the volume. For non-disposable data,
inspect migration state and apply a reviewed forward migration; do not delete
the volume.

### `DATABASE_URL is required`

`scripts/test-postgres-schema.sh` and PostgreSQL integration tests require an
explicit URL. `make test-integration` supplies the documented local default.

### Readiness returns 503

The repository ping failed. Inspect application and PostgreSQL logs:

```bash
docker compose logs app postgres
```

Readiness deliberately does not expose the database error to the HTTP caller.
