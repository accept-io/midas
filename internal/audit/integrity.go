package audit

import (
	"context"
	"fmt"
	"sort"

	"github.com/accept-io/midas/internal/envelope"
)

type EnvelopeRepository interface {
	List(ctx context.Context) ([]*envelope.Envelope, error)
}

// AuditRepository is the minimal interface needed for integrity verification.
type AuditRepository interface {
	ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*AuditEvent, error)
}

// VerifyAuditIntegrity checks that all envelopes have complete, valid audit trails.
func VerifyAuditIntegrity(
	ctx context.Context,
	envelopeRepo EnvelopeRepository,
	auditRepo AuditRepository,
) error {
	if envelopeRepo == nil {
		return fmt.Errorf("envelope repository is nil")
	}
	if auditRepo == nil {
		return fmt.Errorf("audit repository is nil")
	}
	envelopes, err := envelopeRepo.List(ctx)
	if err != nil {
		return fmt.Errorf("listing envelopes: %w", err)
	}

	for _, env := range envelopes {
		if err := verifyEnvelope(ctx, auditRepo, env); err != nil {
			return err
		}
	}

	return nil
}

func verifyEnvelope(ctx context.Context, auditRepo AuditRepository, env *envelope.Envelope) error {
	events, err := auditRepo.ListByEnvelopeID(ctx, env.ID())
	if err != nil {
		return fmt.Errorf("envelope %s: listing events: %w", env.ID(), err)
	}

	if len(events) == 0 {
		return fmt.Errorf("envelope %s: no audit trail", env.ID())
	}

	// Sort events — do not assume the repository returns them in order.
	sort.Slice(events, func(i, j int) bool {
		return events[i].SequenceNo < events[j].SequenceNo
	})

	// Verify first event invariants.
	firstEvent := events[0]
	if firstEvent.SequenceNo != 1 {
		return fmt.Errorf("envelope %s: first event sequence_no=%d, expected 1",
			env.ID(), firstEvent.SequenceNo)
	}
	if firstEvent.PrevHash != "" {
		return fmt.Errorf("envelope %s: first event has non-empty prev_hash=%q",
			env.ID(), firstEvent.PrevHash)
	}

	// Verify hash chain integrity and sequence continuity.
	for i, curr := range events {
		expectedHash, err := ComputeEventHash(curr)
		if err != nil {
			return fmt.Errorf("envelope %s: failed to compute hash at sequence %d: %w",
				env.ID(), curr.SequenceNo, err)
		}

		if curr.EventHash != expectedHash {
			return fmt.Errorf("envelope %s: hash mismatch at sequence %d (stored=%s, computed=%s)",
				env.ID(), curr.SequenceNo, curr.EventHash, expectedHash)
		}

		if i > 0 {
			prev := events[i-1]

			if curr.SequenceNo != prev.SequenceNo+1 {
				return fmt.Errorf("envelope %s: sequence gap at sequence %d (previous=%d)",
					env.ID(), curr.SequenceNo, prev.SequenceNo)
			}

			if curr.PrevHash != prev.EventHash {
				return fmt.Errorf("envelope %s: chain break at sequence %d (prev_hash=%s, previous_event_hash=%s)",
					env.ID(), curr.SequenceNo, curr.PrevHash, prev.EventHash)
			}
		}
	}

	// Verify terminal event consistency.
	//
	// Terminal event rule: a closed envelope's final audit event must be
	// AuditEventEnvelopeClosed regardless of path (normal or escalated):
	//   Normal:   EVALUATING -> OUTCOME_RECORDED -> CLOSED
	//   Escalated: AWAITING_REVIEW -> CLOSED (after EscalationReviewed observation)
	//
	// Open envelopes (e.g. AWAITING_REVIEW) are valid mid-lifecycle states;
	// their final event is AuditEventEscalationPending, which is correct.
	// We only enforce the terminal event check for closed envelopes.
	finalEvent := events[len(events)-1]

	if env.State == envelope.EnvelopeStateClosed {
		if finalEvent.EventType != AuditEventEnvelopeClosed {
			return fmt.Errorf("envelope %s: final event is %s, expected %s",
				env.ID(), finalEvent.EventType, AuditEventEnvelopeClosed)
		}
	}

	// Cross-check: if the final audit event records a to_state, it must match
	// the envelope's persisted state. This catches cases where the audit log
	// and the envelope record have diverged.
	if toState, ok := finalEvent.Payload["to_state"].(string); ok && toState != "" {
		if toState != string(env.State) {
			return fmt.Errorf("envelope %s: state mismatch (envelope=%s, audit=%s)",
				env.ID(), env.State, toState)
		}
	}

	return nil
}
