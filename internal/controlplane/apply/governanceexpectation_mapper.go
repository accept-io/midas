package apply

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/governanceexpectation"
)

// mapGovernanceExpectationDocumentToDomain converts a validated
// GovernanceExpectationDocument into a domain GovernanceExpectation
// ready for persistence.
//
// Key behaviours, mirroring the patterns established by Surface and
// Profile mappers:
//
//   - Status is ALWAYS set to ExpectationStatusReview, regardless of any
//     value in doc.Spec.Lifecycle.Status. The document's status is
//     accepted-but-forced; the validator has already rejected typo'd
//     values, so the field here is informational only.
//
//   - Version is supplied by the planner. 1 for first-time creates,
//     latest+1 for re-applies. The document's Lifecycle.Version is
//     informational only and does not feed into the persisted version.
//
//   - EffectiveDate falls back to now.UTC() when lifecycle.effective_from
//     is empty (matches Profile mapper exactly).
//
//   - ConditionPayload is JSON-marshalled from the YAML-decoded map.
//     A nil/empty payload becomes the canonical empty object "{}", which
//     the repos round-trip identically through #51B's payload
//     normalisation.
//
//   - All string fields are TrimSpace'd. CreatedAt/UpdatedAt are now.UTC().
//     Approval audit fields (ApprovedBy, ApprovedAt) are zero — they are
//     populated only by a future approval flow.
func mapGovernanceExpectationDocumentToDomain(
	doc types.GovernanceExpectationDocument,
	now time.Time,
	createdBy string,
	version int,
) (*governanceexpectation.GovernanceExpectation, error) {
	now = now.UTC()

	effectiveFrom := now
	if s := strings.TrimSpace(doc.Spec.Lifecycle.EffectiveFrom); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("invalid lifecycle.effective_from: %w", err)
		}
		effectiveFrom = parsed.UTC()
	}

	var effectiveUntil *time.Time
	if s := strings.TrimSpace(doc.Spec.Lifecycle.EffectiveUntil); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("invalid lifecycle.effective_until: %w", err)
		}
		utc := parsed.UTC()
		effectiveUntil = &utc
	}

	payload, err := marshalConditionPayload(doc.Spec.ConditionPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal condition_payload: %w", err)
	}

	return &governanceexpectation.GovernanceExpectation{
		ID:                strings.TrimSpace(doc.Metadata.ID),
		Version:           version,
		ScopeKind:         governanceexpectation.ScopeKind(strings.TrimSpace(doc.Spec.ScopeKind)),
		ScopeID:           strings.TrimSpace(doc.Spec.ScopeID),
		RequiredSurfaceID: strings.TrimSpace(doc.Spec.RequiredSurfaceID),
		Name:              strings.TrimSpace(doc.Metadata.Name),
		Description:       strings.TrimSpace(doc.Spec.Description),
		Status:            governanceexpectation.ExpectationStatusReview,
		EffectiveDate:     effectiveFrom,
		EffectiveUntil:    effectiveUntil,
		ConditionType:     governanceexpectation.ConditionType(strings.TrimSpace(doc.Spec.ConditionType)),
		ConditionPayload:  payload,
		BusinessOwner:     strings.TrimSpace(doc.Spec.BusinessOwner),
		TechnicalOwner:    strings.TrimSpace(doc.Spec.TechnicalOwner),
		CreatedAt:         now,
		UpdatedAt:         now,
		CreatedBy:         strings.TrimSpace(createdBy),
	}, nil
}

// marshalConditionPayload turns the YAML-decoded payload map into the
// canonical JSON bytes persisted to the JSONB column. Nil and empty maps
// produce the empty JSON object literal so reads round-trip the same
// shape produced by the schema's DEFAULT '{}' on first-create.
func marshalConditionPayload(payload map[string]any) (json.RawMessage, error) {
	if len(payload) == 0 {
		return json.RawMessage(`{}`), nil
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bytes), nil
}
