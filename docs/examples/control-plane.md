# Control-Plane Example Bundles

These bundles demonstrate how to declare MIDAS governance resources, approve
review-gated authority resources, evaluate runtime decisions, verify
idempotency/conflict behaviour, and inspect evidence integrity.

The examples are self-contained. Each YAML file creates the complete resource
chain needed by `POST /v1/evaluate`:

- `BusinessService`
- `Capability`
- `BusinessServiceCapability`
- `Process`
- `Surface`
- `Agent`
- `Profile`
- `Grant`

They do not rely on demo seed data, Kafka, Event Hubs, or Kubernetes-specific
resources.

## Prerequisites

Before using these examples, have:

- MIDAS running locally or in Kubernetes.
- A Postgres-backed store for the full control-plane, evidence, and
  idempotency behaviour.
- Either local open-auth mode, or bearer tokens with suitable permissions.
- A clean database, or a database where these example IDs have not already
  been applied.

The commands below assume:

```bash
export MIDAS_URL=http://localhost:8080
```

For bearer-token mode in the local Kubernetes quickstart path, also set:

```bash
export MIDAS_ADMIN_TOKEN=dev-admin-token
export MIDAS_OPERATOR_TOKEN=dev-operator-token
```

These are local-only, disposable example tokens created by
[`docs/getting-started/kubernetes.md`](../getting-started/kubernetes.md):

- use `dev-admin-token` for `/v1/controlplane/plan`,
  `/v1/controlplane/apply`, and Surface/Profile approval endpoints;
- use `dev-operator-token` for `POST /v1/evaluate` and evidence reads.

No maintainer-provided credentials are required for the local Kubernetes review
path. For non-local deployments, use tokens issued by your own environment.

Add exactly one matching header to each governed API call:

```bash
-H "Authorization: Bearer $MIDAS_ADMIN_TOKEN"
-H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN"
```

In local open-auth mode, omit the `Authorization` header.

## Directory Overview

| Bundle | Demonstrates | Expected outcome | Expected reason |
|---|---|---|---|
| [`01-banking-low-risk-payment.yaml`](../../examples/control-plane/01-banking-low-risk-payment.yaml) | Low-risk payment decision inside authority | `accept` | `WITHIN_AUTHORITY` |
| [`02-banking-confidence-escalation.yaml`](../../examples/control-plane/02-banking-confidence-escalation.yaml) | Escalation caused by low confidence | `escalate` | `CONFIDENCE_BELOW_THRESHOLD` |
| [`03-insurance-claims-consequence-escalation.yaml`](../../examples/control-plane/03-insurance-claims-consequence-escalation.yaml) | Escalation caused by monetary consequence exceeding authority | `escalate` | `CONSEQUENCE_EXCEEDS_LIMIT` |
| [`04-kyc-missing-context-clarification.yaml`](../../examples/control-plane/04-kyc-missing-context-clarification.yaml) | Request clarification caused by missing required context | `request_clarification` | `INSUFFICIENT_CONTEXT` |
| [`05-healthcare-prior-authorisation.yaml`](../../examples/control-plane/05-healthcare-prior-authorisation.yaml) | Domain-independent healthcare prior-authorisation decision | `accept` | `WITHIN_AUTHORITY` |

## Important Re-Run Note

These examples use stable IDs. They are designed for first-run clarity.
Re-applying them to the same database may produce updates, new Surface/Profile
versions, or conflicts depending on the resource kind and current database
state. For the cleanest experience, use a fresh local database or reset the
example resources.

Do not treat these as fully idempotent control-plane bundles. Runtime
evaluation idempotency is demonstrated separately with `request_source` and
`request_id`.

## Common API Sequence

The standard reviewer sequence is:

```bash
curl -i "$MIDAS_URL/healthz"
curl -i "$MIDAS_URL/readyz"
curl -i "$MIDAS_URL/metrics"
```

When `MIDAS_AUTH_MODE=required`, an unauthenticated evaluate call should return
`401`:

```bash
curl -i -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-low-risk-payment",
    "process_id": "proc-d40j-payment-release",
    "agent_id": "agent-d40j-payment-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "monetary", "amount": 1000, "currency": "GBP"},
    "context": {"account_id": "acct-001", "payment_reference": "pay-001"},
    "request_id": "req-d40j-unauth-001",
    "request_source": "control-plane-examples"
  }'
```

For each bundle:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/plan" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/01-banking-low-risk-payment.yaml | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/apply" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/01-banking-low-risk-payment.yaml | jq .
```

After apply, approve the review-gated resources:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/surfaces/<surface-id>/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "example-author",
    "approver_id":   "<business-owner>",
    "approver_name": "Example Approver"
  }' | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/profiles/<profile-id>/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "<business-owner>"
  }' | jq .
```

Then evaluate, replay exactly once, try a conflict, and retrieve evidence:

```bash
curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @evaluate.json | jq .

curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @evaluate.json | jq .

curl -i -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @evaluate-conflict.json

curl -s "$MIDAS_URL/v1/decisions/request/<request-id>?source=<request-source>" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/packet" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -i "$MIDAS_URL/metrics"
```

For the conflict test, keep the same `request_source` and `request_id` but
change a submitted field such as `confidence` or `consequence`. The expected
HTTP status is `409 Conflict`.

## 01 Banking Low-Risk Payment

Plan and apply:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/plan" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/01-banking-low-risk-payment.yaml | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/apply" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/01-banking-low-risk-payment.yaml | jq .
```

Approve:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/surfaces/surf-d40j-low-risk-payment/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "example-author",
    "approver_id":   "banking-governance",
    "approver_name": "Banking Governance"
  }' | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/profiles/profile-d40j-low-risk-payment/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "banking-governance"
  }' | jq .
```

Evaluate:

```bash
curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-low-risk-payment",
    "process_id": "proc-d40j-payment-release",
    "agent_id": "agent-d40j-payment-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "monetary", "amount": 1000, "currency": "GBP"},
    "context": {"account_id": "acct-001", "payment_reference": "pay-001"},
    "request_id": "req-d40j-payment-001",
    "request_source": "control-plane-examples"
  }' | jq .
```

Expected outcome: `accept` / `WITHIN_AUTHORITY`.

Evidence:

```bash
curl -s "$MIDAS_URL/v1/decisions/request/req-d40j-payment-001?source=control-plane-examples" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
```

## 02 Banking Confidence Escalation

Plan and apply:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/plan" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/02-banking-confidence-escalation.yaml | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/apply" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/02-banking-confidence-escalation.yaml | jq .
```

Approve:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/surfaces/surf-d40j-wire-confidence-review/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "example-author",
    "approver_id":   "wire-governance",
    "approver_name": "Wire Governance"
  }' | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/profiles/profile-d40j-wire-confidence/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "wire-governance"
  }' | jq .
```

Evaluate:

```bash
curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-wire-confidence-review",
    "process_id": "proc-d40j-wire-release",
    "agent_id": "agent-d40j-wire-evaluator",
    "confidence": 0.72,
    "consequence": {"type": "monetary", "amount": 1000, "currency": "GBP"},
    "context": {"account_id": "acct-002", "beneficiary_country": "GB"},
    "request_id": "req-d40j-wire-confidence-001",
    "request_source": "control-plane-examples"
  }' | jq .
```

Expected outcome: `escalate` / `CONFIDENCE_BELOW_THRESHOLD`.

Evidence:

```bash
curl -s "$MIDAS_URL/v1/decisions/request/req-d40j-wire-confidence-001?source=control-plane-examples" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
```

## 03 Insurance Claims Consequence Escalation

Plan and apply:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/plan" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/03-insurance-claims-consequence-escalation.yaml | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/apply" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/03-insurance-claims-consequence-escalation.yaml | jq .
```

Approve:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/surfaces/surf-d40j-claim-settlement/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "example-author",
    "approver_id":   "claims-governance",
    "approver_name": "Claims Governance"
  }' | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/profiles/profile-d40j-claims-standard/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "claims-governance"
  }' | jq .
```

Evaluate:

```bash
curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-claim-settlement",
    "process_id": "proc-d40j-claim-settlement",
    "agent_id": "agent-d40j-claims-evaluator",
    "confidence": 0.92,
    "consequence": {"type": "monetary", "amount": 5000, "currency": "GBP"},
    "context": {"claim_id": "claim-001", "policy_id": "policy-001"},
    "request_id": "req-d40j-claims-001",
    "request_source": "control-plane-examples"
  }' | jq .
```

Expected outcome: `escalate` / `CONSEQUENCE_EXCEEDS_LIMIT`.

Evidence:

```bash
curl -s "$MIDAS_URL/v1/decisions/request/req-d40j-claims-001?source=control-plane-examples" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
```

## 04 KYC Missing Context Clarification

Plan and apply:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/plan" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/04-kyc-missing-context-clarification.yaml | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/apply" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/04-kyc-missing-context-clarification.yaml | jq .
```

Approve:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/surfaces/surf-d40j-kyc-context-check/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "example-author",
    "approver_id":   "onboarding-governance",
    "approver_name": "Onboarding Governance"
  }' | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/profiles/profile-d40j-kyc-context/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "onboarding-governance"
  }' | jq .
```

Evaluate with `identity_document_reference` intentionally omitted:

```bash
curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-kyc-context-check",
    "process_id": "proc-d40j-kyc-review",
    "agent_id": "agent-d40j-kyc-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "risk_rating", "risk_rating": "low"},
    "context": {"customer_id": "cust-001"},
    "request_id": "req-d40j-kyc-001",
    "request_source": "control-plane-examples"
  }' | jq .
```

Expected outcome: `request_clarification` / `INSUFFICIENT_CONTEXT`.

Evidence:

```bash
curl -s "$MIDAS_URL/v1/decisions/request/req-d40j-kyc-001?source=control-plane-examples" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
```

## 05 Healthcare Prior Authorisation

Plan and apply:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/plan" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/05-healthcare-prior-authorisation.yaml | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/apply" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/05-healthcare-prior-authorisation.yaml | jq .
```

Approve:

```bash
curl -s -X POST "$MIDAS_URL/v1/controlplane/surfaces/surf-d40j-prior-authorisation/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "example-author",
    "approver_id":   "authorisation-governance",
    "approver_name": "Authorisation Governance"
  }' | jq .

curl -s -X POST "$MIDAS_URL/v1/controlplane/profiles/profile-d40j-prior-authorisation/approve" \
  -H "Authorization: Bearer $MIDAS_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "authorisation-governance"
  }' | jq .
```

Evaluate with generic, non-sensitive example data:

```bash
curl -s -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-prior-authorisation",
    "process_id": "proc-d40j-prior-authorisation",
    "agent_id": "agent-d40j-authorisation-evaluator",
    "confidence": 0.90,
    "consequence": {"type": "risk_rating", "risk_rating": "low"},
    "context": {"case_reference": "case-001", "requested_service_code": "svc-basic"},
    "request_id": "req-d40j-prior-auth-001",
    "request_source": "control-plane-examples"
  }' | jq .
```

Expected outcome: `accept` / `WITHIN_AUTHORITY`.

Evidence:

```bash
curl -s "$MIDAS_URL/v1/decisions/request/req-d40j-prior-auth-001?source=control-plane-examples" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
```

## Runtime Idempotency And Conflict

The runtime idempotency key is `(request_source, request_id)`. To demonstrate
it, replay the exact same evaluation request from any bundle. MIDAS should
return the existing governed result rather than creating a new evaluation.

Then keep the same `request_source` and `request_id` but change one submitted
field:

```bash
curl -i -X POST "$MIDAS_URL/v1/evaluate" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-low-risk-payment",
    "process_id": "proc-d40j-payment-release",
    "agent_id": "agent-d40j-payment-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "monetary", "amount": 1500, "currency": "GBP"},
    "context": {"account_id": "acct-001", "payment_reference": "pay-001"},
    "request_id": "req-d40j-payment-001",
    "request_source": "control-plane-examples"
  }'
```

Expected HTTP status: `409 Conflict`.

## Evidence

Every evaluation records an evidence envelope. Use the `envelope_id` returned by
`POST /v1/evaluate`:

```bash
curl -s "$MIDAS_URL/v1/decisions/request/<request-id>?source=<request-source>" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/integrity" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
curl -s "$MIDAS_URL/v1/evidence/envelopes/<envelope-id>/packet" \
  -H "Authorization: Bearer $MIDAS_OPERATOR_TOKEN" | jq .
```

The integrity endpoint verifies the audit-event chain for one envelope. The
packet endpoint returns the envelope, audit events, and integrity result in one
response.

## Metrics

`/metrics` can be checked before and after evaluations. Depending on runtime
configuration, it should include runtime, database, and outbox metrics.

```bash
curl -s "$MIDAS_URL/metrics"
```

## Eventing

Evaluation writes outbox rows as part of the runtime transaction. Dispatcher
publication depends on dispatcher and broker configuration. These examples do
not require Kafka or Event Hubs to demonstrate the core
control-plane/evaluate/evidence flow.

For eventing and dispatcher operations, see
[`docs/operations/events.md`](../operations/events.md).

## Links

- [Platform contract](../core/platform-contract.md)
- [First evaluation quickstart](../guides/quickstart-first-evaluation.md)
- [Control plane reference](../control-plane.md)
- [Runtime evaluation](../core/runtime-evaluation.md)
- [Envelope integrity](../core/envelope-integrity.md)
- [HTTP API reference](../api/http-api.md)
