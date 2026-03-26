# Envelope Build Pattern Analysis

> Analysis of the current progressive envelope build in `internal/decision/orchestrator.go`,
> prepared to support the evaluation accumulator refactor described in MICROSERVICES_ASSESSMENT.md.
>
> **No Go code is modified by this document.**

---

## 1. Current Flow — DB Writes per Evaluation

The entire evaluation runs inside `store.WithTx("evaluation", fn)` (line 172). All writes
below are in the same Postgres transaction. A failure at any point rolls everything back.

### 1.1 Happy path (Accept outcome)

```
evaluate() called inside transaction
│
├─ WRITE 1: Envelopes.Create(env)                           [line 254]
│   State: RECEIVED
│   Sections set: Identity, Submitted, Integrity.SubmittedHash
│
├─ Audit.Append(envelope.created)                           [line 263]
│   → Repo assigns SequenceNo=1, PrevHash="", computes EventHash
│   → Returns event with .Hash populated
│
├─ WRITE 2: Envelopes.Update(env)                           [line 273]
│   Adds: Integrity.FirstEventHash, Integrity.FinalEventHash,
│          Integrity.AuditEventIDs[0]
│
├─ applyStep(RECEIVED→EVALUATING, evaluation_started)       [line 278]
│   ├─ env.Transition(EVALUATING)                           [in-memory]
│   ├─ WRITE 3: Envelopes.Update(env)                      [line 559]
│   │   Adds: State=EVALUATING, UpdatedAt
│   ├─ Audit.Append(evaluation_started)
│   │   → Repo assigns SequenceNo=2, PrevHash=event[1].hash
│   ├─ env.Integrity updated in-memory (AuditEventIDs, FinalEventHash)
│   └─ WRITE 4: Envelopes.Update(env)                      [line 584]
│       Adds: Integrity.AuditEventIDs[1], Integrity.FinalEventHash
│
│   — Resolution phase (reads only, observational events in-memory) —
│
├─ resolveSurface()                                         [line 287]
├─ Audit.Append(surface_resolved)                           [line 295]  ← in-memory only
├─ resolveAgent()                                           [line 302]
├─ Audit.Append(agent_resolved)                             [line 310]  ← in-memory only
├─ resolveAuthorityChain()                                  [line 317]
├─ Audit.Append(authority_chain_resolved)                   [line 325]  ← in-memory only
│
│   [env.Integrity.AuditEventIDs and FinalEventHash updated in-memory
│    after each of the above; not persisted until Write 5]
│
├─ env.Resolved = ResolvedAuthority{...}                    [line 332]  ← in-memory
├─ env.ResolvedSurface/Profile/Grant/Agent IDs              [line 370]  ← in-memory
├─ env.Evaluation = Evaluation{EvaluatedAt, Explanation}   [line 382]  ← in-memory
├─ WRITE 5: Envelopes.Update(env)                          [line 404]
│   Adds: Resolved (full section), Evaluation (seeded),
│          denormalized authority chain columns (schema v2.1)
│
├─ Audit.Append(context_validated)                          [line 409]  ← in-memory only
├─ Audit.Append(confidence_checked)                         [line 417]  ← in-memory only
├─ Audit.Append(consequence_checked)                        [line 425]  ← in-memory only
├─ Audit.Append(policy_evaluated)    [line 438, if policy]  ← in-memory only
│
└─ finish(Accept, WITHIN_AUTHORITY)
    ├─ env.Evaluation.Outcome, ReasonCode set               [in-memory]
    ├─ applyStep(EVALUATING→OUTCOME_RECORDED, outcome_recorded) [line 501]
    │   ├─ env.Transition(OUTCOME_RECORDED)
    │   ├─ WRITE 6: Envelopes.Update(env)    (state + Evaluation.Outcome/ReasonCode)
    │   ├─ Audit.Append(outcome_recorded)
    │   └─ WRITE 7: Envelopes.Update(env)    (integrity)
    └─ applyStep(OUTCOME_RECORDED→CLOSED, envelope_closed)  [line 508]
        ├─ env.Transition(CLOSED)             (sets ClosedAt)
        ├─ WRITE 8: Envelopes.Update(env)    (state=closed, ClosedAt)
        ├─ Audit.Append(envelope_closed)
        └─ WRITE 9: Envelopes.Update(env)    (integrity, FinalEventHash)
```

**Total envelope DB writes (happy path): 1 Create + 8 Updates = 9**
**Total audit DB writes (happy path): 10–11 Appends** (3 observational resolution events +
context + confidence + consequence + optional policy + evaluation_started + outcome_recorded +
envelope_closed + envelope_created)

### 1.2 Early-reject path (e.g., SURFACE_NOT_FOUND)

```
evaluate() → Envelopes.Create  [Write 1]
           → Audit.Append(envelope.created)
           → Envelopes.Update  [Write 2]  ← integrity
           → applyStep(EVALUATING)
               → Envelopes.Update  [Write 3]  ← state
               → Audit.Append(evaluation_started)
               → Envelopes.Update  [Write 4]  ← integrity
           → resolveSurface() → returns Reject, SURFACE_NOT_FOUND
           → finish(Reject)
               → applyStep(OUTCOME_RECORDED)
                   → Envelopes.Update  [Write 5]  ← state + evaluation
                   → Audit.Append(outcome_recorded)
                   → Envelopes.Update  [Write 6]  ← integrity
               → applyStep(CLOSED)
                   → Envelopes.Update  [Write 7]  ← state + ClosedAt
                   → Audit.Append(envelope_closed)
                   → Envelopes.Update  [Write 8]  ← integrity
```

**Total (early reject): 1 Create + 7 Updates = 8 envelope writes, 4 audit writes**

### 1.3 Escalation path

Same as happy path through `finish()`, then:

```
finish(Escalate)
    ├─ applyStep(EVALUATING→ESCALATED, outcome_recorded)
    │   ├─ WRITE 6: Envelopes.Update  (state=escalated)
    │   ├─ Audit.Append(outcome_recorded)
    │   └─ WRITE 7: Envelopes.Update  (integrity)
    └─ applyStep(ESCALATED→AWAITING_REVIEW, escalation_pending)
        ├─ WRITE 8: Envelopes.Update  (state=awaiting_review)
        ├─ Audit.Append(escalation_pending)
        └─ WRITE 9: Envelopes.Update  (integrity)
```

**Total (escalation): 1 Create + 8 Updates = 9 envelope writes** (envelope stays OPEN)

---

## 2. Integrity Constraint Map — What Depends on What

### 2.1 FK constraint: audit_events → operational_envelopes

```
audit_events.envelope_id  REFERENCES  operational_envelopes(id)
```

**Implication:** `Envelopes.Create()` must be called before ANY `Audit.Append()`.

The envelope ID is assigned in-process (`uuid.NewString()` at line 248), not by the
database sequence. So the ID is known before `Create()` — but the DB row must exist for
FK satisfaction.

### 2.2 Hash chain — sequential dependency between audit events

Each event's `PrevHash` is set inside `AuditEventRepository.Append()` (memory_repo.go
line 33):

```go
ev.PrevHash = envelopeEvents[len(envelopeEvents)-1].EventHash
```

`EventHash` is computed by `ComputeEventHash(ev)` which hashes the event *including*
`PrevHash`. This means:

- **Event N's hash depends on Event N-1's hash.**
- Events cannot be appended out of order.
- Events cannot be batched and inserted in parallel.
- SequenceNo is `len(existing) + 1` — also assigned at Append time.

### 2.3 Integrity anchoring — envelope ↔ first audit event

```
Integrity.FirstEventHash = firstEvent.Hash    [set at line 271]
```

This value is used by `VerifyAuditIntegrity()` to anchor the chain. It must equal
`AuditEvent[SequenceNo=1].EventHash` for the envelope. It is set in-memory immediately
after the first `Append()` and persisted in Write 2.

**The anchor is readable from the envelope without loading any audit events.**

### 2.4 Transition invariants — what state_machine.checkInvariantsFor requires

| Transition | Required field |
|-----------|----------------|
| `EVALUATING → OUTCOME_RECORDED` | `env.Evaluation.Explanation != nil` |
| `EVALUATING → ESCALATED` | `env.Evaluation.Explanation != nil` |
| `ANY → CLOSED` | `env.Evaluation.Outcome != ""` and `env.Evaluation.ReasonCode != ""` |
| `AWAITING_REVIEW → CLOSED` | `env.Review != nil` |

The `Evaluation.Explanation` is seeded at line 382 (before Write 5). `Outcome` and
`ReasonCode` are set in `finish()` at line 467–468 (before the applyStep calls in
`finish()`). These are in-memory state requirements, not DB-read requirements.

### 2.5 Denormalized columns — no FK on subject, but FKs on authority chain

The schema v2.1 denormalized columns:

```
resolved_surface_id, resolved_surface_version  →  FK to decision_surfaces(id, version)
resolved_profile_id, resolved_profile_version  →  FK to authority_profiles(id, version)
resolved_grant_id                              →  FK to authority_grants(id)
resolved_agent_id                              →  FK to agents(id)
resolved_subject_id                            →  no FK (subject may not exist in DB)
```

These are populated in Write 5. They cannot be populated in Write 1 (Create) because they
are unknown until the authority chain is resolved.

---

## 3. State Dependencies — Can DB ID Be Deferred?

**Short answer: Yes, for the envelope. No, for audit events.**

The envelope ID is `uuid.NewString()` (line 248) — assigned in-process before `Create()`.
No DB-generated sequence or serial is used. The ID is available immediately for use in
audit events (`EnvelopeID: env.ID()`).

However, `audit_events.envelope_id` has a FK constraint. If envelope `Create()` is
deferred to after all audit events are constructed, the audit `Append()` calls (which insert
rows) would fail the FK check.

**Within a transaction, this is solvable with deferred constraints** (`DEFERRABLE INITIALLY
DEFERRED` in the schema). Without that, the constraint is checked at statement time, not
commit time — meaning `Append()` would fail if `Create()` hasn't run yet.

**Currently:** `Create()` runs at line 254, before any `Append()`. This ordering is safe
and correct as-is.

**Read-after-write patterns:** There are no read-after-write patterns that depend on the
envelope existing in the DB between `Create()` and the final commit. All reads of `env`
state within `evaluate()` use the in-memory object, not a DB refetch.

---

## 4. What the Observational Events Already Do Right

The comment in `appendObservationEvent()` (line 807–823) documents an existing partial
accumulator pattern:

> Observational events do NOT persist the envelope immediately. The integrity fields remain
> in-memory until the next applyStep call.

This means the 5–6 observational events (surface_resolved, agent_resolved,
authority_chain_resolved, context_validated, confidence_checked, consequence_checked,
policy_evaluated) already defer their envelope update. They update `env.Integrity` in
memory; that state is flushed by the next `applyStep` or by the explicit `Update` at
line 404.

**The existing write amplification comes from `applyStep`, not from observational events.**
Each `applyStep` call does two `Envelopes.Update()` calls: one for the state transition
and one for the integrity update. With 4–5 `applyStep` calls per evaluation, this
produces 8–10 envelope updates.

---

## 5. Proposed Accumulator Design

### 5.1 The core insight

The intermediate envelope writes exist to keep the DB row consistent as evaluation
progresses. But since the entire evaluation is a single transaction that either commits
or rolls back atomically, intermediate consistency is not externally observable. External
readers see only the pre-transaction state or the post-commit state.

This means: **all intermediate `Envelopes.Update()` calls are logically unnecessary.**
The only required envelope writes are:
1. `Create()` — to satisfy the FK constraint before any audit `Append()`
2. One final `Update()` — to persist the terminal state, all resolved sections,
   evaluation outcome, and integrity anchors

### 5.2 Minimum viable write sequence

```
Persist sequence (inside single transaction)
│
├─ Envelopes.Create(env)           [initial state: RECEIVED, sections 1+2 only]
│                                   ↑ Must happen before any Audit.Append (FK)
│
├─ Audit.Append(event_1)           [SequenceNo=1, PrevHash=""]
├─ Audit.Append(event_2)           [SequenceNo=2, PrevHash=event_1.Hash]
├─ ...
└─ Audit.Append(event_N)           [SequenceNo=N, PrevHash=event_{N-1}.Hash]
│
└─ Envelopes.Update(env)           [terminal state: all sections, final integrity]
```

**From 9 writes to 3 writes** (for a happy-path evaluation): Create + N audit appends +
1 Update. The audit appends are unavoidable (sequential hash dependency). The envelope
write count goes from 9 to 2.

### 5.3 Accumulator interface sketch

```go
// EvaluationAccumulator collects evaluation state and audit events in memory,
// then persists atomically at the end. This eliminates intermediate envelope
// updates while preserving all integrity guarantees.
//
// Usage pattern:
//
//   acc := newEvaluationAccumulator(env, now)
//   acc.recordEvent(audit.AuditEventEnvelopeCreated, payload)
//   acc.applyTransition(EnvelopeStateEvaluating, audit.AuditEventEvaluationStarted, nil)
//   // ... resolve, check thresholds, etc. ...
//   acc.recordObservation(audit.AuditEventSurfaceResolved, map[string]any{...})
//   acc.applyTransition(EnvelopeStateOutcomeRecorded, audit.AuditEventOutcomeRecorded, payload)
//   acc.applyTransition(EnvelopeStateClosed, audit.AuditEventEnvelopeClosed, nil)
//   return acc.persist(ctx, repos)  // single Create + N Appends + 1 Update
//
type evaluationAccumulator struct {
    env         *envelope.Envelope
    pendingEvents []*audit.AuditEvent  // ordered; hash chain NOT yet computed
    now         time.Time
}

// recordEvent queues an audit event without computing hashes yet.
func (a *evaluationAccumulator) recordEvent(
    eventType audit.AuditEventType,
    payload map[string]any,
) {
    ev := audit.NewEvent(
        a.env.ID(),
        a.env.RequestSource(),
        a.env.RequestID(),
        eventType,
        audit.EventPerformerSystem,
        "midas-orchestrator",
        payload,
    )
    a.pendingEvents = append(a.pendingEvents, ev)
}

// applyTransition records a state change + its audit event in memory.
// Validates the transition edge and content invariants immediately,
// so callers get the same error semantics as the current applyStep.
func (a *evaluationAccumulator) applyTransition(
    next envelope.EnvelopeState,
    eventType audit.AuditEventType,
    payload map[string]any,
) error {
    from := a.env.State
    if err := a.env.Transition(next, a.now); err != nil {
        return fmt.Errorf("transition %s→%s: %w", from, next, err)
    }
    cloned := clonePayloadWithStates(payload, from, next)
    a.recordEvent(eventType, cloned)
    return nil
}

// recordObservation queues an observational audit event (no state change).
func (a *evaluationAccumulator) recordObservation(
    eventType audit.AuditEventType,
    payload map[string]any,
) {
    a.recordEvent(eventType, payload)
}

// persist atomically writes the complete evaluation to the database.
// Call order: Create → N×Append → Update.
// The hash chain is computed here, just before Append, so hashes
// reflect the final event order.
func (a *evaluationAccumulator) persist(
    ctx context.Context,
    repos *store.Repositories,
) error {
    // Step 1: Create envelope row (required before any Audit.Append for FK)
    if err := repos.Envelopes.Create(ctx, a.env); err != nil {
        return fmt.Errorf("create envelope: %w", err)
    }

    // Step 2: Append all audit events in order.
    // The repository computes SequenceNo and hash chain internally.
    for _, ev := range a.pendingEvents {
        if err := repos.Audit.Append(ctx, ev); err != nil {
            return fmt.Errorf("audit append %s: %w", ev.EventType, err)
        }
        // Track integrity anchors as events are appended.
        a.env.Integrity.AuditEventIDs = append(a.env.Integrity.AuditEventIDs, ev.ID)
        if a.env.Integrity.FirstEventHash == "" {
            a.env.Integrity.FirstEventHash = ev.Hash
        }
        a.env.Integrity.FinalEventHash = ev.Hash
    }

    // Step 3: Single final envelope update with complete state.
    if err := repos.Envelopes.Update(ctx, a.env); err != nil {
        return fmt.Errorf("persist final envelope state: %w", err)
    }

    return nil
}
```

---

## 6. Recommended Persistence Points

| Write | Required | Reason |
|-------|----------|--------|
| `Envelopes.Create()` | Yes — must be first | FK constraint: audit rows need the envelope row |
| `Envelopes.Update()` after `envelope.created` event | **No** | Intermediate; integrity accumulates fine in-memory |
| `Envelopes.Update()` inside each `applyStep` (state only) | **No** | In-tx; intermediate state not externally visible |
| `Envelopes.Update()` inside each `applyStep` (integrity) | **No** | Same rationale |
| `Envelopes.Update()` at line 404 (resolved + evaluation) | **No** | In-tx; can batch into final update |
| `Audit.Append()` × N | Yes — must be in order | Sequential hash chain dependency |
| Final `Envelopes.Update()` (terminal state) | Yes — must be last | Persist terminal state, final integrity |

**Required writes: 1 Create + N Audit Appends + 1 final Update**

The N intermediate `Envelopes.Update()` calls can all be eliminated.

---

## 7. Red Flags

### 🔴 Sequence numbers assigned in-repo, not in orchestrator

`SequenceNo` and `PrevHash` are assigned inside `AuditEventRepository.Append()` (memory
repo, line 30–36; postgres repo follows same pattern). The orchestrator never touches these
fields — it only reads `.Hash` after the call returns.

**Why this matters for accumulator design:**
If the accumulator pre-builds events before calling `Append`, the repo's `Append` will
still assign `SequenceNo` and `PrevHash` correctly, as long as events are appended in
order in the same transaction. This is already safe — no change needed to repo interfaces.

**If hash chain pre-computation is ever moved to the accumulator**, the repo's `Append`
must accept pre-set values (or trust caller-provided `SequenceNo`/`PrevHash`). This is a
repo interface breaking change.

### 🔴 `NewEvent()` uses `time.Now()` internally (not the injected clock)

`audit.NewEvent()` in `event.go` line 57 calls `time.Now().UTC()` directly:

```go
OccurredAt: time.Now().UTC(),
```

The orchestrator injects a `Clock` for deterministic testing (`o.clock()`), but this is
not threaded through to `audit.NewEvent`. If events are accumulated in memory and
`Append()` is deferred, the `OccurredAt` timestamps will still be set at `NewEvent()`
call time (mid-evaluation), not at `Append()` time — this is actually correct for
audit semantics, but worth noting for test determinism.

**Risk:** Tests that freeze time via an injected clock will still see real timestamps on
audit events. Currently this doesn't matter (tests check hashes, not timestamps), but could
become surprising.

### 🟡 Two `Envelopes.Update()` calls per `applyStep` — first write is state-only

Each `applyStep` call writes the envelope twice:

1. After `env.Transition()` — to persist the new state (line 559)
2. After `Audit.Append()` — to persist updated integrity (line 584)

These could be collapsed to a single write if the accumulator builds up state in memory
and doesn't call `applyStep` at all. But if `applyStep` is kept as the primary API, the
simplest win is to merge the two writes into one (transition + integrity in a single
`Update`).

### 🟡 `appendObservationEvent` comment warns about early-return paths

The comment at line 807–823 is explicit:

> MAINTAINER WARNING: If you add an early-return path that bypasses applyStep, you MUST
> either call repos.Envelopes.Update(ctx, env) before returning, or ensure the path is
> read-only and does not append observation events.

If the accumulator approach is adopted and `applyStep` is replaced with in-memory
operations, this invariant must be re-expressed in the accumulator's `persist()` method —
the accumulator's final `Update()` always flushes everything, eliminating the risk of
an orphaned in-memory integrity update.

### 🟡 `env.Evaluation.Explanation` is checked by `Transition()` before `finish()` is called

`checkInvariantsFor()` verifies `Explanation != nil` before allowing `EVALUATING →
OUTCOME_RECORDED`. This is populated at line 382 (before Write 5). In the accumulator
design, `applyTransition()` calls `env.Transition()` which will run the same check — so
the invariant is preserved as long as `Explanation` is set in-memory before calling
`applyTransition(OUTCOME_RECORDED)`.

This is safe — the in-memory set happens at the same logical point in the flow.

### 🟡 The `applyStep` double-write is explicitly documented as a tradeoff

Line 539–540 in the current code reads:

> // The double-write (steps 2 and 4) is a deliberate v1 tradeoff: it ensures
> // the envelope row is self-describing without requiring a join to audit.

This is true for reads that happen *during* evaluation — but within a transaction, no
external reader can see the intermediate state anyway. The self-describing property is what
matters at commit time, which the accumulator's final `Update()` fully preserves. The
tradeoff comment is accurate for v1 but does not constrain v2 design.

---

## 8. Write Count Summary

| Path | Current envelope writes | Target envelope writes | Reduction |
|------|------------------------|----------------------|-----------|
| Happy path (Accept) | 1 Create + 8 Update = **9** | 1 Create + 1 Update = **2** | 78% |
| Early reject | 1 Create + 7 Update = **8** | 1 Create + 1 Update = **2** | 75% |
| Escalation | 1 Create + 8 Update = **9** | 1 Create + 1 Update = **2** | 78% |
| Audit writes | 10–11 Append | 10–11 Append | 0% (sequential dep.) |

The audit write count does not change — the sequential hash chain dependency requires each
event to be appended in order. The gain is entirely in eliminated intermediate envelope
updates.

---

*Analysis based on `internal/decision/orchestrator.go` as of branch `feat/observability`.*
*Key line references: Create=254, first integrity update=273, applyStep=544–589,*
*resolved+eval update=404, finish=458–522, appendObservationEvent=790–824.*
