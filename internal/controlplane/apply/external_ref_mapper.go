package apply

// Shared mapper for the optional ExternalRef field that five document
// specs gained in Epic 1, PR 3.
//
// Each consuming entity mapper (mapBusinessServiceDocumentToBusinessService,
// mapBusinessServiceRelationshipDocumentToBusinessServiceRelationship,
// the three AI system mappers) calls mapExternalRefSpec once after its
// own field-mover logic and assigns the result to the entity's
// ExternalRef field.
//
// Canonicalisation: empty-but-present specs (`external_ref: {}`) and
// nil specs both map to nil. This matches the storage canonicalisation
// in Cluster A (memory and Postgres repos canonicalise IsZero refs to
// nil) and the externalref.Equal contract that nil and IsZero are
// equivalent.
//
// RFC3339 parsing: the validator has already confirmed last_synced_at
// parses cleanly when present. The mapper re-parses (cheap) and stores
// in UTC — mirrors the EffectiveFrom handling in PR 2's
// mapAISystemVersionDocumentToAISystemVersion.

import (
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/externalref"
)

// mapExternalRefSpec converts an optional types.ExternalRefSpec into a
// domain *externalref.ExternalRef. Returns nil when the spec is nil OR
// when every field is empty after whitespace trimming (canonicalising
// the empty-but-present case).
//
// Returns an error only on RFC3339 parse failure for last_synced_at,
// which the validator should have caught upstream — a returned error
// here indicates programmer error, not user input the validator missed.
func mapExternalRefSpec(spec *types.ExternalRefSpec) (*externalref.ExternalRef, error) {
	if spec == nil {
		return nil, nil
	}
	system := strings.TrimSpace(spec.SourceSystem)
	id := strings.TrimSpace(spec.SourceID)
	url := strings.TrimSpace(spec.SourceURL)
	version := strings.TrimSpace(spec.SourceVersion)
	rawTS := strings.TrimSpace(spec.LastSyncedAt)

	if system == "" && id == "" && url == "" && version == "" && rawTS == "" {
		return nil, nil
	}

	ref := &externalref.ExternalRef{
		SourceSystem:  system,
		SourceID:      id,
		SourceURL:     url,
		SourceVersion: version,
	}
	if rawTS != "" {
		t, err := time.Parse(time.RFC3339, rawTS)
		if err != nil {
			return nil, err
		}
		utc := t.UTC()
		ref.LastSyncedAt = &utc
	}
	return ref, nil
}
