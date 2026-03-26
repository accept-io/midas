# Policy Evaluation in MIDAS

## Current State

MIDAS uses `NoOpPolicyEvaluator` by default. In this mode:

- All policy checks pass — `Allowed: true` is always returned.
- Profiles that declare a `policy_ref` field will **not** have that policy enforced.
- Evaluation outcomes, reason codes, and audit events are unaffected.
- The policy step in the evaluation flow still runs; it just calls the no-op evaluator.

This is not a misconfiguration — it is the documented default for development and
early-stage deployments. Real OPA/Rego integration is planned behind the existing
`PolicyEvaluator` interface (see [Future Direction](#future-direction) below).

## What Operators Will See

### Startup warning

When `NoOpPolicyEvaluator` is active, MIDAS emits a structured warning at startup:

```json
{
  "level": "WARN",
  "msg": "policy_mode_noop",
  "reason": "no policy evaluator configured; all policy checks will pass",
  "action": "configure a real policy evaluator to enforce policy",
  "policy_mode": "noop",
  "policy_evaluator": "NoOpPolicyEvaluator"
}
```

### Health and readiness responses

`GET /healthz` and `GET /readyz` include policy metadata:

```json
{
  "status": "ok",
  "service": "midas",
  "policy_mode": "noop",
  "policy_evaluator": "NoOpPolicyEvaluator"
}
```

### Evaluate response

`POST /v1/evaluate` responses include `policy_mode` on every evaluation.
When a profile has a `policy_ref` configured but the noop evaluator is active,
`policy_skipped: true` appears as an explicit signal:

```json
{
  "outcome": "accept",
  "reason": "WITHIN_AUTHORITY",
  "envelope_id": "env-abc123",
  "policy_mode": "noop",
  "policy_reference": "rego://payments/limits",
  "policy_skipped": true
}
```

When no `policy_ref` is set on the resolved profile, `policy_reference` and
`policy_skipped` are omitted from the response.

## NoOp Behaviour Summary

| Condition | Outcome |
|-----------|---------|
| Profile has no `policy_ref` | Policy step skipped entirely; no `policy_skipped` flag |
| Profile has `policy_ref`, noop active | Policy step runs but always allows; `policy_skipped: true` |
| Profile has `policy_ref`, real evaluator active | Policy evaluated; result determines outcome |
| Evaluator returns error, `fail_mode: open` | Policy error ignored, evaluation continues |
| Evaluator returns error, `fail_mode: closed` | Outcome: `Escalate`, reason: `POLICY_ERROR` |

**Note:** `fail_mode` on the profile is not enforced in noop mode because the
evaluator never returns an error. A profile configured with `fail_mode: closed`
will not escalate if the noop evaluator is active and returns `Allowed: true`.

## Future Direction

Real OPA/Rego integration is planned behind the existing `PolicyEvaluator` interface
in `internal/policy/policy.go`. Swapping in a real evaluator requires:

1. Implementing `PolicyEvaluator` with OPA embedded or called as a sidecar.
2. Implementing `PolicyModer` to expose the mode string (e.g. `"opa"`).
3. Updating `cmd/midas/main.go` to assign the real evaluator.

No handler, orchestrator, or audit code changes are required — the interface
boundary isolates all policy implementation details.
