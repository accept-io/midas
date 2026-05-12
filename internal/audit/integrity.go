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

// IntegrityErrorKind is the stable taxonomy of integrity-verification
// findings the per-envelope verifier can distinguish (D30d). The
// vocabulary is bounded to the kinds the existing verifier already
// implements; new kinds must be added alongside a new check, not
// invented post-hoc on the read surface.
type IntegrityErrorKind string

const (
	// IntegrityErrorKindNone is the empty value used for valid chains.
	IntegrityErrorKindNone IntegrityErrorKind = ""
	// IntegrityErrorKindMissingEvents is reported when the audit chain
	// for the envelope is empty.
	IntegrityErrorKindMissingEvents IntegrityErrorKind = "missing_events"
	// IntegrityErrorKindSequenceGap is reported when the chain has a
	// non-monotonic SequenceNo: first event sequence_no != 1, or any
	// non-adjacent (prev_seq, curr_seq) pair.
	IntegrityErrorKindSequenceGap IntegrityErrorKind = "sequence_gap"
	// IntegrityErrorKindPrevHashMismatch is reported when the chain's
	// prev_hash linkage is broken: first event has a non-empty
	// prev_hash, or any event's prev_hash does not equal the previous
	// event's stored hash.
	IntegrityErrorKindPrevHashMismatch IntegrityErrorKind = "prev_hash_mismatch"
	// IntegrityErrorKindEventHashMismatch is reported when an event's
	// stored EventHash does not equal ComputeEventHash recomputed from
	// the canonical event content.
	IntegrityErrorKindEventHashMismatch IntegrityErrorKind = "event_hash_mismatch"
	// IntegrityErrorKindTerminalStateMismatch is reported when a closed
	// envelope's final audit event is not ENVELOPE_CLOSED, or when the
	// final event's to_state payload does not match the envelope's
	// persisted state.
	IntegrityErrorKindTerminalStateMismatch IntegrityErrorKind = "terminal_state_mismatch"
	// IntegrityErrorKindUnknown is a defensive fallback. Production
	// verification paths should always produce one of the kinds above;
	// "unknown" exists so a future check added without a corresponding
	// kind cannot silently propagate empty error metadata.
	IntegrityErrorKindUnknown IntegrityErrorKind = "unknown"
)

// IntegrityVerificationResult is the structured outcome of a
// per-envelope integrity check (D30d). Valid is true only when every
// check passes; on a failed chain Valid is false, ErrorKind is one of
// the IntegrityErrorKind* constants, and ErrorMessage carries the
// verifier's human-readable diagnostic. ChainLength is the number of
// audit events the verifier observed (zero on missing_events).
// FirstEventHash / FinalEventHash are taken from the observed chain
// (events[0].EventHash and events[len-1].EventHash); they are not
// cross-checked against Envelope.Integrity in this tranche.
type IntegrityVerificationResult struct {
	EnvelopeID     string
	Valid          bool
	ChainLength    int
	FirstEventHash string
	FinalEventHash string
	ErrorKind      IntegrityErrorKind
	ErrorMessage   string
}

// VerifyEnvelopeIntegrity (D30d) is the exported per-envelope
// integrity verifier the HTTP layer uses to back
// GET /v1/evidence/envelopes/{id}/integrity. It returns
// (result, nil) for both valid and invalid chains — Valid=false on
// the result indicates an integrity finding, not a transport
// failure. (zero result, err) is reserved for genuine repository /
// system errors (failed ListByEnvelopeID, failed ComputeEventHash):
// the HTTP layer surfaces these as 500 because they prevent
// verification from completing.
//
// The verifier shares its check vocabulary with VerifyAuditIntegrity
// — the bulk function delegates here and converts integrity findings
// back into errors. New checks must be added in this helper (and
// gain a matching IntegrityErrorKind constant); the bulk wrapper
// stays as a thin adapter.
func VerifyEnvelopeIntegrity(
	ctx context.Context,
	auditRepo AuditRepository,
	env *envelope.Envelope,
) (IntegrityVerificationResult, error) {
	if env == nil {
		return IntegrityVerificationResult{}, fmt.Errorf("envelope is nil")
	}
	if auditRepo == nil {
		return IntegrityVerificationResult{}, fmt.Errorf("audit repository is nil")
	}

	res := IntegrityVerificationResult{EnvelopeID: env.ID()}

	events, err := auditRepo.ListByEnvelopeID(ctx, env.ID())
	if err != nil {
		// Repository failure prevents verification — propagate so the
		// HTTP layer can return 500. Bulk callers wrap with the
		// existing "listing events" prefix to preserve test substrings.
		return IntegrityVerificationResult{}, fmt.Errorf("listing events: %w", err)
	}

	if len(events) == 0 {
		res.Valid = false
		res.ChainLength = 0
		res.ErrorKind = IntegrityErrorKindMissingEvents
		res.ErrorMessage = "no audit trail"
		return res, nil
	}

	// Defensive sort — both production repositories already return
	// SequenceNo-ASC, but the verifier does not trust that contract.
	sort.Slice(events, func(i, j int) bool {
		return events[i].SequenceNo < events[j].SequenceNo
	})

	res.ChainLength = len(events)
	res.FirstEventHash = events[0].EventHash
	res.FinalEventHash = events[len(events)-1].EventHash

	// First-event invariants.
	first := events[0]
	if first.SequenceNo != 1 {
		res.Valid = false
		res.ErrorKind = IntegrityErrorKindSequenceGap
		res.ErrorMessage = fmt.Sprintf("first event sequence_no=%d, expected 1", first.SequenceNo)
		return res, nil
	}
	if first.PrevHash != "" {
		res.Valid = false
		res.ErrorKind = IntegrityErrorKindPrevHashMismatch
		res.ErrorMessage = fmt.Sprintf("first event has non-empty prev_hash=%q", first.PrevHash)
		return res, nil
	}

	// Per-event hash + chain integrity.
	for i, curr := range events {
		expectedHash, hashErr := ComputeEventHash(curr)
		if hashErr != nil {
			// Hash computation failure is a system/data error, not an
			// integrity finding — propagate so the HTTP layer can
			// return 500. Bulk wrapper preserves the existing
			// "failed to compute hash at sequence %d" substring.
			return IntegrityVerificationResult{}, fmt.Errorf(
				"failed to compute hash at sequence %d: %w", curr.SequenceNo, hashErr)
		}
		if curr.EventHash != expectedHash {
			res.Valid = false
			res.ErrorKind = IntegrityErrorKindEventHashMismatch
			res.ErrorMessage = fmt.Sprintf(
				"hash mismatch at sequence %d (stored=%s, computed=%s)",
				curr.SequenceNo, curr.EventHash, expectedHash)
			return res, nil
		}
		if i > 0 {
			prev := events[i-1]
			if curr.SequenceNo != prev.SequenceNo+1 {
				res.Valid = false
				res.ErrorKind = IntegrityErrorKindSequenceGap
				res.ErrorMessage = fmt.Sprintf(
					"sequence gap at sequence %d (previous=%d)",
					curr.SequenceNo, prev.SequenceNo)
				return res, nil
			}
			if curr.PrevHash != prev.EventHash {
				res.Valid = false
				res.ErrorKind = IntegrityErrorKindPrevHashMismatch
				res.ErrorMessage = fmt.Sprintf(
					"chain break at sequence %d (prev_hash=%s, previous_event_hash=%s)",
					curr.SequenceNo, curr.PrevHash, prev.EventHash)
				return res, nil
			}
		}
	}

	// Terminal-event consistency for closed envelopes.
	final := events[len(events)-1]
	if env.State == envelope.EnvelopeStateClosed {
		if final.EventType != AuditEventEnvelopeClosed {
			res.Valid = false
			res.ErrorKind = IntegrityErrorKindTerminalStateMismatch
			res.ErrorMessage = fmt.Sprintf(
				"final event is %s, expected %s",
				final.EventType, AuditEventEnvelopeClosed)
			return res, nil
		}
	}
	if toState, ok := final.Payload["to_state"].(string); ok && toState != "" {
		if toState != string(env.State) {
			res.Valid = false
			res.ErrorKind = IntegrityErrorKindTerminalStateMismatch
			res.ErrorMessage = fmt.Sprintf(
				"state mismatch (envelope=%s, audit=%s)",
				env.State, toState)
			return res, nil
		}
	}

	res.Valid = true
	return res, nil
}

// VerifyAuditIntegrity checks that all envelopes have complete, valid audit trails.
//
// Bulk-mode wrapper around VerifyEnvelopeIntegrity. Preserved verbatim
// in external behaviour: returns the first error encountered, with the
// same "envelope %s: …" message format that prior callers and tests
// depend on. Repository / hash-compute errors are surfaced with the
// existing wraps; integrity findings are converted back to errors
// formatted the same way the prior implementation produced them.
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

// verifyEnvelope adapts VerifyEnvelopeIntegrity back to the bulk
// function's "first error wins" contract. Preserves the exact
// "envelope %s: <body>" message format the original implementation
// emitted, so the existing TestVerifyAuditIntegrity_* substring
// assertions continue to hold.
func verifyEnvelope(ctx context.Context, auditRepo AuditRepository, env *envelope.Envelope) error {
	result, err := VerifyEnvelopeIntegrity(ctx, auditRepo, env)
	if err != nil {
		// Repository / hash-compute error; the helper already wrapped
		// the underlying cause with the prefix the original code used
		// ("listing events: …" or "failed to compute hash at sequence %d: …").
		return fmt.Errorf("envelope %s: %w", env.ID(), err)
	}
	if !result.Valid {
		return fmt.Errorf("envelope %s: %s", env.ID(), result.ErrorMessage)
	}
	return nil
}
