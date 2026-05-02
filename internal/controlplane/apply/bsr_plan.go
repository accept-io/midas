package apply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// planBusinessServiceRelationshipEntry inspects bundle peers and persisted
// state to decide the apply action for a BusinessServiceRelationship document
// (Epic 1, PR 1).
//
// Decision matrix:
//
//   - source/target referenced BS not present in bundle nor in store     → Invalid
//   - row with same metadata.id already exists                           → Conflict (id collision)
//   - row with same (source, target, type) triple already exists         → Conflict (triple collision)
//   - otherwise                                                          → Create (CreateKindNew)
//
// Per-document validation (field formats, self-reference, enum, etc.) and
// bundle-level triple-uniqueness are handled in the validator before this
// runs. The planner therefore only addresses cross-reference resolution
// and persisted-state collision detection.
//
// Posture: BSR is an immediate-apply junction (mirrors BSC). Existing rows
// are Conflict, not Update — the apply framework's ApplyAction enum has no
// Update value in PR 1 scope, and BSC, the closest precedent, treats
// existing junction rows as Conflict. The Description field is
// reserved as future-mutable; the BusinessServiceRelationshipRepo already
// exposes Update so a follow-up PR can wire description-update once the
// framework gains an Update action.
func (s *Service) planBusinessServiceRelationshipEntry(
	ctx context.Context,
	doc parser.ParsedDocument,
	bundleBusinessServiceIDs map[string]struct{},
	entry *ApplyPlanEntry,
) {
	bsrDoc, ok := doc.Doc.(types.BusinessServiceRelationshipDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a BusinessServiceRelationshipDocument",
		})
		return
	}

	source := strings.TrimSpace(bsrDoc.Spec.SourceBusinessServiceID)
	target := strings.TrimSpace(bsrDoc.Spec.TargetBusinessServiceID)
	relType := strings.TrimSpace(bsrDoc.Spec.RelationshipType)

	// Cross-reference resolution: source and target must exist in the
	// bundle or the persisted store. Reuses the existing helper that
	// every BS-referencing planner uses (BSC, Process). The helper sets
	// entry.Action / DecisionSource / ValidationErrors on failure.
	if !s.checkBusinessServiceExists(ctx, doc, source, bundleBusinessServiceIDs, entry) {
		return
	}
	if !s.checkBusinessServiceExists(ctx, doc, target, bundleBusinessServiceIDs, entry) {
		return
	}

	if s.bsRelationshipRepo == nil {
		// No repository wired (memory dev mode without BSR repo, or unit
		// tests that exercise the planner without persistence). The action
		// is Create and the decision source is the validator that already
		// ran upstream.
		entry.Action = ApplyActionCreate
		entry.DecisionSource = DecisionSourceValidation
		entry.CreateKind = CreateKindNew
		return
	}

	// ID-collision: a row with the same metadata.id already exists.
	if _, err := s.bsRelationshipRepo.GetByID(ctx, doc.ID); err == nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"business service relationship %q already exists; junction rows are immutable in the apply path",
			doc.ID,
		)
		return
	} else if !errors.Is(err, businessservice.ErrRelationshipNotFound) {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}

	// Triple-collision: scan ListBySourceBusinessService for the same
	// (source, target, relationship_type). The list is small in practice
	// (a typical BS has at most a handful of outgoing relationships);
	// a dedicated index would be premature.
	existing, err := s.bsRelationshipRepo.ListBySourceBusinessService(ctx, source)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}
	for _, e := range existing {
		if e.TargetBusinessService == target && e.RelationshipType == relType {
			entry.Action = ApplyActionConflict
			entry.DecisionSource = DecisionSourcePersistedState
			entry.Message = fmt.Sprintf(
				"business service relationship triple already exists: (source=%q, target=%q, relationship_type=%q) is recorded as %q",
				source, target, relType, e.ID,
			)
			return
		}
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// applyBusinessServiceRelationship persists a BusinessServiceRelationship
// document via the apply path's Create branch.
//
// No control-plane audit emission. BSR follows the BusinessServiceCapability
// posture: junction-style entities do not currently emit control-plane
// audit records. See the PR 1 Step 0.5 finding for the schema CHECK
// rationale (the controlplane_audit_events.resource_kind CHECK list does
// not include junctions).
func (s *Service) applyBusinessServiceRelationship(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	bsrDoc, ok := doc.Doc.(types.BusinessServiceRelationshipDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindBusinessServiceRelationship)
	}

	rel := mapBusinessServiceRelationshipDocumentToBusinessServiceRelationship(bsrDoc, now, actor)
	if err := repos.BusinessServiceRelationships.Create(ctx, rel); err != nil {
		return fmt.Errorf("create business service relationship: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}
