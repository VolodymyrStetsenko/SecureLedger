# Operations runbook

## Start and stop

For the reproducible local stack:

```bash
make compose-up
docker compose ps
make compose-down
```

The application runs as a non-root user in a read-only distroless container
with Linux capabilities dropped and privilege escalation disabled. PostgreSQL
data resides in the named Compose volume.

For direct execution with PostgreSQL:

```bash
make postgres-up
make run-postgres
```

Send `SIGTERM` or `SIGINT` for graceful shutdown. The process stops accepting
new work and gives active HTTP requests up to ten seconds to finish.

## Health signals

| Endpoint | Success | Meaning |
|---|---|---|
| `GET /healthz` | `200 {"status":"ok"}` | HTTP process is alive |
| `GET /readyz` | `200 {"status":"ready"}` | Repository ping succeeded |

`/readyz` returns `503` and a stable public error when PostgreSQL is unavailable.
It does not expose connection details. Neither endpoint verifies an external
risk publisher.

## Structured logs

The service emits JSON logs in the container. Request logs include method,
path, response status and elapsed milliseconds. They exclude body content,
identity headers, idempotency keys and database URLs.

Inspect local logs with:

```bash
docker compose logs --tail=200 app
docker compose logs --tail=200 postgres
```

Risk publication logs include only event ID, type, severity and transfer ID.
Treat those identifiers as operational metadata and apply an appropriate
retention/access policy in a central log system.

## Reconciliation

Run against the default local PostgreSQL instance:

```bash
make reconcile-postgres
```

A clean result resembles:

```json
{
  "checked_at": "2026-07-12T12:00:00Z",
  "accounts_checked": 3,
  "unbalanced_transaction_count": 0,
  "balance_differences": []
}
```

Any journal or balance difference produces a JSON report and a non-zero exit.
Do not repair it with direct updates or delete history. Preserve logs and a
database snapshot, stop unsafe writes, identify the first divergence, and use a
reviewed compensating-entry procedure appropriate to the incident.

The current project does not schedule reconciliation. A deployed environment
should run it periodically, retain reports and alert on non-zero exits.

## Risk outbox

The normal states are `pending`, `processing`, `failed` and `published`.
Repeated `failed` rows or stale `processing` rows indicate publisher or worker
problems. The worker reclaims a processing lease after one minute and stops
claiming an event after ten attempts.

The included publisher logs events immediately, so persistent failures normally
indicate a database operation problem. An external publisher implementation
must add delivery metrics, consumer deduplication and a reviewed dead-letter
procedure.

## Ambiguous transfer response

If a client loses the connection during a transfer, do not advise it to create
a new key. It should retry the identical request using the original actor and
idempotency key. A `201` response means the retry created the transfer; a `200`
with `Idempotent-Replayed: true` means it had already committed.

## Database migration

Migration 1 is applied automatically only for a newly created local Compose
volume. CI also applies it to a clean database. Before a future migration:

1. back up and restore into an isolated environment;
2. test old and new application compatibility;
3. estimate lock duration and table rewrite risk;
4. define rollback or forward-fix behaviour;
5. run reconciliation before and after;
6. deploy using a separate migration role.

Do not grant the runtime service permission to replace append-only triggers.

## Backup and restore expectations

No automated backup policy is supplied. A real deployment needs documented
recovery point and recovery time objectives, encrypted backups, access logging,
retention, restore drills and post-restore reconciliation. A backup that has not
been restored and checked is not sufficient recovery evidence.

## Incident priorities

For a suspected financial-integrity incident:

1. prevent additional unsafe writes while preserving evidence;
2. record affected environment, time range and deployed commit;
3. run reconciliation against a consistent copy when possible;
4. inspect transaction, posting, audit and outbox identifiers together;
5. determine whether customer-visible balances or only derived state differ;
6. restore service only with a reviewed containment or correction plan;
7. add a regression test and update the threat model after root cause is known.

## Known operational gaps

The repository does not yet provide metrics, traces, dashboards, automated
backups, failover, rate limiting, schema rollout automation or an external
event broker. These remain prerequisites for a production service.
