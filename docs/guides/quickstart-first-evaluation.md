# Quickstart: from `midas init quickstart` to your first evaluation

This guide takes a fresh Postgres-backed MIDAS install and walks it
through to a successful `/v1/evaluate` call. It is the canonical
end-to-end walkthrough for the structural quickstart bundle.

`midas init quickstart` only applies the **structural skeleton**:
BusinessServices, Capabilities, BusinessServiceCapability links,
Processes, and Surfaces. To evaluate a decision you also need an Agent,
a Profile, and a Grant — those you author yourself through the standard
control-plane apply path. This guide shows how, end to end.

## What this guide does

- Applies the quickstart bundle.
- Approves one of the Surfaces it creates.
- Applies a small bundle that adds an Agent, a Profile, and a Grant.
- Approves the Profile.
- Calls `POST /v1/evaluate` and gets a successful outcome.

The walkthrough uses [`surf-v2-credit-assess`](#why-this-surface) from
the bundle. The same shape works for any of the bundle's Surfaces if
you substitute the Surface ID, Process ID, Profile ID, Grant ID, and
owner strings appropriately.

## Prerequisites

Before starting, you need:

- **Postgres backend.** `midas init quickstart` rejects the memory
  backend (memory state is per-process and would not survive the
  command's exit). Set `MIDAS_STORE_BACKEND=postgres` and provide a
  reachable `DATABASE_URL`.
- **MIDAS server running** against that same Postgres. The schema is
  applied automatically the first time the store is opened.
- **`midas init quickstart` already applied** (covered in step 1
  below). Re-running is refused via preflight, so subsequent runs are
  safe but no-op.
- **An authenticated principal able to call control-plane and
  approval endpoints.** The simplest path is the bootstrap admin user
  (created on first run with credentials `admin / admin`, password
  change forced on first login). The bootstrap admin holds the
  `platform.admin` role, which bypasses the Surface owner-match check
  during approval. If you are using a non-admin principal you must
  hold `governance.approver` and your principal ID must match the
  Surface's `business_owner` or `technical_owner` — see
  [Approve a quickstart Surface](#3-approve-a-quickstart-surface).
- **A policy posture that permits the example.** MIDAS uses the
  `NoOpPolicyEvaluator` by default, which permits the policy step. If
  you have configured a real policy evaluator, make sure the example
  `spec.policy.reference` resolves to a policy that permits the sample
  request.

The examples below use `Authorization: Bearer $MIDAS_TOKEN` style
headers consistent with the rest of MIDAS's API documentation. If you
are running with `MIDAS_AUTH_MODE=open` (development only) you can omit
the `Authorization` header on `/v1/*` calls.

## 1. Apply the quickstart bundle

```bash
go run ./cmd/midas init quickstart
```

Successful output names every created Surface and points at the
approval endpoint. The bundle creates:

- 2 BusinessServices
- 4 Capabilities
- 5 BusinessServiceCapability links
- 4 Processes
- 6 Surfaces (in `review` status)

The Surfaces are in `review` because the apply path always persists
new Surfaces in review status. Approving a Surface is the next step.

## 2. Pick a Surface

This walkthrough uses **`surf-v2-credit-assess`**. Set it as a shell
variable so the rest of the commands stay readable:

```bash
export SURFACE_ID=surf-v2-credit-assess
```

### Why this Surface

Of the six Surfaces in the bundle, `surf-v2-credit-assess` is a good
one to walk through first:

- It's `category: financial`, `risk_tier: high`, which exercises the
  monetary `consequence_threshold` shape on Profile.
- Its owning Process is `proc-credit-assessment`, which belongs to
  `bs-consumer-lending`.
- It has no `required_context` keys, so the evaluate request body
  stays minimal.

If you'd rather walk through a different Surface, substitute one of:

| Surface ID | Process | Domain |
|---|---|---|
| `surf-v2-id-verify` | `proc-consumer-onboarding` | consumer-lending |
| `surf-v2-consumer-fraud` | `proc-consumer-onboarding` | consumer-lending |
| `surf-v2-credit-assess` | `proc-credit-assessment` | consumer-lending |
| `surf-v2-merchant-risk` | `proc-merchant-risk-screen` | merchant-services |
| `surf-v2-merchant-payment` | `proc-merchant-payment-auth` | merchant-services |
| `surf-v2-merchant-hv-pay` | `proc-merchant-payment-auth` | merchant-services |

The walkthrough's Profile, Grant, and evaluate-call IDs reference
`surf-v2-credit-assess` directly. If you choose a different Surface,
change the Profile ID, Grant ID, Profile `surface_id`, evaluate
`surface_id`, and evaluate `process_id` to match your chosen row.

## 3. Approve a quickstart Surface

```bash
curl -s -X POST \
  -H "Authorization: Bearer $MIDAS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "cli:init-quickstart",
    "approver_id":   "user:admin",
    "approver_name": "Admin"
  }' \
  "http://localhost:8080/v1/controlplane/surfaces/$SURFACE_ID/approve" | jq .
```

Expected response:

```json
{
  "surface_id":  "surf-v2-credit-assess",
  "status":      "active",
  "approved_by": "user:admin"
}
```

If you are not authenticated as `platform.admin`, the approver
principal must hold `governance.approver` and `approver_id` must equal
the Surface's `business_owner` (`consumer-lending-team`) or
`technical_owner` (`midas`). See `docs/control-plane.md` for the full
approval semantics.

You only need to approve the Surface you intend to evaluate against
for this walkthrough. The other quickstart Surfaces stay in `review`
until you approve them too.

## 4. Author Agent, Profile, and Grant

The quickstart bundle does **not** create authority artefacts.
Author your own Agent, Profile, and Grant through the standard apply
path. Save the following as `authority.yaml`:

```yaml
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-quickstart-demo
  name: Quickstart Demo Agent
spec:
  type: automation
  status: active
---
apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: profile-quickstart-credit-assess
  name: Quickstart Credit-Assess Profile
spec:
  surface_id: surf-v2-credit-assess
  authority:
    decision_confidence_threshold: 0.85
    consequence_threshold:
      type: monetary
      amount: 5000
      currency: GBP
  policy:
    reference: noop://quickstart-credit-assess
    fail_mode: closed
  lifecycle:
    status: active
    effective_from: "2026-01-01T00:00:00Z"
    version: 1
---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-quickstart-credit-assess
spec:
  agent_id:       agent-quickstart-demo
  profile_id:     profile-quickstart-credit-assess
  granted_by:     user:admin
  granted_at:     "2026-01-01T00:00:00Z"
  effective_from: "2026-01-01T00:00:00Z"
  status:         active
```

Notes:

- `spec.type: automation` avoids the `llm_agent`-only requirement to
  set `spec.runtime.model` and `spec.runtime.provider`.
- `spec.surface_id` on the Profile must match the Surface ID you
  approved in step 3.
- `spec.policy.reference` is required by the control-plane Profile
  validator. `noop://quickstart-credit-assess` is an example reference
  string; the default `NoOpPolicyEvaluator` permits it, while a real
  policy evaluator may interpret references differently.
- `spec.lifecycle.status: active` is validated, but the apply path
  still persists the Profile in `review` regardless of this value.
  You will approve the Profile in step 6.
- `spec.granted_at` and `spec.effective_from` on Grant are required
  RFC3339 timestamps. `granted_at` must be before or equal to
  `effective_from`.

If you picked a different Surface in step 2, change the Profile ID,
the Grant ID, the `surface_id` reference, the policy reference, and
the evaluate request's `surface_id` / `process_id` accordingly.

## 5. Apply the authority bundle

```bash
curl -s -X POST \
  -H "Authorization: Bearer $MIDAS_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @authority.yaml \
  http://localhost:8080/v1/controlplane/apply | jq .
```

Expected response (abbreviated):

```json
{
  "results": [
    {"kind": "Agent",   "id": "agent-quickstart-demo",            "status": "created"},
    {"kind": "Profile", "id": "profile-quickstart-credit-assess", "status": "created"},
    {"kind": "Grant",   "id": "grant-quickstart-credit-assess",   "status": "created"}
  ]
}
```

The Agent and Grant are created with the status from their YAML
(`active`). The Profile is persisted as `review` regardless of the
`lifecycle.status` value in the document — this is the apply path's
normal behaviour for Profiles, not a quickstart-specific carve-out.

## 6. Approve the Profile

```bash
curl -s -X POST \
  -H "Authorization: Bearer $MIDAS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "user:admin"
  }' \
  http://localhost:8080/v1/controlplane/profiles/profile-quickstart-credit-assess/approve | jq .
```

Expected response:

```json
{
  "profile_id":  "profile-quickstart-credit-assess",
  "version":     1,
  "status":      "active",
  "approved_by": "user:admin"
}
```

`version: 1` is the first persisted version assigned by the apply
planner; it also matches the informational `lifecycle.version`
declared in the YAML. Profile approval has a simpler policy than
Surface approval — it checks only the lifecycle transition (`review`
→ `active`), so no owner-match is required.

## 7. Call `/v1/evaluate`

```bash
curl -s -X POST \
  -H "Authorization: Bearer $MIDAS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-v2-credit-assess",
    "process_id":     "proc-credit-assessment",
    "agent_id":       "agent-quickstart-demo",
    "confidence":     0.91,
    "consequence": {
      "type":     "monetary",
      "amount":   2500,
      "currency": "GBP"
    },
    "request_id":     "req-quickstart-001",
    "request_source": "quickstart-walkthrough"
  }' \
  http://localhost:8080/v1/evaluate | jq .
```

The `confidence` (0.91) is above the Profile's
`decision_confidence_threshold` (0.85). The `consequence.amount`
(2500 GBP) is below the Profile's `consequence_threshold.amount`
(5000 GBP). The `process_id` matches the approved Surface's owning
Process, which keeps the request valid when structural enforcement is
enabled. With a clean Surface/Profile/Grant chain, the evaluation
should produce a non-reject outcome and persist a governance envelope.

The exact response fields depend on the policy evaluator wired in your
deployment. The minimum guarantee is that the request reaches the
authority chain (no `SURFACE_INACTIVE`, no `AGENT_NOT_FOUND`, no
`NO_ACTIVE_GRANT`) and an envelope is recorded. To retrieve the
envelope:

```bash
curl -s \
  -H "Authorization: Bearer $MIDAS_TOKEN" \
  "http://localhost:8080/v1/decisions/request/req-quickstart-001?source=quickstart-walkthrough" \
  | jq .
```

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Evaluate returns `reject` with `SURFACE_INACTIVE` | The Surface is still in `review` status | Run step 3: approve the Surface |
| Evaluate returns `reject` with `PROFILE_NOT_FOUND` | The Profile is in `review` status, or no Profile is bound to that Surface | Run step 6: approve the Profile (or check `spec.surface_id` matches the Surface ID) |
| Evaluate returns `reject` with `NO_ACTIVE_GRANT` | No Grant exists for this Agent, or the Grant is not yet effective | Check the Grant was applied (step 5) and that `effective_from` is in the past |
| Evaluate returns `reject` with `GRANT_PROFILE_SURFACE_MISMATCH` | The Agent's Grant points at a Profile bound to a different Surface | Verify Profile's `spec.surface_id` matches the request's `surface_id` and that the Grant references that Profile |
| Evaluate returns `reject` with `AGENT_NOT_FOUND` | The Agent ID in the request doesn't match a persisted Agent | Check `agent_id` in the request matches the Agent applied in step 5 |
| Evaluate returns `400` with `process_id is required` | Structural enforcement is enabled and the request omitted `process_id` | Include the Process ID for the Surface; for this walkthrough use `proc-credit-assessment` |
| Evaluate returns `400` with an explicit structure validation error | The request's `process_id` does not match the Surface's configured Process | Check the Surface/Process table in step 2 and update the request |
| Surface approval returns `403 forbidden` / `approval forbidden` | Approver lacks `platform.admin`, or lacks `governance.approver`, or `approver_id` doesn't match the Surface's owners | Authenticate as `platform.admin`, or supply an `approver_id` equal to the Surface's `business_owner` (`consumer-lending-team`) or `technical_owner` (`midas`) |
| Apply returns validation errors on the authority bundle | A field is missing or has an invalid enum value | Re-check the YAML against the shapes in step 4; common mistakes: missing `spec.type` on Agent, missing `spec.policy.reference` on Profile, missing `spec.granted_at` or `spec.effective_from` on Grant |
| `midas init quickstart` rejects with `store.backend=memory is not supported` | The CLI is configured against the memory backend | Set `MIDAS_STORE_BACKEND=postgres` and a valid `DATABASE_URL`, then re-run |
| `midas init quickstart` reports "bundle already applied" | A previous run already applied the bundle | This is expected; the command refuses re-runs to avoid creating duplicate pending-review Surface and Profile versions. Use the apply path to evolve the platform from here |

## Where to go next

- [docs/control-plane.md](../control-plane.md) — full control-plane bundle
  format and apply semantics.
- [docs/examples/control-plane.md](../examples/control-plane.md) —
  self-contained example bundles for accepted, escalated, and
  request-clarification outcomes.
- [docs/api/http-api.md](../api/http-api.md) — full HTTP API reference,
  including evaluate response shape and approval endpoints.
- [docs/architecture/architecture.md](../architecture/architecture.md) —
  the structural metamodel `Capability ↔ BusinessService → Process →
  Surface` and how it maps to evaluation.
- [docs/adr/0002-service-led-structural-metamodel.md](../adr/0002-service-led-structural-metamodel.md) —
  the structural metamodel decision.
- [docs/adr/0001-envelope-structural-denormalisation.md](../adr/0001-envelope-structural-denormalisation.md) —
  how the envelope captures the structural chain at evaluation time.
