# Drift

**Drift** is the divergence between the *projected* authority state (what
the Authority Graph says agents *should* be able to do) and the *runtime*
state (what agents *actually* did, as recorded in evidence envelopes).

Typical drift signals:

- An agent producing evidence on a surface where it holds no active grant.
- An evaluation citing a fail-mode policy version that no longer matches
  the projected effective policy.
- An escalation routed to a target that is no longer wired up.

## How drift is detected

Drift detection runs on a schedule and reads from the evidence store. The
Explorer surfaces the most recent drift report; the underlying detection
pipeline lives in the platform's analytics layer.

## Investigating drift

1. Open the workbench Evidence tab for the affected service.
2. Filter by the surface or agent flagged in the drift report.
3. Compare the cited grant / policy version with what the Authority Graph
   currently projects.
4. If the gap is real, fix it through the control plane (revoke the
   misaligned grant, replace the policy, re-issue evidence).
