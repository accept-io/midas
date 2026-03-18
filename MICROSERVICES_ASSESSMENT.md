# MIDAS Runtime Architecture and Decomposition Assessment

> **tl;dr** — MIDAS is a modular monolith by design, not by accident.  
> The evaluation loop is intentionally atomic: a single transaction creates the envelope, builds a verifiable audit chain, and produces a final decision.
>
> The correct evolution is not broad microservices decomposition, but selective extraction around a stable transactional core.

---

## 0. Architectural Positioning

MIDAS should be explicitly positioned as:

**A transactional governance runtime with a modular monolith core and controlled service extraction at the edges.**

### Core Principles

**Atomic evaluation is non-negotiable**  
Every decision must produce either:
- a complete, verifiable audit chain, or
- no result at all

**Audit integrity is verified at commit time**  
Not eventually. Not asynchronously.

**Policy is pluggable, not required**  
MIDAS must run standalone.

**Runtime ≠ Governance**  
Evaluation is millisecond-scale; governance workflows are human-scale.

**Do not distribute what must remain consistent**  
Service boundaries must follow consistency boundaries.

**Modularity before distribution**  
A well-structured monolith is more "modern" than a fragmented distributed system.

---

## 1. Current Architecture Summary

### Evaluation Flow (unchanged, but reframed)
```
HTTP request
    │
    ▼
decision.Orchestrator.Evaluate()
    │
    ├── Resolve authority chain (surface, agent, grants, profile)
    │
    ├── BEGIN TRANSACTION
    │
    ├── Envelope lifecycle
    ├── Audit chain construction (hash-linked)
    ├── Optional policy evaluation
    ├── Final outcome + integrity anchoring
    │
    └── COMMIT (all-or-nothing)
```

### Key Property

**The transaction boundary = the governance guarantee**

This is the architectural centre of gravity. Everything else is secondary.

---

## 2. Natural Service Boundaries

| Domain | Coupling | Extraction Readiness | Decision |
|--------|----------|---------------------|----------|
| Evaluation Engine | Extremely tight | Not viable | Keep in monolith |
| Audit | Extremely tight (hash chain) | Not viable | Keep in monolith |
| Envelope Store | Extremely tight | Not viable | Keep in monolith |
| Policy Engine | Loose (interface-based) | Already decoupled | Keep as plugin |
| Control Plane | Medium | High | Extract first |
| Authority Registry | Medium (read-heavy) | Conditional | Defer, prepare first |

---

## 3. Domain Analysis (Refined)

### 3.1 Evaluation Engine (`internal/decision/`)

This is the transactional core.

- Orchestrates all domains
- Owns transaction boundary
- Produces final decision + audit chain

**Conclusion:**  
This is not a candidate for decomposition. This **is** MIDAS.

---

### 3.2 Audit (`internal/audit/`)

The audit model is cryptographically chained and anchored to the envelope.

**Key invariant:**
```
Event[n].PrevHash == Event[n-1].EventHash
Envelope.FinalHash == Event[last].EventHash
```

If audit becomes asynchronous:
- hash chain breaks
- envelope cannot be verified at commit
- retry logic becomes unsafe

**Conclusion:**  
Audit must remain synchronous and in-transaction while this integrity model exists.

---

### 3.3 Envelope (`internal/envelope/`)

**Currently:**
- progressively built
- updated multiple times per evaluation

**Problem:**
- write amplification (6–7 updates)
- tight coupling to transaction lifecycle

**Target Direction:**  
Introduce an in-memory evaluation accumulator that:
- Builds envelope state progressively in memory
- Persists envelope + audit chain in fewer writes (ideally 1–2, not 7)
- Preserves integrity semantics exactly

**Important nuance:**  
The goal is NOT "single write at all costs" — it's clearer lifecycle boundaries and reduced write amplification while maintaining cryptographic integrity.

---

### 3.4 Policy Engine (`internal/policy/`)

Already correctly designed:
```go
type PolicyEvaluator interface {
    Evaluate(ctx context.Context, input PolicyInput) (PolicyResult, error)
}
```

- **Default:** `NoOpPolicyEvaluator`
- **Optional:** external (OPA, custom, etc.)
- **Called only when referenced**

**Conclusion:**  
This is already a clean extension point.  
Do not convert into a required service.

---

### 3.5 Control Plane (`internal/controlplane/`)

**Characteristics:**
- separate routes
- optional dependency
- governance-time workflows

**Conclusion:**  
This is the first extraction candidate.

---

### 3.6 Authority Registry (surface, authority, agent)

**Characteristics:**
- read-heavy
- write-light
- governance-owned

**Current constraint:**
- enforced by DB foreign keys
- tightly coupled to envelope schema

**Future requirement:**
- versioned reads
- decision-time consistency tokens

**Conclusion:**  
Not ready yet — requires preparation (see Phase 1).

---

## 4. Data Ownership Model (Refined)

### Current State

All tables share one database with FK constraints.

### Target State (Future)

| Domain | Ownership |
|--------|-----------|
| Evaluation + Envelope + Audit | Evaluation Service |
| Authority (surfaces, profiles, grants, agents) | Authority Service |
| Control Plane | Control Plane Service |

### Key Constraint

**Do not remove FK constraints until separation is imminent**

Instead:
- prepare denormalized identifiers (already done)
- introduce application-level validation
- only relax DB constraints when needed

---

## 5. Consistency Model

### 5.1 Must Remain ACID

- Envelope creation + audit chain
- State transitions
- Final outcome + integrity

### 5.2 Can Be Eventually Consistent

| Domain | Pattern |
|--------|---------|
| Authority reads | Cached (short TTL) |
| Control plane workflows | Async |
| Policy | Pluggable (sync call) |
| Audit verification | Background job |

---

## 6. Red Flags (Refined)

### 🔴 Core Risks

- Envelope ↔ Audit bidirectional dependency
- Hash chain requires synchronous ordering
- Sequence numbers assigned in-process
- Cross-domain FK constraints

### 🟡 Structural Risks

- Progressive envelope build
- Orchestrator spans multiple domains
- String-based error handling (fixable)

---

## 7. Phased Evolution Strategy (Reordered & Tightened)

### Phase 1 — Modularisation (Do Now)

**Focus:** improve structure without changing deployment

1. Introduce typed errors across domains
2. Refactor evaluation into an in-memory accumulator
3. Reduce envelope write amplification
4. Add caching layer for authority reads
5. Introduce authority version tokens in resolution
6. Document FK constraint removal strategy (do not execute yet)
7. Add application-level validation alongside FK checks (defense-in-depth)

**Outcome:**
- cleaner architecture
- better testability
- ready for extraction

**Important:** Phase 1 improvements are foundational, not optional.

---

### Phase 2 — Control Plane Extraction

Separate binary: `midas-controlplane`

- Own lifecycle, own scaling
- Communicates via API, not DB

**Outcome:**
- visible "modern architecture"
- zero impact on evaluation integrity

---

### Phase 3 — Authority Registry (Optional)

Only if needed for scale/team boundaries.

**Requirements:**
- versioned authority reads
- optimistic validation at evaluation time
- cache-first access pattern

**New outcome:**  
Reject → `STALE_AUTHORITY`

**Risk:** High  
**Decision:** Optional

---

### Phase 4 — Audit Separation (Only with Product Approval)

**Requires:**
- redesign of integrity model
- acceptance of eventual consistency

This is a **governance trade-off**, not a technical refactor.

---

## 8. Target Deployment Model

### Near-Term (Recommended)
```
[ API Gateway ]
     │
     ├── midas-runtime
     │     - evaluate
     │     - envelopes
     │     - audit
     │
     ├── midas-controlplane
     │     - apply
     │     - approval
     │
     └── optional policy adapter

- shared database
- shared observability
- separate scaling
```

---

## 9. What "Modern Architecture" Means for MIDAS

MIDAS is modern when it has:

- clear domain boundaries
- ports-and-adapters design
- stable contracts
- pluggable components
- strong transactional guarantees
- extraction seams designed in advance

**Modern ≠ microservices**  
**Modern = intentional architecture with controlled complexity**

---

## 10. Final Recommendation

MIDAS should evolve as:

**A modular monolith runtime with selective service extraction at the governance layer.**

### Keep together
- evaluation
- envelope
- audit
- escalation

### Keep pluggable
- policy

### Extract first
- control plane

### Prepare (not rush)
- authority registry

### Avoid
- premature microservices
- async audit
- early FK removal
- distributed evaluation
- treating Phase 1 as optional

---

## 11. Key Insight

**The strength of MIDAS is not that it can be decomposed — it's that it doesn't need to be, except where it makes sense.**

---

*Assessment based on codebase state as of branch `feat/observability`, 2026-03-18.*  
*Key files analyzed: `internal/decision/orchestrator.go`, `internal/store/postgres/schema.sql`, `internal/audit/integrity.go`, `internal/envelope/envelope.go`, `internal/httpapi/server.go`, `internal/controlplane/apply/service.go`, `internal/policy/policy.go`.*