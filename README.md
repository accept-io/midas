# MIDAS

**Authority governance engine for autonomous decisions and side-effecting actions.**

MIDAS determines whether an automated agent is within authority to perform a consequential action and produces a verifiable audit envelope explaining **why** the action was allowed, escalated, rejected, or requires clarification.

Designed for environments where autonomous systems must operate safely within defined authority boundaries:

- AI agents and agentic workflows
- automated lending and credit decisions
- financial transaction execution
- customer operations automation
- compliance-sensitive business processes

---

## Why MIDAS Exists

Modern systems increasingly delegate consequential decisions to software agents. Enterprises must still be able to answer:

- Is this agent authorised to perform this action?
- Under what confidence or risk thresholds?
- Who delegated that authority, and for how long?
- What policy rules apply at this decision surface?
- Can we prove the decision was within bounds after the fact?

MIDAS provides a **governance control plane** for these decisions.

It evaluates requests against an authority model and produces a **tamper-evident audit trail** for every evaluation.

---

## How MIDAS Fits In Your Architecture

MIDAS runs as standalone infrastructure.

Before executing a consequential action, your system calls MIDAS to request an authority decision.

```
Agent / Service  →  MIDAS /v1/evaluate  →  EXECUTE | ESCALATE | REJECT | REQUEST_CLARIFICATION
```

MIDAS governs **the action**, not the intelligence behind it. A Gemini-based agent, a LangChain workflow, and a bespoke Go service all integrate the same way — via HTTP or the MIDAS client SDK.

Typical p99 evaluation latency is **under 10ms** using the in-memory store.

---

## Core Concepts

### Decision Surface

A governed business capability where autonomous actions may occur. Each surface carries its own authority profile — confidence thresholds, consequence limits, and policy references.

Examples: `loan_auto_approval`, `payment_execution`, `customer_update`

### Agent

A system or AI model requesting authority to act. Agents must operate within a delegated authority grant — MIDAS does not permit self-authorization.

### Authority Profile

Defines the bounds of autonomous authority including:

- confidence thresholds
- consequence limits
- policy references
- required context fields

### Authority Grant

Delegates an authority profile to an agent for a given surface. Without a valid grant, an agent cannot act — which is the failure mode MIDAS is designed to prevent.

### Envelope

Full record of a governed evaluation including request metadata, authority evidence, decision explanation, and the complete audit event chain.

### Audit Event

Append-only, sequenced events linked with cryptographic hashes to form a tamper-evident audit trail. Each event is linked to its predecessor, making post-hoc tampering detectable.

---

## Evaluation Flow

```
Request
  ↓ Surface Resolution
  ↓ Agent Resolution
  ↓ Authority Chain Resolution
  ↓ Context Validation
  ↓ Threshold Evaluation
  ↓ Policy Evaluation
  ↓ Outcome Recorded
  ↓ Audit Envelope Persisted
```

Possible outcomes: `EXECUTE` · `ESCALATE` · `REJECT` · `REQUEST_CLARIFICATION`

---

## Quick Start

Run MIDAS locally:

```bash
go run ./cmd/midas
```

Evaluate a decision:

```bash
curl -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "REQ-123",
    "timestamp": "2026-03-11T10:23:00Z",
    "expires_at": "2026-03-11T10:23:30Z",
    "surface_id": "loan_auto_approval",
    "action": {
      "type": "loan.approve",
      "description": "Approve credit facility drawdown"
    },
    "agent": {
      "id": "agent-credit-1",
      "kind": "autonomous",
      "confidence": 0.87,
      "runtime": {
        "model": "gemini-1.5-pro",
        "version": "2.1.0"
      }
    },
    "delegation": {
      "initiated_by": "user:U456",
      "session_id": "SESS-789",
      "chain": ["user:U456", "service:credit-orchestrator", "agent:agent-credit-1"],
      "scope": ["loan_auto_approval"],
      "authorized_at": "2026-03-11T09:00:00Z",
      "authorized_until": "2026-03-11T17:00:00Z"
    },
    "consequence": {
      "type": "monetary",
      "amount": 4500,
      "currency": "GBP",
      "reversible": false
    },
    "context": {
      "customer_id": "C123",
      "risk_band": "low",
      "risk_band_source": "service:risk-engine",
      "risk_band_evaluated_at": "2026-03-11T10:20:00Z"
    }
  }'
```

Example response:

```json
{
  "request_id": "REQ-123",
  "outcome": "EXECUTE",
  "reason": "WITHIN_AUTHORITY",
  "envelope_id": "ENV-456",
  "evaluated_at": "2026-03-11T10:23:00Z",
  "authority_evidence": {
    "surface_id": "loan_auto_approval",
    "grant_id": "GRN-789",
    "profile_id": "PRF-001",
    "confidence_threshold": 0.85,
    "confidence_actual": 0.87,
    "consequence_limit": 5000,
    "consequence_actual": 4500
  },
  "audit_chain": {
    "envelope_id": "ENV-456",
    "event_count": 6,
    "chain_valid": true,
    "final_hash": "a3f8c2d1e9b74056..."
  }
}
```

Retrieve the full decision envelope:

```
GET /v1/decisions/request/{request_id}
```

---

## Storage Backends

| Backend | Configuration |
|---|---|
| In-memory (default) | `MIDAS_STORE=memory` |
| PostgreSQL | `MIDAS_STORE=postgres` |

---

## Observability

MIDAS includes three layers:

**Metrics** — evaluation latency, evaluation outcomes, transaction lifecycle signals.

**Structured Logging** — runtime diagnostics via Go `slog`.

**Immutable Audit Trail** — every decision produces a chain of hashed, sequenced events. The built-in integrity verifier checks ordering, sequence continuity, hash chain linkage, and envelope state consistency.

---

## Architecture Overview

```
Client / Agent
  ↓
MIDAS API
  ↓
Evaluation Engine
  ↓
Authority Model + Policy
  ↓
Envelope + Audit Events
  ↓
Persistence (Memory / PostgreSQL)
```

---

## Project Status

MIDAS is currently in **active development**.

### Available now

- authority evaluation engine
- transactional evaluation
- immutable audit events
- audit integrity verification
- structured logging
- in-memory and PostgreSQL persistence

### In progress

- review workflow for escalated decisions
- authority introspection APIs
- administrative APIs for surfaces, profiles, and grants
- metrics export

---

## License

Apache License 2.0
