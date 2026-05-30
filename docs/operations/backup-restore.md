# MIDAS Postgres Backup, Restore, and Evidence Verification

This guide defines the operator drill for proving that a MIDAS Postgres backup
can be restored and that governed runtime evidence remains usable after restore.

It is not a backup product design, HA design, RPO/RTO claim, or Bank Tier 1
readiness claim. Use standard Postgres backup mechanisms and run this drill only
against local or disposable non-production databases unless an environment owner
has explicitly approved a staging target.

## 1. Backup Scope

| Domain | Required in backup? | Why |
|---|---|---|
| `operational_envelopes` | Yes | Persisted governed decisions, request idempotency scope, submitted request evidence, resolved authority snapshot, outcome, and integrity anchor. |
| `audit_events` | Yes | Hash-chained runtime audit trail used to verify how each decision was reached. |
| `outbox_events` | Yes | Transactional downstream-publication intent and published/unpublished posture. |
| Authority/configuration tables | Yes | `decision_surfaces`, `authority_profiles`, `authority_grants`, `agents`, `fail_mode_policies`, and related control data are required to interpret historic evidence and future retries. |
| Structural graph tables | Yes | `business_services`, `processes`, `capabilities`, relationships, and AI-system bindings explain the service/process/surface context recorded on envelopes. |
| Control-plane audit | Yes | `controlplane_audit_events` records governed configuration changes. |
| Platform/admin audit | Yes | `platform_admin_audit_events` records high-value administrative actions such as apply invocation and local IAM changes. |
| Local IAM/session tables | Deployment-dependent | Required when the deployment relies on local IAM for operator access to restored evidence APIs. |
| Explorer projections/read models | Rebuildable if derived | Back up if they are the only practical operator view, but they must not replace runtime truth. |

## 2. Safe Drill Preconditions

- Confirm the source database is local or explicitly disposable.
- Create a fresh restore target; do not restore over a live runtime database.
- Stop or disable the dispatcher while inspecting restored outbox posture.
- Keep the original request body for at least one governed evaluation so
  idempotency can be replayed after restore.
- Record the source database name, restore database name, backup file name,
  MIDAS git commit, Postgres version, and whether the dispatcher was enabled.

## 3. Backup Command Pattern

For small local or staging drills, a logical custom-format backup is sufficient:

```powershell
docker exec midas-postgres pg_dump `
  -U midas `
  -d midas_source_drill `
  -Fc `
  -f /tmp/midas_source_drill.dump
```

For larger environments, use the platform-standard physical/WAL backup system
instead, such as managed snapshots, `pgBackRest`, or an approved Postgres HA
tooling stack. The post-restore evidence checks below still apply.

## 4. Restore Command Pattern

Restore into a fresh database:

```powershell
docker exec midas-postgres createdb -U midas midas_restore_drill

docker exec midas-postgres pg_restore `
  -U midas `
  -d midas_restore_drill `
  --no-owner `
  /tmp/midas_source_drill.dump
```

Do not use `--clean` against any database unless it is known disposable and the
target database name has been checked.

## 5. Post-Restore SQL Checks

Count the core evidence tables:

```powershell
docker exec midas-postgres psql -U midas -d midas_restore_drill `
  -v ON_ERROR_STOP=1 `
  -c "SELECT 'operational_envelopes' AS table_name, count(*) FROM operational_envelopes
      UNION ALL SELECT 'audit_events', count(*) FROM audit_events
      UNION ALL SELECT 'outbox_events', count(*) FROM outbox_events;"
```

Check restored outbox posture:

```powershell
docker exec midas-postgres psql -U midas -d midas_restore_drill `
  -v ON_ERROR_STOP=1 `
  -c "SELECT count(*) FILTER (WHERE published_at IS NULL) AS unpublished,
             count(*) FILTER (WHERE published_at IS NOT NULL) AS published
      FROM outbox_events;"
```

For a known request scope, confirm there is still exactly one envelope per
`(request_source, request_id)`:

```powershell
docker exec midas-postgres psql -U midas -d midas_restore_drill `
  -v ON_ERROR_STOP=1 `
  -c "SELECT request_source, request_id, count(*)
      FROM operational_envelopes
      WHERE request_source = 'restore-drill'
      GROUP BY request_source, request_id
      ORDER BY request_id;"
```

## 6. Audit Verification

MIDAS has per-envelope HTTP verification today. Start MIDAS against the restored
database with the dispatcher disabled, then verify one or more restored
envelopes:

```powershell
Invoke-RestMethod `
  -Uri "http://127.0.0.1:18081/v1/evidence/envelopes/$ENVELOPE_ID/integrity"
```

Expected result:

- `valid` is `true`;
- `chain_length` is greater than zero;
- `first_event_hash` and `final_event_hash` are populated.

For operator review, export the composed packet:

```powershell
Invoke-RestMethod `
  -Uri "http://127.0.0.1:18081/v1/evidence/envelopes/$ENVELOPE_ID/packet"
```

The code-level verifier also supports all-envelope verification through
`audit.VerifyAuditIntegrity`, but MIDAS does not yet ship a first-class
operator CLI for running that bulk verification directly against a restored
database.

## 7. Idempotency Check

Replay the exact original request body for a restored envelope:

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri "http://127.0.0.1:18081/v1/evaluate" `
  -ContentType "application/json" `
  -Body $originalBody
```

Expected result:

- the response returns the original `envelope_id`;
- the outcome and audit status are unchanged;
- `operational_envelopes`, `audit_events`, and `outbox_events` counts do not
  increase;
- the `(request_source, request_id)` count remains one.

If the original request body is unavailable, operators can still verify the
unique idempotency index at the database layer, but that is weaker evidence than
an end-to-end replay.

## 8. Outbox Restore Posture

Unpublished outbox rows remain dispatchable after restore because the dispatcher
claims rows where `published_at IS NULL`. Published rows are skipped by that
claim query and should not be replayed by a normal dispatcher restart.

Before enabling the dispatcher on a restored environment:

- record unpublished and published counts;
- confirm whether downstream consumers can tolerate replay;
- confirm consumer-side idempotency;
- start only the intended dispatcher deployment;
- monitor backlog depth and oldest unpublished age.

MIDAS uses at-least-once outbox delivery. A crash after broker publish but
before marking the row published can still duplicate delivery, so consumer
idempotency remains required.

## 9. Drill Result Template

Record each drill with:

| Check | Result | Evidence |
|---|---|---|
| Source database seeded through `/v1/evaluate` |  |  |
| `pg_dump` completed |  |  |
| Fresh restore database created |  |  |
| `pg_restore` completed |  |  |
| Envelope count present |  |  |
| Audit event count present |  |  |
| Integrity endpoint passed |  |  |
| Idempotency replay returned original envelope |  |  |
| Counts unchanged after replay |  |  |
| Outbox published/unpublished posture recorded |  |  |

## 10. Limitations

This drill closes only local or staging backup/restore evidence for the scoped
database. It does not prove:

- production RPO or RTO;
- Postgres HA or failover;
- point-in-time recovery under write load;
- multi-replica application behaviour;
- dispatcher HA/replay behaviour against a real broker;
- zero-downtime migration safety;
- Bank Tier 1 inline readiness.

