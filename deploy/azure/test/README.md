# MIDAS Dedicated Azure Test Environment

This directory defines the local planning artefacts for a dedicated MIDAS Azure
Container App test environment. It is separate from the public demo deployment
at `midas.accept.io`.

No Azure resources are created by these files. They are the operator contract
for a later provisioning tranche.

## Purpose

The `midas-test` environment exists for controlled Level 2 and early Level 3
validation. It has two distinct operating shapes:

| Shape | Purpose | Availability posture |
|---|---|---|
| Functional validation environment | Prove configuration, auth, fixture setup, governed evaluation, evidence integrity, outbox row creation, and basic p999 harness access. | May run small and short-lived. Not an availability target. |
| Tier 1 availability-capable test environment | Exercise multi-replica runtime availability, scale behaviour, revision restart behaviour, Postgres HA posture, and higher-confidence p999 evidence. | Sized for availability testing, but still not a Bank Tier 1 inline readiness claim. |

The Phase 1 build should target the Tier 1 availability-capable test shape so
that the same environment can run both functional and availability evidence
without rebuilding the deployment after smoke tests.

This environment supports:

- authenticated `/v1/evaluate` integration tests;
- Postgres-backed governed runtime persistence;
- p999 and sustained-load testing over HTTPS;
- evidence and audit-integrity verification;
- transactional outbox and dispatcher validation;
- metrics, logs, alert, and dashboard validation.

It must not contain production data or seeded demo data, and it must not be used
to claim Bank Tier 1 inline readiness.

## Separation from demo

| Area | Demo environment | Dedicated test environment |
|---|---|---|
| Purpose | Public product demo | Controlled validation |
| Example host | `midas.accept.io` | `midas-test.accept.io` or `test.midas.accept.io` |
| Data | Demo-friendly data allowed | No demo data; explicit test fixtures only |
| Store | Demo-specific | Dedicated Postgres database |
| Auth | Demo posture | `MIDAS_AUTH_MODE=required` |
| Metrics | Demo posture | Public only during a short-lived test window, or restricted/trusted scrape |
| Eventing | Optional | Event Hubs Kafka endpoint for dispatcher validation |
| Lifecycle | Long-lived demo | Disposable or explicitly lifecycle-managed |

## Target architecture

```text
Codex/Claude or test runner
        |
        | HTTPS + temporary bearer token
        v
Azure Container App: midas-test-api
        |
        | private connection / firewall-restricted connection string
        v
Azure Database for PostgreSQL Flexible Server
        |
        | transactional outbox rows
        v
MIDAS dispatcher goroutine (Phase 2)
        |
        | Kafka protocol over TLS
        v
Azure Event Hubs Kafka endpoint (Phase 2)

Container App logs + MIDAS /metrics
        |
        v
Trusted scraper / D40k Prometheus rules / Grafana dashboard
```

## Proposed Azure resources

| Resource | Suggested name | Purpose | Public? |
|---|---|---|---|
| Resource group | `rg-midas-test` | Holds dedicated test resources | No |
| Container Apps environment | Existing environment if suitable, otherwise `cae-midas-test` | Hosts Container App and optional internal services | No direct public endpoint |
| Container App | `midas-test-api` | Runs MIDAS test runtime | HTTPS ingress only |
| Custom hostname | `midas-test.accept.io` or `test.midas.accept.io` | Stable test base URL | Yes, MIDAS only |
| PostgreSQL Flexible Server | `pg-midas-test` | Runtime source of truth, General Purpose or better | No |
| PostgreSQL database | `midas_test` | Dedicated MIDAS test database | No |
| Event Hubs namespace | `evh-midas-test` | Kafka-compatible dispatcher target | No public admin surface |
| Event Hubs / topics | `midas.decisions`, `midas.surfaces`, `midas.profiles`, `midas.grants` | Outbox delivery validation using MIDAS logical topics | Broker auth required |
| Log Analytics workspace | `law-midas-test` | Container App logs | No |
| Managed identity | `id-midas-test-app` | Secret retrieval and future Azure integration | No |

## Hosting posture

Initial Container App posture for the Tier 1 availability-capable test shape:

- Container App name: `midas-test-api`;
- minimum replicas: `3`;
- maximum replicas: `6` initially;
- CPU: `2 vCPU` per replica;
- memory: `4 GiB` per replica;
- workload profile: Dedicated preferred;
- zone redundancy: required where supported;
- external HTTPS ingress for MIDAS only;
- revision mode `single` for the first validation pass, with revision restart
  behaviour tested during Phase 1E;
- `MIDAS_AUTH_MODE=required`;
- no automatic demo seeding;
- Postgres-backed runtime only;
- `/metrics` may be public during the short-lived test window if useful for
  validation; otherwise use restricted or trusted-runner scrape;
- no public Postgres endpoint;
- no public Event Hubs admin endpoint;
- no production or customer data.

Reuse an existing Azure Container Apps environment only if it supports the
required workload profile, zone redundancy, networking model, and resource
headroom for 3 to 6 replicas at `2 vCPU / 4 GiB` each. If it does not, create a
separate test Container Apps environment. The public demo Container App
`midas-api` remains out of scope and must not be modified for this environment.

This sizing makes the environment suitable for availability-capable testing. It
does not, by itself, prove Bank Tier 1 inline readiness.

## Postgres posture

Use Azure Database for PostgreSQL Flexible Server:

- tier: General Purpose or better;
- initial baseline: 4 vCore minimum;
- high availability enabled;
- zone-redundant HA preferred where supported;
- separate database: `midas_test`;
- no shared demo database;
- backup/restore posture documented before availability evidence is claimed.

Initial application pool settings should be:

```text
MIDAS_DATABASE_MAX_OPEN_CONNS=20
MIDAS_DATABASE_MAX_IDLE_CONNS=5
```

These limits are per MIDAS replica. With 3 to 6 replicas, the effective maximum
runtime connection demand is 60 to 120 open connections before headroom,
operator sessions, Postgres maintenance, and any future dispatcher/eventing
connections. Confirm the Postgres `max_connections` budget before load testing.

## Phase 1 sequence

Preserve dependency-first sequencing:

| Phase | Name | Purpose | Exit criteria |
|---|---|---|---|
| 1A | Azure area and dependency confirmation | Confirm resource group, Container Apps environment suitability, image/tag, Postgres server/database, network path, secrets, tokens, metrics exposure, and teardown plan. | No Azure build starts until dependencies and safety checkpoint are confirmed. |
| 1B | Tier 1-sized Container App deployment | Deploy `midas-test-api` with 3 to 6 replica envelope, `2 vCPU / 4 GiB`, Dedicated preferred, zone redundancy where supported, Postgres backend, auth required, demo data disabled, dispatcher disabled. | At least 3 healthy replicas and `/healthz`/`/readyz` green. |
| 1C | Fixture and governed evaluation validation | Apply explicit non-demo fixtures, approve required surface/profile, validate governed `/v1/evaluate`, idempotency, evidence integrity, and outbox row creation. | Functional validation complete with no demo data and no demo DB/token reuse. |
| 1D | Low-rate and moderate-rate p999 evidence | Run low-rate and moderate-rate sustained HTTPS load with p50/p95/p99/p999, errors, pool metrics, transaction metrics, and outbox backlog metrics captured. | Tail latency and error posture understood for this Azure shape. |
| 1E | Availability and failover evidence | Validate replica health, revision restart behaviour, single-replica failure tolerance, scale-out under load, Postgres HA status, metrics continuity, and planned Postgres failover behaviour if safe. | Availability evidence captured without material error spike or undocumented failure mode. |

## Required runtime posture

The test environment must set:

```text
MIDAS_STORE_BACKEND=postgres
MIDAS_AUTH_MODE=required
MIDAS_METRICS_ENABLED=true
MIDAS_DEV_SEED_DEMO_DATA=false
MIDAS_DEV_SEED_DEMO_USER=false
MIDAS_DEV_SEED_SYNTHETIC_DRIFT=false
MIDAS_DISPATCHER_ENABLED=false
MIDAS_DISPATCHER_PUBLISHER=none
```

See [env.example](env.example) for the full variable contract.

## Pre-start safety checkpoint

Before the first revision starts:

- confirm no copied or inherited demo seed values are `true`;
- confirm `MIDAS_DEV_SEED_DEMO_DATA=false`;
- confirm `MIDAS_DEV_SEED_DEMO_USER=false`;
- confirm `MIDAS_DEV_SEED_SYNTHETIC_DRIFT=false`;
- confirm `midas-test-api` does not point at the demo database;
- confirm `midas-test-api` does not reuse demo auth tokens;
- confirm `MIDAS_AUTH_MODE=required`;
- confirm Postgres credentials, auth tokens, and later Event Hubs credentials
  are stored as secrets;
- confirm `midas-api` and `midas.accept.io` remain untouched.

## Endpoint controls

| Endpoint class | Public? | Control | Purpose |
|---|---|---|---|
| `/healthz` | Yes | No secrets; liveness only | Container App/liveness checks |
| `/readyz` | Yes | No secrets; dependency readiness | Load balancer/readiness checks |
| `/v1/evaluate` | Yes | Bearer token, `platform.operator`+ | Controlled governed evaluations |
| `/v1/evidence/*` | Yes | Bearer token, `platform.viewer`+ | Integrity and evidence validation |
| `/v1/controlplane/*` | Conditional | Bearer token, admin/governance roles; test window only | Explicit test fixture setup |
| `/explorer` | Prefer no | Enable only if needed and auth-required | Human investigation during tests |
| `/metrics` | Conditional | May be public during a short-lived synthetic test window, otherwise private scrape/IP allowlist/trusted runner | D40k metrics and dashboard validation |
| Postgres | No | Private networking/firewall and secret connection string | Runtime persistence |
| Event Hubs/Kafka | No public admin | Broker auth/TLS, secrets in Azure | Dispatcher validation |

## Codex/Claude access model

Codex/Claude should receive only:

- base URL: `https://midas-test.accept.io` or `https://test.midas.accept.io`;
- temporary bearer token with `platform.operator`;
- optional separate `platform.viewer` token for read-only evidence checks;
- test-only `request_source`, for example `codex-d41b`;
- fixture IDs from [test-fixture.yaml](test-fixture.yaml);
- explicit load-test rate and duration limits.

Codex/Claude should not receive Azure portal access, database credentials, or
broker credentials unless a later tranche specifically requires that access.
Tokens must be rotated or destroyed after each test window.

## Test fixture strategy

Automatic demo seeding is disabled. Test fixtures are applied explicitly through
the control plane.

[test-fixture.yaml](test-fixture.yaml) defines a minimal chain:

```text
BusinessService -> Capability -> BusinessServiceCapability -> Process -> Surface
Surface -> Profile -> Grant -> Agent
```

Operator setup sequence:

1. Apply the fixture bundle with `POST /v1/controlplane/apply`.
2. Approve `surf-d41b-evaluate`.
3. Approve `profile-d41b-evaluate` version `1`.
4. Use `agent-d41b-evaluator` and `surf-d41b-evaluate` for `/v1/evaluate`.

This creates enough governed state for accept-path evaluation, idempotency
replay, idempotency conflict, evidence integrity checks, and outbox event
creation. It is not demo data.

## Kafka/Event Hubs/dispatcher posture

The selected path is Azure Event Hubs with its Kafka-compatible endpoint.

Why this path:

- Azure-native and easier to operate than a standalone broker for this test
  environment;
- sufficient to validate MIDAS Kafka client compatibility, dispatcher claiming,
  broker acknowledgement, `published_at` marking, backlog drain, and
  at-least-once consumer posture;
- avoids adding a public Kafka/Redpanda endpoint.

Limitation: Event Hubs validates Kafka-compatible client behaviour, not every
Kafka broker semantic. A later tranche can use Redpanda/Kafka when exact broker
behaviour matters.

Dispatcher-on revision variables:

```text
MIDAS_DISPATCHER_ENABLED=true
MIDAS_DISPATCHER_PUBLISHER=kafka
MIDAS_KAFKA_BROKERS=<event-hubs-namespace>.servicebus.windows.net:9093
MIDAS_KAFKA_CLIENT_ID=midas-test
MIDAS_KAFKA_REQUIRED_ACKS=-1
MIDAS_KAFKA_WRITE_TIMEOUT=10s
MIDAS_KAFKA_TLS_ENABLED=true
MIDAS_KAFKA_SASL_MECHANISM=plain
MIDAS_KAFKA_SASL_USERNAME='$ConnectionString'
MIDAS_KAFKA_SASL_PASSWORD=secretref:eventhubs-send-connection-string
```

MIDAS keeps the logical topic from each outbox row. Create Event Hubs with
matching names rather than overriding topics in the app.

Keep dispatcher disabled throughout Phase 1. Event Hubs/Kafka dispatcher
validation is Phase 2 only, after Phase 1 functional, p999, and availability
evidence are clean. The Phase 1 posture still validates transactional outbox row
creation and backlog metrics.

## Observability

Minimum observability:

- Container App logs enabled through Log Analytics;
- MIDAS structured logs retained for the test window;
- `/metrics` scraped from a trusted runner, private network path, or temporary
  public test-window scrape;
- D40k Prometheus rules available to the scraper;
- D40k Grafana dashboard imported where Prometheus data is stored;
- outbox backlog metrics visible:
  `midas_outbox_unpublished_total` and
  `midas_outbox_oldest_unpublished_age_seconds`;
- pool and transaction metrics visible for load-test interpretation.

If Prometheus/Grafana are not deployed in Azure initially, the first validation
pass may scrape `/metrics` from a trusted operator runner and archive the output
with Container App logs.

## Initial validation plan

| Test | Purpose | Required access | Pass criteria |
|---|---|---|---|
| `GET /healthz` | Liveness | Public HTTPS | 200, no secret data |
| `GET /readyz` | DB readiness | Public HTTPS | 200 while Postgres is reachable |
| No-token `/v1/evaluate` | Auth posture | Public HTTPS | 401, not validation/business error |
| Apply fixture | Create non-demo test data | Temporary admin/governance token | Fixture resources created or known conflicts |
| Approve fixture surface/profile | Activate authority chain | Temporary admin/governance token | Surface/profile active |
| Authenticated `/v1/evaluate` | Governed runtime path | `platform.operator` token | 200, `audit_status:"recorded"` |
| Idempotent replay | Retry safety | Same token/request ID/body | Original envelope returned |
| Idempotency conflict | Conflict safety | Same token/request ID, changed body | 409 conflict |
| Evidence integrity | Audit verification | `platform.viewer`+ token | Integrity valid for sampled envelope |
| Low-rate p999 run | HTTPS target evidence | Operator token and metrics | Zero errors; p50/p95/p99/p999 captured |
| Moderate-rate p999 run | Load evidence | Operator token and metrics | Error count, p50/p95/p99/p999, pool metrics, transaction metrics, and outbox backlog captured |
| Outbox row creation | Transactional outbox | Metrics/evidence | Unpublished count changes as expected |
| Metrics scrape | Observability | Trusted scraper | Required MIDAS metrics present |

## Availability validation plan

| Check | Purpose | Pass criteria |
|---|---|---|
| Replica baseline | Confirm availability-capable shape | At least 3 healthy `midas-test-api` replicas running. |
| Revision restart | Validate restart continuity | `/healthz` and `/readyz` remain green through restart, allowing normal rolling-update transients. |
| Single replica failure | Validate replica loss tolerance | Service remains available when one replica is stopped or replaced by the platform. |
| Scale-out under load | Validate scale behaviour | Scale-out to the configured maximum works without a material evaluation error spike. |
| Postgres HA status | Validate database posture | Azure Postgres HA reports healthy before load evidence is captured. |
| Planned Postgres failover | Validate failover behaviour if safe | Behaviour, duration, errors, retry posture, and recovery are captured; skip if unsafe for the test window. |
| Metrics continuity | Validate observability during change | `/metrics` remains available or scrape gaps are bounded and explained during restart/scale activity. |

## Performance evidence plan

Capture the following for both low-rate and moderate-rate p999 runs:

- total requests;
- successful evaluations;
- error count;
- p50, p95, p99, and p999;
- maximum observed latency;
- database pool open, in-use, idle, wait count, and wait duration;
- transaction begin/callback/commit/total latency;
- transaction errors and rollbacks;
- outbox unpublished count;
- outbox oldest unpublished age;
- Container App replica count during the run;
- Postgres HA status before and after the run.

## Safety and lifecycle controls

- no production data;
- no demo data by default;
- test-only fixture IDs and request sources;
- temporary tokens only;
- token rotation/destruction after each test window;
- no public unauthenticated runtime;
- `/metrics` may be public only during the short-lived synthetic test window
  when useful for validation;
- initial load cap: 5 eval/sec smoke, then operator-approved low-rate and
  moderate-rate increases;
- initial duration cap: 5 minutes;
- environment owner and incident contact tagged on resources;
- resource TTL tag, for example `ttl=14d`;
- cost tags, for example `env=midas-test`, `purpose=validation`;
- teardown runbook required before increasing scale;
- availability-capable test posture documented without treating it as a Bank
  Tier 1 readiness claim;
- no Bank Tier 1 readiness claim.

## Readiness impact

This environment is intended to move MIDAS validation beyond local Docker while
remaining separate from the public demo. It can support deployment-specific
Level 2 evidence once its controls and tests pass. Because Phase 1 uses multiple
replicas and an HA Postgres posture, it can also produce early availability and
p999 evidence needed for later Level 3 and highest-criticality readiness
assessment work.

It does not prove active-active behaviour, signed RPO/RTO achievement,
production SLOs, or Bank Tier 1 inline readiness.

## Recommended next tranche

`D41c-azure-test-environment-provisioning`

Provision the dedicated resources described here, wire auth-required runtime,
Postgres, no-demo-data settings, metrics access controls, and base fixture setup
without changing `midas-api` or `midas.accept.io`.
