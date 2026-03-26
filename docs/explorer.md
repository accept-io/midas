# MIDAS Explorer

Explorer is a browser-based developer sandbox embedded in MIDAS. It lets you submit evaluation requests, inspect governed outcomes, and view the resulting decision envelope — without writing curl commands.

Explorer runs on an **isolated in-memory store** seeded with demo data. It is completely separate from the live backend. Requests sent through Explorer never touch the configured store.

Explorer is enabled by default in memory mode and can be enabled in Postgres mode via config. It is a **developer tool only — do not expose it in production environments.**

---

## Accessing Explorer

When enabled, Explorer is available at:

```
http://localhost:8080/explorer
```

Open it in a browser. Demo scenarios are pre-loaded and ready to run immediately — no configuration required.

---

## Sandbox vs Live

Explorer operates on a fully isolated runtime. This table shows the separation:

| | Explorer sandbox | Live MIDAS |
|--|-----------------|------------|
| Evaluate | `POST /explorer` | `POST /v1/evaluate` |
| Retrieve envelope | `GET /explorer/envelopes/{id}` | `GET /v1/envelopes/{id}` |
| Store | Isolated in-memory | Configured backend (memory or Postgres) |
| Demo data | Always seeded | Only if `dev.seed_demo_data: true` |
| Auth scope | Same as live (see below) | Same |

Envelopes created via `POST /explorer` exist only in the Explorer sandbox store. They cannot be retrieved via `GET /v1/envelopes/{id}` — use `GET /explorer/envelopes/{id}` instead. The Envelope Inspector in the UI does this automatically.

---

## Explorer endpoints

### `GET /explorer`

Serves the Explorer single-page UI.

### `POST /explorer`

Submits an evaluation request to the Explorer's isolated in-memory orchestrator. Request and response shape are identical to `POST /v1/evaluate`.

**Auth:** same middleware as `/v1/evaluate` — requires `operator` or `admin` role when `auth.mode = required`. In open mode, no token is needed.

### `GET /explorer/envelopes/{id}`

Retrieves an envelope from the Explorer sandbox store by ID. Returns the same JSON shape as `GET /v1/envelopes/{id}`.

**Auth:** same middleware as `/v1/envelopes/{id}` — requires authentication when `auth.mode = required`.

### `GET /explorer/config`

Returns runtime metadata used by the Explorer UI. Unauthenticated.

```json
{
  "running": true,
  "authMode": "open",
  "policyMode": "noop",
  "store": "memory",
  "explorerStore": "memory",
  "demoSeeded": true
}
```

---

## Demo data

Explorer seeds a set of demo surfaces, agents, profiles, and grants unconditionally on startup, independent of `dev.seed_demo_data`. This means Explorer scenarios are always ready, even when the main backend is Postgres and has no demo data.

The pre-loaded scenarios in the UI cover common outcomes:

- **Accept** — confidence and consequence within authority limits
- **Escalate** — confidence below threshold
- **Escalate** — consequence exceeds limit
- **Reject** — unknown surface or agent
- **Request clarification** — required context key missing

Select a scenario from the dropdown. The form populates automatically. Click **Submit** to run it.

---

## Auth behavior

Explorer follows the server's configured auth mode — it does not have a separate auth setting.

**`auth.mode = open` (memory mode default):** No token required. Submit freely.

**`auth.mode = required`:** A bearer token with `operator` or `admin` role is required to call `POST /explorer` or `GET /explorer/envelopes/{id}`. The Explorer UI includes a token input field. Enter your token and click **Apply Token** — the UI validates it and stores it for the session. Token is stored in `sessionStorage` only (cleared when the browser tab closes).

If a token is required but missing or invalid, the response panel shows a clear authentication error. The envelope inspector will similarly surface a clear error if the token is missing when attempting to load envelope details.

---

## Envelope Inspector

After a successful evaluation, the result panel shows an **Inspect envelope** button. Clicking it fetches the full envelope from `GET /explorer/envelopes/{id}` and displays:

- A summary row: state, outcome, reason code, evaluated-at timestamp
- The full envelope JSON

The inspector uses the same bearer token as the evaluation request. If auth is required and no valid token is set, the inspector shows a clear error.

The envelope JSON can be copied to clipboard using the **Copy** button.

---

## Curl output

The UI generates equivalent curl commands for both Explorer and the live endpoint:

```bash
# Explorer sandbox (isolated in-memory store)
curl -X POST http://localhost:8080/explorer \
  -H "Content-Type: application/json" \
  -d '{"surface_id": "...", ...}'

# Production endpoint (uses configured store)
curl -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{"surface_id": "...", ...}'
```

When a token is active, `-H "Authorization: Bearer <token>"` is included automatically.

---

## Configuration

Explorer is enabled via `midas.yaml`:

```yaml
server:
  explorer_enabled: true
```

Or via environment variable:

```bash
export MIDAS_EXPLORER_ENABLED=true
```

Explorer is disabled by default in Postgres mode. Enable it explicitly if needed for development — but do not expose it in production.
