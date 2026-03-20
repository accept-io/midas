# Operator Journey

This document walks through the complete lifecycle of a governed decision surface — from initial definition through to active evaluation and eventual deprecation. It is written for operators who configure and maintain the authority model.

---

## Overview

The lifecycle of authority governance in MIDAS follows this path:

```
Define resources → Apply bundle → Approve surface → Evaluate → (Deprecate)
```

Authority flows in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

Resources must be created before they can be referenced. Profiles reference surfaces. Grants reference profiles and agents.

---

## Step 1: Define your resources

Create a YAML bundle file (e.g. `governance-bundle.yaml`). A bundle may contain one or more documents separated by `---`. Supported kinds are `Surface`, `Agent`, `Profile`, and `Grant`.

```yaml
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-refund-auto-approval
  name: Refund Auto-Approval
spec:
  domain: customer-operations
  description: Governs autonomous refund approvals up to operator-defined limits
  decision_type: operational
  reversibility_class: reversible
  failure_mode: closed
  business_owner: Head of Customer Operations
  technical_owner: platform-team
  required_context:
    fields:
      - name: customer_id
        type: string
        required: true
      - name: order_id
        type: string
        required: true
  compliance_frameworks:
    - ISO27001
---
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-refund-service-v2
  name: Refund Automation Service v2
spec:
  type: automation
  runtime:
    model: refund-engine
    version: "2.1.0"
    provider: internal
  status: active
---
apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: prof-refund-standard
  name: Standard Refund Authority
spec:
  surface_id: surf-refund-auto-approval
  authority:
    decision_confidence_threshold: 0.85
    consequence_threshold:
      type: monetary
      amount: 500
      currency: GBP
  input_requirements:
    required_context:
      - customer_id
      - order_id
  policy:
    fail_mode: closed
  lifecycle:
    status: active
    effective_from: "2026-01-01T00:00:00Z"
---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-refund-service-standard
spec:
  agent_id:   agent-refund-service-v2
  profile_id: prof-refund-standard
  granted_by: user-governance-lead
  status: active
```

---

## Step 2: Dry-run with plan

Before writing anything, validate the bundle and preview what would happen:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/plan \
  -H "Content-Type: application/yaml" \
  --data-binary @governance-bundle.yaml | jq .
```

Example response:

```json
{
  "entries": [
    {
      "kind":           "Surface",
      "id":             "surf-refund-auto-approval",
      "action":         "create",
      "document_index": 1,
      "decision_source": "persisted_state"
    },
    {
      "kind":           "Agent",
      "id":             "agent-refund-service-v2",
      "action":         "create",
      "document_index": 2,
      "decision_source": "persisted_state"
    },
    {
      "kind":           "Profile",
      "id":             "prof-refund-standard",
      "action":         "create",
      "document_index": 3,
      "decision_source": "persisted_state"
    },
    {
      "kind":           "Grant",
      "id":             "grant-refund-service-standard",
      "action":         "create",
      "document_index": 4,
      "decision_source": "persisted_state"
    }
  ],
  "would_apply":    true,
  "invalid_count":  0,
  "conflict_count": 0,
  "create_count":   4
}
```

`would_apply: true` means the bundle is valid and will succeed if applied. If any entry has `action: invalid`, the `validation_errors` array on that entry explains what is wrong.

---

## Step 3: Apply the bundle

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Content-Type: application/yaml" \
  --data-binary @governance-bundle.yaml | jq .
```

Example response:

```json
{
  "results": [
    {"kind": "Surface", "id": "surf-refund-auto-approval", "status": "created"},
    {"kind": "Agent",   "id": "agent-refund-service-v2",   "status": "created"},
    {"kind": "Profile", "id": "prof-refund-standard",      "status": "created"},
    {"kind": "Grant",   "id": "grant-refund-service-standard", "status": "created"}
  ]
}
```

**Important:** The surface is created with status `review`. It is not yet eligible for evaluation. This is enforced by the control plane regardless of what `status` field you specify in the YAML — apply always sets new surfaces to `review`.

---

## Step 4: Approve the surface

A surface must be explicitly promoted to `active` before its grants are eligible for evaluation. This enforces a governance checkpoint between definition and operation.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-refund-auto-approval/approve \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "user-team-lead",
    "approver_id":   "user-governance-lead",
    "approver_name": "Governance Lead"
  }' | jq .
```

Response:

```json
{
  "surface_id":  "surf-refund-auto-approval",
  "status":      "active",
  "approved_by": "user-governance-lead"
}
```

The surface is now `active`. If the outbox dispatcher is enabled, a `surface.approved` event is published to the `midas.surfaces` Kafka topic.

---

## Step 5: Verify the authority model

Check the surface:

```bash
curl -s http://localhost:8080/v1/surfaces/surf-refund-auto-approval | jq .
```

Check profiles on the surface:

```bash
curl -s "http://localhost:8080/v1/profiles?surface_id=surf-refund-auto-approval" | jq .
```

Check the agent:

```bash
curl -s http://localhost:8080/v1/agents/agent-refund-service-v2 | jq .
```

Check grants for the agent:

```bash
curl -s "http://localhost:8080/v1/grants?agent_id=agent-refund-service-v2" | jq .
```

---

## Step 6: Evaluate decisions

Your service now calls MIDAS before executing a refund:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-refund-auto-approval",
    "agent_id":       "agent-refund-service-v2",
    "confidence":     0.92,
    "consequence": {
      "type":     "monetary",
      "amount":   250,
      "currency": "GBP"
    },
    "context": {
      "customer_id": "C-10021",
      "order_id":    "ORD-88821"
    },
    "request_id":     "req-refund-00421",
    "request_source": "refund-service"
  }' | jq .
```

`accept` / `WITHIN_AUTHORITY` — proceed with the refund.

Try above the consequence threshold:

```bash
# amount: 750 exceeds the 500 GBP threshold
```

Response: `escalate` / `CONSEQUENCE_EXCEEDS_LIMIT` — route to manual review.

---

## Step 7: Handle escalations

List envelopes awaiting review:

```bash
curl -s http://localhost:8080/v1/escalations | jq .
```

Resolve an escalation:

```bash
curl -s -X POST http://localhost:8080/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "envelope_id": "<envelope_id_from_escalation>",
    "decision":    "approve",
    "reviewer":    "user-ops-manager",
    "notes":       "Exception approved — customer VIP status confirmed"
  }' | jq .
```

The envelope closes. If the dispatcher is enabled, a `decision.review_resolved` event is published.

---

## Step 8: View surface version history

Surfaces are versioned. Every apply that creates or updates a surface creates a new version record.

```bash
curl -s http://localhost:8080/v1/surfaces/surf-refund-auto-approval/versions | jq .
```

Each version entry shows its version number, status, effective date, and approval metadata.

---

## Step 9: Deprecate the surface

When a surface is superseded (e.g. replaced by a higher-limit profile or a redesigned process), deprecate it:

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-refund-auto-approval/deprecate \
  -H "Content-Type: application/json" \
  -d '{
    "deprecated_by": "user-governance-lead",
    "reason":        "Replaced by surf-refund-enhanced with higher limits",
    "successor_id":  "surf-refund-enhanced"
  }' | jq .
```

Response:

```json
{
  "surface_id":           "surf-refund-auto-approval",
  "status":               "deprecated",
  "deprecation_reason":   "Replaced by surf-refund-enhanced with higher limits",
  "successor_surface_id": "surf-refund-enhanced"
}
```

A `surface.deprecated` event is published if the dispatcher is enabled.

**Important:** Deprecation does not revoke existing grants. Agents holding grants against a deprecated surface continue to evaluate successfully. Deprecation is a signal to migrate, not a cut-off. If you need to stop evaluations immediately, the agent's operational state must be changed or the grant revoked directly.

---

## Surface status lifecycle

```
draft → review → active → deprecated → retired
```

- `review` — set automatically on every apply. Not yet eligible for evaluation.
- `active` — promoted via the approve endpoint. Eligible for evaluation.
- `deprecated` — transitioned via the deprecate endpoint. Still evaluates; signals migration expected.
- `retired` — terminal state (direct database operation; no API endpoint in community edition).

There is no `inactive` status.

---

## How to modify resources

The apply endpoint enforces different modification semantics per resource kind. Read this before re-applying any bundle.

### How do I modify a surface?

Re-apply the updated YAML bundle. A new version is created in `review` status. The currently active version continues to serve evaluations. Approve the new version to activate it.

```
Apply bundle (v2 created in review) → Approve v2 → v2 becomes active
```

A conflict is returned if the latest persisted version is already in `review` — resolve that review before applying again.

### How do I modify a profile?

Re-apply the updated YAML bundle. A new version is appended to the profile's lineage. Version 1 → Version 2 → Version 3. The runtime continues using whichever version is `active` until a new version is explicitly activated.

### Can I modify an agent?

Not via apply. Agents are immutable once created. If you need to change an agent's configuration:

1. Register a new agent with a new ID (e.g. `agent-payments-v2`).
2. Create a new grant linking the new agent to the appropriate profile.
3. Revoke the old grant when you are ready to cut over.

### Can I modify a grant?

Not via apply. Grants are immutable once created. To change authority binding:

1. Revoke the existing grant.
2. Create a new grant with the updated agent or profile reference.

### What creates a new version?

| Action | Resource | New version? |
|--------|----------|-------------|
| `POST /v1/controlplane/apply` with new ID | Any | Yes (version 1) |
| `POST /v1/controlplane/apply` with existing ID | Surface | Yes (version N+1) |
| `POST /v1/controlplane/apply` with existing ID | Profile | Yes (version N+1) |
| `POST /v1/controlplane/apply` with existing ID | Agent | No — conflict |
| `POST /v1/controlplane/apply` with existing ID | Grant | No — conflict |

### What stays active?

The active version of a surface or profile is the one with `status = active` and `effective_from <= evaluation_timestamp`. Applying a new version does not change which version is active. The governance workflow (apply → approve) controls that transition.

### What does "latest" mean?

"Latest" means the highest version number, regardless of status. `GET /v1/surfaces/{id}` returns the latest version. During a governance review cycle, the latest version is in `review` while the active version (used for evaluation) may be a lower version number.

### What should I do if I made a mistake?

- **Surface applied with wrong content:** Apply the corrected version. A new version is created in `review`. Approve the corrected version. The mistaken version stays in `review` and is never activated.
- **Profile applied with wrong content:** Apply the corrected version. A new version is created and can be activated independently.
- **Agent registered with wrong ID:** Register the correct agent. Revoke any grants pointing to the incorrect agent.
- **Grant created with wrong agent/profile:** Revoke the grant. Create a new grant with the correct references.

---

## Recovery and rollback

### Why MIDAS uses history-preserving recovery

MIDAS does not support destructive edits or hard rollback. Every version is immutable once persisted. This is a governance property, not a limitation: an immutable audit trail is required for regulated environments.

Recovery in MIDAS means:

- Preserve all history.
- Create corrected newer versions where appropriate.
- Deprecate mistaken active versions where appropriate (pointing a successor to the correct replacement).
- Make successor relationships explicit.
- Expose clear operator-visible state and guidance.

### Latest vs active — the key distinction

| Term | Meaning |
|------|---------|
| **latest** | Highest version number, any status. `GET /v1/surfaces/{id}` returns this. |
| **active** | The version with `status=active` whose effective window covers the evaluation timestamp. The orchestrator uses this version. |

During a governance review cycle these will differ. The latest version may be in `review` while the active version is a lower version number. Both the recovery endpoints and the impact endpoint make this distinction explicit.

### Recovery endpoints

```
GET /v1/surfaces/{id}/recovery
GET /v1/profiles/{id}/recovery
```

Both endpoints are read-only. They return the current state analysis, operator warnings, and a deterministic list of recommended next actions based on what is actually in the store.

### How to fix a bad active surface version

1. **Apply a corrected version.** A new version is created in `review`. The bad version remains active — evaluation still works, using the (bad) active version.

2. **Approve the corrected version.** `POST /v1/controlplane/surfaces/{id}/approve`. The corrected version becomes active. The bad version retains its `active` status but will be superseded.

3. **Deprecate the bad version (optional but recommended).** `POST /v1/controlplane/surfaces/{id}/deprecate` with `successor_id` pointing to the corrected surface. This marks the old version deprecated with an explicit migration path.

Check `GET /v1/surfaces/{id}/recovery` at any point to see the current state and recommended actions.

### How to fix a bad profile version

Profiles have no governance review checkpoint. The apply path sets `status=active` immediately. There is no approval workflow for profiles.

1. **Apply a corrected profile version.** A new version is created and activated immediately.

2. **Verify the new version is effective.** Check `GET /v1/profiles/{id}/recovery`. The `active_version` should reflect the new version.

3. **Check active grants.** If grants reference the old profile ID (grants use the logical profile ID, not a specific version), they will automatically resolve to the new active version at evaluation time.

If the corrected profile has a future `effective_from`, it will not be active yet. The recovery endpoint warns about this and recommends re-applying with a past `effective_from`.

### How to migrate agents to a successor surface

When a surface is deprecated with a successor:

1. `GET /v1/surfaces/{old-id}/recovery` returns the successor ID and the action "inspect successor surface '{new-id}' and plan grant migration".

2. List all grants referencing profiles on the old surface: `GET /v1/grants?profile_id={prof-id}`.

3. For each agent, create a new grant referencing a profile on the successor surface.

4. Revoke the old grants once the new grants are verified.

### What MIDAS does not yet automate

- Automatic rollback of a bad active surface to the previous version (would require history search for the prior active version — this must be done manually by applying and approving the corrected configuration).
- Automatic migration of grants from a deprecated surface to its successor.
- Profile deprecation via the apply path (profile status is always set to `active` on apply; there is no review state for profiles).

---

## Key rules

- **Surface and Profile reapply creates a new version.** Agent and Grant reapply returns conflict.
- Surface `status` in the YAML is validated but always overridden to `review` on apply.
- Profiles reference surfaces by `surface_id`. Grants reference both agents and profiles. If a referenced resource does not exist in the bundle or in the store, the entry is marked `invalid`.
- Version resolution at evaluation time selects the version with `status = active` and `effective_from <= evaluation_timestamp`. If no such version exists, the evaluation returns `SURFACE_INACTIVE` or `PROFILE_NOT_FOUND`.
- `GET /v1/surfaces/{id}` returns the *latest* version (highest version number). The runtime uses the *active* version. These differ during a governance review cycle.
