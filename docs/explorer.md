# MIDAS Explorer

The MIDAS Explorer is a browser-based developer sandbox built into the MIDAS server. It lets you submit governance evaluation requests, inspect the reasoning behind each outcome, trace the authority chain that produced the decision, and view the full governance envelope — without writing curl commands or reading raw JSON.

Explorer is designed for:

- **Developers** integrating MIDAS into an autonomous system who want to understand how the evaluation pipeline works
- **Architects and reviewers** evaluating whether MIDAS fits their governance requirements
- **CNCF reviewers and evaluators** who want to exercise the system hands-on

Explorer runs on an **isolated in-memory store** seeded with demo data. It is completely separate from the live backend and has no effect on any configured production store.

---

## What MIDAS Is Demonstrating

MIDAS is a **runtime governance engine**. Its job is to answer a specific question at decision time:

> Does this autonomous actor have the authority to execute this decision, right now, given its confidence level and the consequence of being wrong?

This is different from what most engineers reach for first:

| | Rules engine | Policy engine | MIDAS |
|---|---|---|---|
| Primary concern | Data transformation logic | Access control | Execution authority |
| Asks | "What should happen?" | "Is this allowed?" | "Does this actor have authority to act?" |
| Output | Result value | Permit / deny | Accept / Escalate / Reject / Clarify |
| Governs | Business rules | Permissions | Autonomous decision-making |
| Audit trail | Typically absent | Typically absent | Tamper-evident governance envelope |

MIDAS operates on a specific model:

**Decision Surface** — a named governance boundary for a type of autonomous decision (e.g., payment approvals, content moderation). Surfaces have a domain, reversibility class, and failure mode. A request cannot proceed unless the surface is active and the agent has an authority chain against it.

**Agent** — a registered autonomous actor: an AI model, an automated system, or a robotic process. Agents must be registered before they can evaluate decisions.

**Authority Profile** — a set of operational constraints that define what an agent is permitted to do on a surface: minimum confidence threshold, maximum consequence limit, required context keys, escalation behaviour. A profile is attached to a surface.

**Authority Grant** — an explicit linkage that authorises a specific agent to act under a specific profile. Grants are point-in-time approvals with lifecycle management.

**Confidence threshold** — the minimum certainty score the agent must report for autonomous execution to be permitted. Scores below the threshold route to human review (escalation), not rejection.

**Consequence limit** — the maximum impact of the decision (monetary, risk rating, etc.) that the agent is authorised to carry without human oversight. Exceeding the limit triggers escalation.

**Required context keys** — fields the agent must supply in the request for the authority profile to be satisfied. Missing keys cause the system to request clarification rather than attempt evaluation.

**Governance envelope** — the immutable record produced for every evaluation. It captures the full authority chain that was resolved, the threshold comparison, the outcome with reason code, the submitted context, and a hash-chained integrity anchor. This is the audit trail.

---

## Getting Started

### Running MIDAS

The simplest way to start MIDAS with the Explorer available:

```bash
./midas serve
```

By default, MIDAS starts in memory mode with the Explorer enabled. No configuration is required.

To use Docker:

```bash
docker compose up
```

See the [Getting Started guide](../docs/getting-started.md) for full instructions.

### Opening Explorer

Navigate to:

```
http://localhost:8080/explorer
```

### What you see on first load

The Explorer initialises in **Evaluate mode** and immediately runs a demo request (an unknown-surface scenario) to confirm the runtime is reachable. Within a second you will see:

- A green sandbox banner confirming the isolated in-memory store is active
- The outcome card populated with a `Reject` result (`SURFACE_NOT_FOUND`)
- The authority chain panel and explanation panel (empty for this result — no authority was resolved)
- The response JSON and equivalent curl commands

The Runtime section at the bottom of the right panel shows the auth mode, policy mode, and store backend.

### Authentication

Explorer follows the server's configured auth mode.

**`auth.mode = open` (default in memory mode):** No token is required. The Explorer runs without authentication.

**`auth.mode = required`:** A bearer token with `operator` or `admin` role is required. The Authentication section in the left panel accepts a token. Paste your token and click **Apply Token** or press Enter. The token is validated immediately and stored in `sessionStorage` for the duration of the tab. It is not persisted beyond the browser tab.

**Local IAM (`localiam` mode):** A login overlay appears on load, blurring the Explorer interface until you sign in. Enter your MIDAS username and password. If your account requires a password change, a second overlay prompts you before you can proceed.

**OIDC (`oidc` mode):** The login overlay shows an organisation sign-in button that redirects to your configured identity provider. After successful authentication you are returned to the Explorer.

In all authenticated modes, the interface is visually blurred and non-interactive while the login overlay is active.

---

## Explorer Layout

### Left panel

The left panel is the request side of the Explorer.

**Quick Start** summarises what MIDAS governs and the difference between the two execution modes.

**Demo Resources** shows the six resources seeded into the Explorer's isolated store. These are the surfaces, profiles, agents, and grants that power the pre-built scenarios. Each card shows the resource type, name, ID, and key detail (thresholds, required keys, link targets). See [Resource Reference Panel](#resource-reference-panel) for details.

**Authentication** (shown when auth mode is `required`) provides the bearer token input.

**Authority Scenarios** is the scenario picker. Scenarios are grouped by category and filterable by text search. Selecting a scenario populates the Request Builder and shows a microcopy panel explaining what the scenario demonstrates and what outcome to expect.

**Request Builder** lets you compose a request manually in Form mode (individual fields) or JSON mode (raw editor). Switching between modes keeps the values in sync. The **Submit** button sends the request. **Reset** clears all fields. **Copy as curl** generates an equivalent curl command using the current form values.

### Right panel

The right panel is the result side.

**Outcome card** displays the primary outcome (`Accept`, `Escalate`, `Reject`, or `Request Clarification`) in large text, colour-coded by result type. The card also shows the reason code, envelope ID (in Evaluate mode), policy mode, and HTTP status.

**Explanation box** provides a plain-English sentence describing why the outcome occurred, written using the actual threshold values from the evaluation where available (e.g., "Confidence 0.30 is below the required threshold of 0.85").

**Threshold Evaluation panel** shows the numeric breakdown: the confidence provided vs the required threshold (with a pass/fail indicator), the consequence provided vs the limit (with a pass/fail indicator), and the outcome driver — the field that ultimately determined the result.

**Authority Chain panel** shows the resolved authority path as a flow: `Surface → Profile → Grant → Agent`, with version numbers for surfaces and profiles.

**Response JSON** shows the raw API response with the HTTP status badge and a copy button.

**Equivalent curl** provides ready-to-run curl commands for both the Explorer sandbox endpoint and the live production endpoint. In Simulate mode, only the sandbox curl is shown.

**Decision Record** (Evaluate mode only) provides a "View decision record" button that fetches the full governance envelope and displays a summary grid (state, outcome, reason code, evaluated timestamp) and the complete envelope JSON.

**Runtime** shows live server metadata: auth mode, policy mode, and store backend.

---

## Using Scenarios

The Authority Scenarios panel is the fastest way to explore MIDAS. Scenarios are pre-built evaluation payloads that exercise distinct paths through the governance engine, grouped into four categories.

Select a scenario to populate the request form and see a description of what the scenario demonstrates. Then click **Submit** to run it.

### Category: Thresholds

These scenarios exercise the confidence threshold configured on the authority profile.

| Scenario | Confidence | Expected |
|---|---|---|
| Within authority | 0.95 | Accept |
| Below confidence threshold | 0.30 | Escalate |
| At threshold boundary | 0.85 | Accept |

The demo authority profile requires confidence ≥ 0.85. The "Below confidence threshold" scenario sends 0.30, which falls below the minimum. MIDAS escalates — it does not reject. The agent may have made the right call; a human reviewer decides. The "At threshold boundary" scenario confirms that the comparison is inclusive: exactly 0.85 passes.

### Category: Consequence

These scenarios exercise the consequence limit.

| Scenario | Consequence | Expected |
|---|---|---|
| Low value approval | £50 | Accept |
| Near consequence limit | £999 | Accept |
| Above consequence limit | £2,500 | Escalate |

The demo authority profile sets a consequence limit of £1,000 GBP. Sending £999 passes (the comparison is ≤, so the limit is inclusive). Sending £2,500 escalates. MIDAS does not reject based on consequence alone — it routes to human review when the impact exceeds delegated authority.

### Category: Context

These scenarios exercise context validation and required context key enforcement.

| Scenario | Surface | Context | Expected |
|---|---|---|---|
| Request with context | surf-payments-approval | customer\_id + channel + reason | Accept |
| Missing required context | surf-context-validation | (none) | Clarify |
| With required context | surf-context-validation | customer\_id | Accept |

The `surf-context-validation` surface uses a profile (`profile-context-strict`) that declares `customer_id` as a required context key. Submitting a request against this surface without providing `customer_id` in the context object causes MIDAS to return `Request Clarification` with reason code `INSUFFICIENT_CONTEXT`. The agent must resend the request with the required fields present. Providing `customer_id` in a second request against the same surface proceeds normally and accepts.

The `surf-payments-approval` surface has no required context keys — context fields are recorded and audited but not required for authority resolution.

### Category: Authority Chain

These scenarios test what happens when the authority chain cannot be established.

| Scenario | What is broken | Expected |
|---|---|---|
| Unknown surface | Surface ID not found | Reject |
| Unknown agent | Agent ID not registered | Reject |

Authority chain failures produce `Reject` (not `Escalate`). There is no ambiguity to escalate — the system has no record of the surface or agent and cannot proceed. These are hard stops.

---

## Evaluate vs Simulate

The mode selector bar at the top of the Explorer switches between Evaluate and Simulate.

### Evaluate mode

Evaluate mode calls `POST /explorer` against the Explorer's isolated in-memory orchestrator. Every evaluation:

- Creates a **governance envelope** with a unique ID
- Writes an **audit record** into the sandbox store
- Produces a **tamper-evident hash chain** (verifiable without application secrets)
- Returns an `envelope_id` in the response

After evaluation, the Explorer automatically fetches the full envelope in the background and populates the Threshold Evaluation panel and Authority Chain panel.

The Decision Record section shows the resulting envelope. Click "View decision record" to load the full governance record.

### Simulate mode

Simulate mode calls `POST /explorer/simulate`. Nothing is written. No envelope is created, no audit record is produced, and no outbox messages are queued. The response omits `envelope_id`.

Simulate mode is safe to use as many times as needed. Use it to:

- Test how changing a confidence score affects the outcome
- Check whether a different consequence amount crosses the escalation threshold
- Experiment with inputs without producing permanent records

In Simulate mode, the Threshold Evaluation panel and Authority Chain panel are not shown (they require envelope data, which simulate does not produce).

### Simulate from this

After a successful Evaluate call that produces an outcome, a **Simulate from this** button appears. Clicking it:

1. Stores the current evaluated request and result as a baseline in `sessionStorage`
2. Switches to Simulate mode

Subsequent simulations show a **Comparison vs Baseline** panel that highlights what changed between the baseline and the simulated result — outcome, reason code, and input fields. This makes it easy to find the boundary of an agent's authority by iterating on a single field.

---

## Understanding the Result

### Outcome

MIDAS returns one of four outcomes:

| Outcome | Meaning |
|---|---|
| **Accept** | The agent has authority to execute. Confidence and consequence are within limits; all required context is present; authority chain resolves. |
| **Escalate** | The agent does not have autonomous authority for this specific request — confidence is too low or consequence is too high — but the decision is not refused. It is routed to human review. |
| **Reject** | The authority chain cannot be established. The surface or agent is unknown, no active grant exists, or the profile is inactive. These are hard failures, not escalation candidates. |
| **Request Clarification** | The authority profile requires context keys that were not supplied. The agent must resend the request with the missing fields. |

The outcome card uses colour coding: green for Accept, amber for Escalate, red for Reject, purple for Request Clarification.

### Reason Code

The reason code is a typed constant that identifies exactly which evaluation step produced the outcome. It is stable across versions and suitable for programmatic handling.

| Reason Code | Outcome | Cause |
|---|---|---|
| `WITHIN_AUTHORITY` | Accept | Both confidence and consequence within profile limits |
| `CONFIDENCE_BELOW_THRESHOLD` | Escalate | Agent confidence below profile minimum |
| `CONSEQUENCE_EXCEEDS_LIMIT` | Escalate | Consequence above profile maximum |
| `INSUFFICIENT_CONTEXT` | Clarify | Required context key(s) missing from request |
| `SURFACE_NOT_FOUND` | Reject | No active decision surface with the given ID |
| `AGENT_NOT_FOUND` | Reject | No registered agent with the given ID |
| `NO_ACTIVE_GRANT` | Reject | Agent has no active grant for this surface |
| `PROFILE_NOT_FOUND` | Reject | Profile referenced by grant does not exist |
| `SURFACE_INACTIVE` | Reject | Surface exists but is not in an active state |
| `GRANT_PROFILE_SURFACE_MISMATCH` | Reject | Grant's profile is not linked to the requested surface |
| `POLICY_DENY` | Reject | A policy rule explicitly denied the request |
| `POLICY_ERROR` | Escalate | Policy evaluation encountered an error |

### Threshold Evaluation Panel

After a successful Evaluate call, the Threshold Evaluation panel shows the numeric comparison that determined the outcome. It is populated from the governance envelope fetched automatically after evaluation.

The panel shows:

**Confidence:** the value submitted by the agent vs the minimum required by the authority profile, with a green tick (pass) or red cross (fail).

**Consequence:** the consequence provided vs the maximum allowed by the profile, with a pass/fail indicator. This row is omitted if no consequence was submitted.

**Outcome driver:** the field that drove the final outcome — the factor that was determinative.

Example for the "Below confidence threshold" scenario:

```
Confidence    0.30 ≥ 0.85  ✗
Consequence   £100 ≤ £1,000  ✓
Outcome driver  confidence threshold
```

The confidence check failed; the consequence check passed; the outcome driver was confidence. MIDAS escalated because the agent's certainty score was below the delegated minimum, not because the action itself was problematic.

The explanation box above the panel renders a plain-English sentence using the actual values: "Confidence 0.30 is below the required threshold of 0.85. Human review is required before this decision can execute."

### Authority Chain

The Authority Chain panel shows the full governance path that was resolved to reach the outcome. It is displayed as a left-to-right flow:

```
Surface  →  Profile  →  Grant  →  Agent
```

Each node shows the resource ID and, for surfaces and profiles, the version number. This makes the chain auditable and reproducible — you can see exactly which version of which profile was active at the time of evaluation.

- **Surface** — the decision boundary the request was evaluated against
- **Profile** — the authority constraints that applied (thresholds, context requirements, escalation mode)
- **Grant** — the explicit authorisation that linked this agent to this profile
- **Agent** — the autonomous actor that submitted the request

If the authority chain could not be resolved (for example, `SURFACE_NOT_FOUND`), the panel is not shown.

---

## Decision Record

After an Evaluate call, the Decision Record section appears at the bottom of the right panel. Click **View decision record** to load the full governance envelope from the Explorer's isolated store.

The envelope is presented in two parts:

**Summary grid** shows key fields at a glance:

| Field | Description |
|---|---|
| state | Current lifecycle state (`OUTCOME_RECORDED`, `ESCALATED`, `CLOSED`, etc.) |
| outcome | The evaluated outcome (`accept`, `escalate`, `reject`, `request_clarification`) |
| reason | The reason code that drove the outcome |
| evaluated | ISO 8601 timestamp of evaluation |

**Envelope JSON** shows the complete governance record, copyable to clipboard. This includes the full authority resolution, threshold evaluation detail, submitted context, and integrity hash references.

The envelope is the tamper-evident audit record MIDAS produces for every governed decision. It does not require application secrets to verify — the hash chain is self-contained.

---

## Resource Reference Panel

The Demo Resources panel in the left sidebar shows the six demo resources seeded into the Explorer's isolated in-memory store.

| Resource | ID | Key Detail |
|---|---|---|
| Surface | `surf-payments-approval` | domain: payments |
| Profile | `profile-payments-std` | confidence ≥ 0.85, £1,000 consequence limit |
| Agent | `agent-payments-bot` | type: AI |
| Grant | `grant-payments-bot-std` | links agent to standard profile |
| Surface | `surf-context-validation` | domain: compliance |
| Profile | `profile-context-strict` | requires: `customer_id` |

These resources directly back the Authority Scenarios:

- **Thresholds and Consequence** scenarios use `surf-payments-approval` and `profile-payments-std`. The thresholds shown on the profile card (confidence ≥ 0.85, consequence ≤ £1,000) are the exact values against which the threshold scenarios are evaluated.
- **Context** scenarios that target `surf-context-validation` use `profile-context-strict`. The "requires: customer\_id" detail on the profile card explains why the "Missing required context" scenario returns `INSUFFICIENT_CONTEXT`.
- **Authority Chain** scenarios use `surf-payments-approval` and `agent-payments-bot`. Unknown-surface and unknown-agent scenarios intentionally use IDs that are not in this list, which is why they reject.

The grant `grant-payments-bot-std` is the explicit authorisation that connects `agent-payments-bot` to `profile-payments-std`. Without this grant, the agent would have no authority on `surf-payments-approval` even if both the surface and the agent are registered.

---

## Scenario Walkthroughs

### 1. Autonomous execution within authority

**Input:**

```json
{
  "surface_id": "surf-payments-approval",
  "agent_id": "agent-payments-bot",
  "confidence": 0.95,
  "consequence": { "type": "monetary", "amount": 100, "currency": "GBP" }
}
```

**Evaluation:** MIDAS resolves the authority chain — surface is active, agent has an active grant, grant links to an active profile. Confidence 0.95 ≥ 0.85 (pass). Consequence £100 ≤ £1,000 (pass). All context requirements are satisfied (none required).

**Outcome:** `Accept` / `WITHIN_AUTHORITY`. The agent is authorised to execute.

**Authority chain:** `surf-payments-approval v1 → profile-payments-std v1 → grant-payments-bot-std → agent-payments-bot`

**Threshold Evaluation panel shows:**
- Confidence: 0.95 ≥ 0.85 ✓
- Consequence: £100 ≤ £1,000 ✓
- Outcome driver: within authority

---

### 2. Confidence below threshold — escalation required

**Input:**

```json
{
  "surface_id": "surf-payments-approval",
  "agent_id": "agent-payments-bot",
  "confidence": 0.30,
  "consequence": { "type": "monetary", "amount": 100, "currency": "GBP" }
}
```

**Evaluation:** Authority chain resolves successfully. Confidence 0.30 < 0.85 (fail). Consequence £100 ≤ £1,000 (pass). The confidence check is the deciding factor.

**Outcome:** `Escalate` / `CONFIDENCE_BELOW_THRESHOLD`. The action is not refused — it is queued for human review. A low confidence score means the agent is uncertain; a human must decide whether to proceed.

**Threshold Evaluation panel shows:**
- Confidence: 0.30 ≥ 0.85 ✗
- Consequence: £100 ≤ £1,000 ✓
- Outcome driver: confidence threshold

---

### 3. Consequence exceeds authority limit

**Input:**

```json
{
  "surface_id": "surf-payments-approval",
  "agent_id": "agent-payments-bot",
  "confidence": 0.95,
  "consequence": { "type": "monetary", "amount": 2500, "currency": "GBP" }
}
```

**Evaluation:** Authority chain resolves. Confidence passes. Consequence £2,500 > £1,000 (fail). The consequence check is the deciding factor.

**Outcome:** `Escalate` / `CONSEQUENCE_EXCEEDS_LIMIT`. The impact of the decision exceeds the agent's delegated limit. Regardless of confidence, a human must approve decisions of this size.

**Threshold Evaluation panel shows:**
- Confidence: 0.95 ≥ 0.85 ✓
- Consequence: £2,500 ≤ £1,000 ✗
- Outcome driver: consequence limit

---

### 4. Missing required context

**Input:**

```json
{
  "surface_id": "surf-context-validation",
  "agent_id": "agent-payments-bot",
  "confidence": 0.95,
  "consequence": { "type": "monetary", "amount": 100, "currency": "GBP" }
}
```

**Evaluation:** Authority chain resolves via `profile-context-strict`. That profile requires `customer_id` to be present in the request context. No context was supplied. MIDAS cannot evaluate without the required fields.

**Outcome:** `Request Clarification` / `INSUFFICIENT_CONTEXT`. This is not an escalation or rejection — the agent must resend the request with the missing field. No envelope is created for clarification requests.

Run the "With required context" scenario next to see the same surface accept when `customer_id` is present.

---

### 5. Unknown surface — hard reject

**Input:**

```json
{
  "surface_id": "surf-unknown-xyz",
  "agent_id": "agent-payments-bot",
  "confidence": 0.95
}
```

**Evaluation:** MIDAS looks up `surf-unknown-xyz` in the store. No surface with this ID exists.

**Outcome:** `Reject` / `SURFACE_NOT_FOUND`. Authority chain lookup stops immediately. There is no profile to evaluate against, so escalation would be meaningless. The request is rejected and an envelope is created recording the authority failure.

**Authority chain panel:** not shown (no authority was resolved).

---

## Troubleshooting

**Symptom:** 401 Unauthorized in the outcome card
**Cause:** No bearer token set, or the token has expired
**Solution:** In `required` auth mode, paste a valid token in the Authentication section and click Apply Token. In `localiam` mode, the login overlay will reappear automatically.

**Symptom:** 403 Forbidden in the outcome card
**Cause:** Token is valid but the account does not have `operator` or `admin` role
**Solution:** Use a token for an account with the correct role. Check role assignment in the platform user management API.

**Symptom:** "Request JSON is not valid" validation error
**Cause:** The JSON editor contains a syntax error
**Solution:** Switch to Form mode to build the request using individual fields, then switch back to JSON to inspect the generated payload.

**Symptom:** Submit button does nothing / result shows a validation error
**Cause:** Required fields are empty — `surface_id`, `agent_id`, and `confidence` are mandatory
**Solution:** Ensure all three required fields are populated. Use a scenario to pre-fill a valid request.

**Symptom:** `INSUFFICIENT_CONTEXT` when you expect `Accept`
**Cause:** You are submitting against `surf-context-validation`, which requires `customer_id` in the context
**Solution:** Add `{"customer_id": "your-id"}` to the Context field, or use the "With required context" scenario.

**Symptom:** `SURFACE_NOT_FOUND` when submitting a custom request
**Cause:** The surface ID does not exist in the Explorer's demo store
**Solution:** Use one of the seeded surface IDs: `surf-payments-approval` or `surf-context-validation`. Custom surfaces can only be used in the Explorer if demo.go is extended and the server is restarted.

**Symptom:** Threshold Evaluation and Authority Chain panels do not appear
**Cause:** These panels are only populated in Evaluate mode (they require a governance envelope). In Simulate mode they are intentionally hidden
**Solution:** Switch to Evaluate mode and resubmit. If using Evaluate mode and the panels are still absent, check the browser console for a failed request to `/explorer/envelopes/{id}`.

**Symptom:** "View decision record" shows an auth error
**Cause:** The bearer token is missing or invalid when the envelope inspector attempts to fetch the envelope
**Solution:** Apply a valid token before inspecting the envelope.

---

## Sandbox vs Live Endpoints

Explorer operates on a fully isolated runtime. This table shows the separation:

| | Explorer sandbox | Live MIDAS |
|---|---|---|
| Evaluate | `POST /explorer` | `POST /v1/evaluate` |
| Retrieve envelope | `GET /explorer/envelopes/{id}` | `GET /v1/envelopes/{id}` |
| Store | Isolated in-memory | Configured backend (memory or Postgres) |
| Demo data | Always seeded | Only if `dev.seed_demo_data: true` |

Envelopes created via `POST /explorer` exist only in the Explorer sandbox store. They cannot be retrieved via `GET /v1/envelopes/{id}`.

The Equivalent curl section in the Explorer generates commands for both endpoints. The sandbox curl targets `/explorer`; the production curl targets `/v1/evaluate`. Both commands use the same request body.

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

Explorer is enabled by default in memory mode. In Postgres mode, it must be enabled explicitly.

**Do not expose the Explorer in production.** It is a developer sandbox. Enabling it in a production environment where the Explorer endpoint is externally reachable is an unnecessary attack surface. Use `server.headless: true` in production deployments to disable all browser-facing surfaces.

---

## Key Takeaways

- MIDAS governs execution authority, not decision logic. It answers whether an autonomous actor is permitted to act — not what the actor should do.
- Authority chains are explicit, versioned, and fully traceable. Every governed decision records which surface, profile, grant, and agent were resolved at evaluation time.
- Outcomes are explainable through threshold evaluation. Every result includes the numeric comparison — confidence provided vs required, consequence provided vs allowed — not just the binary outcome.
- Governance envelopes are tamper-evident audit records that do not require application secrets to verify. They are the compliance artefact for every governed decision.
- Escalation and rejection are distinct. Low confidence or high consequence routes to human review; authority chain failures are hard rejections. The system does not conflate uncertainty with refusal.
