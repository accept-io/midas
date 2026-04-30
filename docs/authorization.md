# Authorization

MIDAS uses a **scoped-permission** authorization model for control-plane write operations. Each control-plane write endpoint is gated on a single permission string of the form `resource:action` (for example `surface:approve`, `controlplane:apply`, `grant:revoke`). Roles are re-expressed as named bundles of permissions.

This document describes what exists today. For design rationale and the rejected alternatives, see [docs/design/040-finer-grained-write-authz.md](design/040-finer-grained-write-authz.md).

---

## Scope

This model governs **control-plane write operations only**:

- `POST /v1/controlplane/apply`, `/plan`, `/promote`, `/cleanup`
- `POST /v1/controlplane/surfaces/{id}/{approve,deprecate}`
- `POST /v1/controlplane/profiles/{id}/{approve,deprecate}`
- `POST /v1/controlplane/grants/{id}/{suspend,revoke,reinstate}`

The following are **outside** this model and continue to use role-based gates:

- Read-path introspection (`GET /v1/...`) — role gate `platform.viewer`+
- Data-plane evaluation (`POST /v1/evaluate`) — role gate `platform.operator`+
- Envelope escalation review (`POST /v1/reviews`) — role gate `governance.reviewer` or `platform.admin`
- Explorer sandbox (`POST /explorer`, `POST /explorer/simulate`) — role gate `platform.operator`+
- Authentication endpoints (`POST /auth/login`, `POST /auth/logout`, `POST /auth/change-password`) — session only

---

## Resource × action matrix

The twenty control-plane write permissions. A `✓` means the permission is in that role's bundle.

| Permission | platform.admin | governance.approver | platform.operator | platform.viewer | governance.reviewer |
|---|:-:|:-:|:-:|:-:|:-:|
| `controlplane:apply` | ✓ | | | | |
| `controlplane:plan` | ✓ | | | | |
| `controlplane:promote` | ✓ | | | | |
| `controlplane:cleanup` | ✓ | | | | |
| `capability:write` | ✓ | | | | |
| `process:write` | ✓ | | | | |
| `businessservice:write` | ✓ | | | | |
| `surface:write` | ✓ | | | | |
| `profile:write` | ✓ | | | | |
| `agent:write` | ✓ | | | | |
| `grant:write` | ✓ | | | | |
| `processcapability:write` | ✓ | | | | |
| `processbusinessservice:write` | ✓ | | | | |
| `surface:approve` | ✓ | ✓ | | | |
| `surface:deprecate` | ✓ | | | | |
| `profile:approve` | ✓ | ✓ | | | |
| `profile:deprecate` | ✓ | | | | |
| `grant:suspend` | ✓ | | | | |
| `grant:revoke` | ✓ | | | | |
| `grant:reinstate` | ✓ | | | | |

The canonical source of this matrix lives in [`internal/authz/authz.go`](../internal/authz/authz.go). A test in [`internal/authz/authz_test.go`](../internal/authz/authz_test.go) enforces the matrix count and prevents neighbouring-cell leaks.

### Per-Kind permissions for `POST /v1/controlplane/apply`

The `*:write` permissions correspond to document Kinds in a control-plane apply bundle:

| Document Kind | Permission required per document |
|---|---|
| `Capability` | `capability:write` |
| `Process` | `process:write` |
| `BusinessService` | `businessservice:write` |
| `BusinessServiceCapability` | `businessservicecapability:write` |
| `Surface` | `surface:write` |
| `Profile` | `profile:write` |
| `Agent` | `agent:write` |
| `Grant` | `grant:write` |

---

## Two-tier check on `/v1/controlplane/apply`

The apply endpoint enforces authorization at **two layers**:

1. **Middleware gate — coarse.** `requirePermission(controlplane:apply)` runs before the request body is read. A caller lacking `controlplane:apply` receives `403 forbidden` with `required_permission: "controlplane:apply"` and never reaches the bundle parser.

2. **Planner per-document check — fine-grained.** Once the bundle is parsed, the apply planner checks the caller's per-Kind write permission for each document. A caller holding `controlplane:apply` but missing (for example) `agent:write` will see the Agent document marked invalid in the apply result — with a validation error quoting `agent:write` — while other documents plan normally. No document from the bundle is persisted when any document is invalid, matching existing `ApplyPlan` semantics.

The two tiers exist so that an operator can scope a caller to a subset of Kinds without giving them blanket admin. For example, a catalogue-editor role composition could grant `controlplane:apply`, `controlplane:plan`, and only `businessservice:write` + `businessservicecapability:write`.

The plan endpoint (`POST /v1/controlplane/plan`) applies the same per-document check. Under the default five roles this is observationally a no-op (only `platform.admin` holds `controlplane:plan`, and `platform.admin` holds every `*:write`), but it keeps the two-tier model uniform for future custom role compositions.

---

## Default roles and their bundles

### `platform.admin` — 20 permissions (full control-plane write access)

All `controlplane:*`, all `*:write`, all lifecycle actions. This is the load-bearing bundle for the seeded bootstrap `admin` user and for every static-token or OIDC mapping that grants admin.

**Implementation note.** `platform.admin` is not a wildcard and the enforcement path does not special-case it. The bundle contains each permission literally. Removing a permission from the admin bundle removes the capability for every admin caller.

### `governance.approver` — 2 permissions

Exactly `{surface:approve, profile:approve}`. Preserves the maker–checker split: an approver can approve a surface or profile version authored by someone else, but cannot author versions, deprecate, apply, plan, promote, cleanup, or touch grants.

### `platform.operator` — 0 permissions

No control-plane write permissions. The role's existing role-gated access to `POST /v1/evaluate` (data-plane) and `POST /explorer` (sandbox) is unchanged and out of this model's scope.

### `platform.viewer` — 0 permissions

Read-only. The role's existing role-gated access to `GET /v1/...` introspection endpoints is unchanged and out of this model's scope.

### `governance.reviewer` — 0 permissions

No control-plane write permissions. The role's existing role-gated access to `POST /v1/reviews` (envelope escalation review) is unchanged and out of this model's scope.

### Custom role bundles

Not implemented. The role→permission map lives in [`internal/authz/authz.go`](../internal/authz/authz.go) as a Go-level constant. Custom bundles are a deferred enhancement.

---

## Administrative separation patterns enabled by the model

The scoped-permission primitive makes these separations expressible, even though only the two bundles in §Default roles are shipped today.

- **Approver-only.** `{surface:approve, profile:approve}`. Delivered as `governance.approver`.
- **Admin.** All 20 permissions. Delivered as `platform.admin`.
- **Service-catalogue editor** *(not shipped)*. `{controlplane:apply, controlplane:plan, businessservice:write, processbusinessservice:write}`. Can write and link business services, cannot touch surfaces, agents, profiles, or grants.
- **Process designer** *(not shipped)*. `{controlplane:apply, controlplane:plan, capability:write, process:write, processcapability:write, processbusinessservice:write, controlplane:promote, controlplane:cleanup}`. Can model structural entities and formalise inferred ones.
- **Authority steward** *(not shipped)*. `{controlplane:apply, controlplane:plan, profile:write, agent:write, grant:write, profile:approve, profile:deprecate, grant:suspend, grant:revoke, grant:reinstate}`. Owns authority without touching structural modelling.
- **Control-plane planner** *(not shipped)*. `{controlplane:plan}`. Read-of-preview access: can validate a bundle but cannot apply it.

---

## Denial response shape

### 403 forbidden

Every control-plane-write 403 carries the required permission additively:

```json
{
  "error": "forbidden",
  "required_permission": "<permission>"
}
```

`required_permission` names the specific permission the caller lacked. The body does **not** leak which permissions the caller holds, does **not** echo principal fields, and does **not** include the full role list.

### 401 unauthorized

Unchanged:

```json
{
  "error": "unauthorized"
}
```

No `required_permission` is emitted on 401s. The two statuses have distinct semantics: 401 means "no valid credentials", 403 means "authenticated but lacking the required permission".

### Bundle per-document denials

When the per-document check denies a document within a bundle, the denial appears in the existing `ApplyPlan.Entries` and `ApplyResult.ValidationErrors` channels. The top-level HTTP status is `200 OK`; the plan result carries the denial:

```json
{
  "results": [],
  "validation_errors": [
    {
      "kind": "Agent",
      "id": "agent-x",
      "message": "caller lacks permission \"agent:write\" required to write documents of kind \"Agent\""
    }
  ]
}
```

No new rejection path is introduced; per-document denials reuse the existing invalid-entry channel.

---

## Role normalization and deprecated aliases

Role resolution operates on the principal's **normalised** roles. Normalisation happens at every principal-construction path:

- Local IAM sessions — [`internal/localiam/service.go`](../internal/localiam/service.go) `UserToPrincipal`
- Static token authenticator — [`cmd/midas/main.go`](../cmd/midas/main.go) `buildAuthenticator`
- OIDC role mapping — [`internal/oidc/rolemapping.go`](../internal/oidc/rolemapping.go) `MapExternalRoles`

Deprecated aliases (`admin`, `operator`, `approver`, `reviewer`) are canonicalised by `identity.NormalizeRoles` to their `platform.*` / `governance.*` equivalents before the permission lookup runs. This means the bootstrap admin user (whose DB-stored role is the deprecated alias `admin`) resolves to the full `platform.admin` permission bundle at request time, without any authorization-layer change.

Role matching is **case-insensitive and whitespace-trimmed**, matching `identity.Principal.HasRole` semantics. A token carrying `"PLATFORM.ADMIN"` resolves identically to `"platform.admin"`.

---

## `AuthModeOpen` pass-through

When `MIDAS_AUTH_MODE=open` (the default for memory/dev deployments), `requirePermission` short-circuits and forwards the request without inspecting the principal. This matches the long-standing behaviour of `requireRole` in open mode and is required for Docker-mode and `go run ./cmd/midas` out-of-the-box operability.

For bundle apply in open mode, the planner receives a permissive authorizer that allows every Kind.

---

## Code map

- **Package:** [`internal/authz/`](../internal/authz/) — permission constants, role bundles, `HasPermission`, `KindToWritePermission`. No I/O, no global state beyond constants.
- **Middleware:** `requirePermission` in [`internal/httpapi/auth.go`](../internal/httpapi/auth.go) — the coarse gate replacement for `requireRole` on write endpoints.
- **Per-document check:** [`internal/controlplane/apply/authz.go`](../internal/controlplane/apply/authz.go) + the switch in [`internal/controlplane/apply/service.go`](../internal/controlplane/apply/service.go) — the fine-grained layer.
- **HTTP wiring:** [`internal/httpapi/server.go`](../internal/httpapi/server.go) `applyCtxWithKindAuthorizer` — constructs the per-document authorizer from the request principal.

---

## Related documentation

- [docs/design/040-finer-grained-write-authz.md](design/040-finer-grained-write-authz.md) — design rationale, options analysis, test strategy
- [docs/control-plane.md](control-plane.md) — control-plane endpoints and bundle format
- [docs/guides/authentication.md](guides/authentication.md) — authentication modes and token shape
- [docs/api/http-api.md](api/http-api.md) — full API reference including denial status codes
