# Performance — reference inline-evaluation benchmarks

This document describes the reference benchmarks for the inline `/v1/evaluate` path. They exist to give operators and contributors a reproducible way to measure latency, throughput, and tail behaviour against a real Postgres backend before making any tier-readiness claim.

Two benchmarks live in the repo:

| Benchmark | Package | Measures | Excludes |
|---|---|---|---|
| `BenchmarkInlineEvaluate_Postgres` | `internal/decision` | core orchestrator + Postgres transaction path | HTTP, auth, middleware, JSON encode/decode |
| `BenchmarkHTTPInlineEvaluate_Postgres` | `internal/httpapi` | full `/v1/evaluate` API path | external network, ingress, TLS, real deployment topology |

Neither benchmark proves bank-Tier-1 readiness on its own. Both measure relative behaviour on the local hardware they run on. See "Important caveats" at the end of this document.

## `BenchmarkInlineEvaluate_Postgres` — direct orchestrator path

### What it measures

`BenchmarkInlineEvaluate_Postgres` exercises `decision.Orchestrator.Evaluate` directly against a Postgres-backed store. It is **not** a full HTTP path measurement — it does not include `net/http` parsing, JSON decode, the `requireAuth`/`requireRole` chain, the D27d safety middleware, or the D27c metrics recorder overhead. It does include:

- the SHA-256 raw-payload hash
- the (request_source, request_id) idempotency lookup
- surface / process / business-service / capability / agent / profile / grant resolution
- the audit hash chain (one event per lifecycle step)
- the envelope create + update writes
- the outbox event queueing
- a single `BeginTx` → `Commit` per evaluation

In other words: every database operation that a real `/v1/evaluate` would perform, in the same single-transaction shape, but without the HTTP frame.

### What it does NOT measure

- HTTP request parsing, JSON decoding, response encoding.
- Auth / RBAC middleware cost.
- Safety middleware (panic recovery, per-handler timeout) overhead.
- TLS termination (MIDAS does not do TLS itself).
- Network latency between client and server.
- Postgres failover behaviour.
- Outbox dispatcher throughput (the dispatcher is a separate goroutine).

For a measurement that includes the HTTP frame, see `BenchmarkHTTPInlineEvaluate_Postgres` below.

## Required environment

- A reachable Postgres 17 instance with the MIDAS schema applied.
- `DATABASE_URL` exported with the connection string.
- Go 1.26+ (or run inside the repo's `golang:1.26-alpine` Docker harness).

The repository's standard test harness already provides this. The fastest path is the same one the test suite uses:

```bash
docker compose up -d postgres
# Reset and apply the schema as test.sh does.
```

## How to run

### Direct (Postgres reachable from host)

```bash
DATABASE_URL="postgresql://midas:midas@localhost:5432/midas?sslmode=disable" \
  go test -run '^$' -bench BenchmarkInlineEvaluate_Postgres \
    -benchtime=5s -benchmem ./internal/decision/...
```

### Via the repo's Docker harness

```bash
docker compose up -d postgres
docker run --rm --network midas_default \
  -v "$(pwd):/app" -w /app \
  -e DATABASE_URL="postgresql://midas:midas@postgres:5432/midas?sslmode=disable" \
  golang:1.26-alpine \
  sh -c "go test -run '^\$' -bench BenchmarkInlineEvaluate_Postgres -benchtime=5s -benchmem ./internal/decision/..."
```

Set `-benchtime=` higher (e.g. `30s`) for a more stable percentile estimate. The default `1s` is too short for tail latency.

## What the benchmark reports

For every concurrency × pool sub-benchmark the standard `go test -bench` line is supplemented with these custom metrics, emitted via `b.ReportMetric`:

| Metric | Meaning |
|---|---|
| `p50_us` | Median per-evaluation latency in microseconds. |
| `p95_us` | 95th-percentile per-evaluation latency in microseconds. |
| `p99_us` | 99th-percentile per-evaluation latency in microseconds. |
| `max_us` | Worst observed evaluation latency in the sample. |
| `ops/sec` | Sustained throughput (b.N / wall-clock elapsed). |
| `errors` | Count of failed evaluations. The benchmark `b.Fatalf`s if non-zero on the happy path. |

A typical line looks like:

```
BenchmarkInlineEvaluate_Postgres/pool=default(25/5)/concurrency=8-12   1234   3812456 ns/op   2150 p50_us   5840 p95_us   8210 p99_us   12450 max_us   2098 ops/sec   0 errors
```

(Numbers are illustrative.)

## Sub-benchmark matrix

The benchmark sweeps a small Cartesian:

- **Concurrency**: 1, 4, 8, 16. Includes a serial baseline so "how much does concurrency cost p99?" is directly readable.
- **Pool profile**: default (`max_open_conns=25`, `max_idle_conns=5`) and constrained (`max_open_conns=5`, `max_idle_conns=2`). The constrained profile lets you see whether pool sizing is in your critical path before you raise it on real traffic.

Eight sub-benchmarks total. On a typical developer laptop with a containerised Postgres, all eight run in well under a minute at `-benchtime=5s`.

## Interpreting p50 / p95 / p99

- **p50 (median)** dominates user-facing perceived latency on the success path.
- **p95** is where most production SLO budgets live. It is sensitive to GC pauses, connection-pool contention, and Postgres planner variability.
- **p99** is the tail. For governance evaluations it is the latency a small fraction of callers see; for high-criticality service use, p99 is what matters most.
- **max** is one outlier. Noisy and not actionable on its own. Useful only when it diverges from p99 by more than ~5×, which usually points at GC, network blip, or a pool starvation event.

The benchmark records each iteration's wall-clock duration into a slice, sorts it, and picks via nearest-rank. There is no interpolation; for moderate `b.N` (say, > 1000) the difference is negligible.

## How pool configuration affects results

D27b added explicit `database/sql` pool controls. The benchmark's `pool=constrained(5/2)` profile typically shows:

- p50 nearly unchanged
- p95 climbing visibly above the default profile
- p99 climbing more steeply once concurrency exceeds `max_open_conns`

If `pool=constrained` p99 is more than ~3× the default-pool p99 at the same concurrency, the pool is the bottleneck. Scale `max_open_conns` to `replicas × max_open_conns ≤ Postgres max_connections − headroom` and re-run.

(See "Important caveats" below for guidance that applies to both benchmarks.)

## `BenchmarkHTTPInlineEvaluate_Postgres` — full HTTP path

### What it measures

`BenchmarkHTTPInlineEvaluate_Postgres` exercises `POST /v1/evaluate` end-to-end through `Server.ServeHTTP` against a Postgres-backed orchestrator. It includes everything `BenchmarkInlineEvaluate_Postgres` includes, **plus**:

- `net/http` request handling (route dispatch, header parsing).
- Request body read with `maxBytes` enforcement.
- Strict JSON decode (`DisallowUnknownFields`) and request validation.
- The `requireAuth` middleware, with a real static-token authenticator.
- The `requireRole` middleware, gating `/v1/evaluate` on `RolePlatformOperator`.
- The D27d safety middleware (panic recovery + per-handler timeout) wrapped around the mux.
- The handler dispatch in `handleEvaluateWith`.
- JSON response encoding.

The test uses Go's `httptest.NewRecorder` to drive `Server.ServeHTTP` directly — no real socket. That keeps the benchmark deterministic and fast while still exercising every byte of MIDAS-owned code on the request path. It does **not** include the kernel TCP stack, TLS termination, or any reverse-proxy overhead. Those are deliberately out of scope: they are bound by the deployment topology, not by MIDAS.

### What it does NOT measure

- Real network round-trip latency.
- Ingress / load-balancer overhead.
- TLS handshake cost.
- Real client-side connection pooling (`net/http` server-side handler accepts a single in-memory request per iteration).
- Sustained load against a multi-replica deployment.
- The D27c metrics recorder — left at the default no-op for this benchmark to keep the focus on the inline path. Enabling it adds a small constant cost per evaluation that operators should measure separately on production-shaped infrastructure.

### How to run

#### Direct (Postgres reachable from host)

```bash
DATABASE_URL="postgresql://midas:midas@localhost:5432/midas?sslmode=disable" \
  go test -run '^$' -bench BenchmarkHTTPInlineEvaluate_Postgres \
    -benchtime=5s -benchmem ./internal/httpapi/...
```

#### Via the repo's Docker harness

```bash
docker compose up -d postgres
docker run --rm --network midas_default \
  -v "$(pwd):/app" -w /app \
  -e DATABASE_URL="postgresql://midas:midas@postgres:5432/midas?sslmode=disable" \
  golang:1.26-alpine \
  sh -c "go test -run '^\$' -bench BenchmarkHTTPInlineEvaluate_Postgres -benchtime=5s -benchmem ./internal/httpapi/..."
```

### Reported metrics

Same metric inventory as the direct benchmark (`p50_us`, `p95_us`, `p99_us`, `max_us`, `ops/sec`, `errors`), measured around the full `srv.ServeHTTP(rec, req)` call.

The benchmark `b.Fatalf`s on the first non-`200` response so an auth misconfiguration, a seed failure, or any production regression that turns valid evaluations into failed ones is caught immediately.

### Sub-benchmark matrix

Mirrors the direct benchmark exactly:

- **Concurrency**: 1, 4, 8, 16.
- **Pool profile**: `default(25/5)`, `constrained(5/2)`.

Eight sub-benchmarks total, runnable in well under a minute at `-benchtime=5s`.

### Comparing direct and HTTP results

Run both benchmarks back-to-back at the same concurrency × pool point:

```bash
go test -run '^$' -bench BenchmarkInlineEvaluate_Postgres -benchtime=5s ./internal/decision/...
go test -run '^$' -bench BenchmarkHTTPInlineEvaluate_Postgres -benchtime=5s ./internal/httpapi/...
```

The HTTP benchmark's `p50` minus the direct benchmark's `p50` is the per-request overhead of the full HTTP frame: parsing, decode, auth, role check, safety middleware, encode. On a typical developer laptop this delta is in the low-hundreds-of-microseconds range and is dominated by JSON encode/decode plus the safety middleware's goroutine + buffer pattern. The delta should not depend on concurrency in any obvious way; if it does, that is a finding worth investigating.

`p99` deltas are noisier than `p50` deltas: the safety middleware's buffering writer involves an extra goroutine per request, which means the GC interaction differs. Compare medians first.

## Important caveats

> **These are local numbers, not production guarantees.** Latency on a developer laptop with a containerised Postgres on the same machine is fundamentally different from latency in a deployed environment with network round-trips, real disk I/O, real concurrent workloads, and real Postgres tuning.

Use the benchmarks to:

- regression-test latency between MIDAS releases on the **same** hardware,
- compare the relative effect of pool configuration changes,
- compare the cost of HTTP-frame work vs. core evaluation,
- catch obviously broken changes before they ship.

Do **not** use them to:

- claim p99 numbers in customer-facing materials,
- size a production deployment without re-running on production-shaped infrastructure,
- make tier-readiness claims on their own.

Capture benchmark results on at least one production-shaped staging environment before promoting MIDAS to Tier 1 or Tier 2.

## Changing the matrix

Concurrency levels and pool profiles are defined as package-level slices:

- direct (`internal/decision/orchestrator_postgres_bench_test.go`): `benchConcurrencyLevels`, `benchPoolProfiles`
- HTTP (`internal/httpapi/evaluate_bench_test.go`): `httpBenchConcurrencyLevels`, `httpBenchPoolProfiles`

Both pairs are kept in lock-step so direct-vs-HTTP comparisons are apples-to-apples. Adjust them locally for one-off measurement; commit changes to the matrix only when broadening the published reference is intentional.
