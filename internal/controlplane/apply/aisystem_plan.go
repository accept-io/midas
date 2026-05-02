package apply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// ---------------------------------------------------------------------------
// AISystem (Epic 1, PR 2)
// ---------------------------------------------------------------------------

// planAISystemEntry inspects the persisted state for an AISystem document and
// sets the entry action.
//
// Posture: AISystem is status-honouring (mirrors BusinessService, Agent). The
// apply framework currently has no Update action — existing rows therefore
// produce Conflict, mirroring BusinessService's posture. The
// AISystemRepository.Update method exists for a future framework-level
// follow-up that introduces ApplyActionUpdate.
//
// Cross-reference: spec.replaces, when set, must resolve in the bundle or
// in the persisted store. The validator already rejected self-replace.
func (s *Service) planAISystemEntry(
	ctx context.Context,
	doc parser.ParsedDocument,
	bundleAISystemIDs map[string]struct{},
	entry *ApplyPlanEntry,
) {
	aiDoc, ok := doc.Doc.(types.AISystemDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not an AISystemDocument",
		})
		return
	}

	// Cross-reference: spec.replaces must resolve.
	if replaces := strings.TrimSpace(aiDoc.Spec.Replaces); replaces != "" {
		if _, inBundle := bundleAISystemIDs[replaces]; !inBundle {
			if s.aiSystemRepo == nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.replaces",
					Message: "replaces validation unavailable: AISystemRepository not configured",
				})
				return
			}
			exists, err := s.aiSystemRepo.Exists(ctx, replaces)
			if err != nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.replaces",
					Message: "repository error checking ai system existence: " + err.Error(),
				})
				return
			}
			if !exists {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.replaces",
					Message: fmt.Sprintf("predecessor ai system %q does not exist", replaces),
				})
				return
			}
		}
	}

	if s.aiSystemRepo == nil {
		entry.Action = ApplyActionCreate
		entry.DecisionSource = DecisionSourceValidation
		entry.CreateKind = CreateKindNew
		return
	}

	existing, err := s.aiSystemRepo.GetByID(ctx, doc.ID)
	if err != nil && !errors.Is(err, aisystem.ErrAISystemNotFound) {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}

	if existing != nil {
		// Status-honouring + framework-no-Update: existing rows are
		// reported as Conflict (mirrors BusinessService). When the
		// framework gains ApplyActionUpdate, this branch will return
		// Update for description / status / owner edits.
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"ai system %q already exists; ai systems are immutable in the apply path until the framework gains ApplyActionUpdate",
			doc.ID,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// ---------------------------------------------------------------------------
// AISystemVersion (Epic 1, PR 2)
// ---------------------------------------------------------------------------

// planAISystemVersionEntry inspects the persisted state for an
// AISystemVersion document and sets the entry action.
//
// AISystemVersion uses an explicit-version posture (deliberate divergence
// from Surface/Profile, which auto-increment): the bundle declares the
// version number directly, and a duplicate (ai_system_id, version) tuple
// is a Conflict. There is no version-bump logic.
//
// Cross-reference: spec.ai_system_id must resolve in the bundle or store.
func (s *Service) planAISystemVersionEntry(
	ctx context.Context,
	doc parser.ParsedDocument,
	bundleAISystemIDs map[string]struct{},
	entry *ApplyPlanEntry,
) {
	vDoc, ok := doc.Doc.(types.AISystemVersionDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not an AISystemVersionDocument",
		})
		return
	}

	aiSystemID := strings.TrimSpace(vDoc.Spec.AISystemID)
	if !s.checkAISystemExists(ctx, doc, aiSystemID, bundleAISystemIDs, entry, "spec.ai_system_id") {
		return
	}

	if s.aiVersionRepo == nil {
		entry.Action = ApplyActionCreate
		entry.DecisionSource = DecisionSourceValidation
		entry.CreateKind = CreateKindNew
		entry.NewVersion = vDoc.Spec.Version
		return
	}

	existing, err := s.aiVersionRepo.GetByIDAndVersion(ctx, aiSystemID, vDoc.Spec.Version)
	if err != nil && !errors.Is(err, aisystem.ErrAISystemVersionNotFound) {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"ai system version %q v%d already exists; versions are immutable on the (id, version) tuple",
			aiSystemID, vDoc.Spec.Version,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
	entry.NewVersion = vDoc.Spec.Version
}

// ---------------------------------------------------------------------------
// AISystemBinding (Epic 1, PR 2)
// ---------------------------------------------------------------------------

// planAISystemBindingEntry inspects bundle peers and persisted state to
// decide the apply action for an AISystemBinding document. Mirrors the
// BSR junction posture: ID-collision → Conflict, otherwise Create.
//
// Implements rules 2–5 of the five cross-reference rules (rule 1 —
// at-least-one-context — is in the validator):
//
//  2. (surface_id, process_id) consistency: surfaces.process_id must
//     equal binding.process_id.
//  3. (process_id, business_service_id) consistency: processes.business_service_id
//     must equal binding.business_service_id.
//  4. (business_service_id, capability_id) consistency: BSC must link them.
//  5. (process_id, capability_id) consistency: BSC must link the
//     process's business_service to the binding's capability.
//
// And the (ai_system_id, version) FK rule:
//
//   - When ai_system_version is supplied, the (ai_system_id, version)
//     tuple must exist in the bundle or in the persisted store.
//
// Bundle resolution is preferred to persisted lookup (matches BSR/BSC).
//
// Posture: AISystemBinding is an immediate-apply junction. No audit
// emission. Existing rows by metadata.id are Conflict (mirrors BSR/BSC).
func (s *Service) planAISystemBindingEntry(
	ctx context.Context,
	doc parser.ParsedDocument,
	bundleAISystemIDs map[string]struct{},
	bundleAISystemVersionPairs map[string]struct{},
	bundleBusinessServiceIDs map[string]struct{},
	bundleCapabilityIDs map[string]struct{},
	bundleProcessIDs map[string]struct{},
	bundleSurfaceIDs map[string]struct{},
	bundleSurfaceProcessIDs map[string]string,
	bundleProcessBusinessServiceIDs map[string]string,
	bundleBSCPairs map[string]int,
	entry *ApplyPlanEntry,
) {
	bDoc, ok := doc.Doc.(types.AISystemBindingDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not an AISystemBindingDocument",
		})
		return
	}

	aiSystemID := strings.TrimSpace(bDoc.Spec.AISystemID)
	bsID := strings.TrimSpace(bDoc.Spec.BusinessServiceID)
	capID := strings.TrimSpace(bDoc.Spec.CapabilityID)
	procID := strings.TrimSpace(bDoc.Spec.ProcessID)
	surfID := strings.TrimSpace(bDoc.Spec.SurfaceID)

	// (a) ai_system_id existence.
	if !s.checkAISystemExists(ctx, doc, aiSystemID, bundleAISystemIDs, entry, "spec.ai_system_id") {
		return
	}

	// (b) ai_system_version FK to (ai_system_id, version) when supplied.
	if bDoc.Spec.AISystemVersion != nil {
		v := *bDoc.Spec.AISystemVersion
		key := fmt.Sprintf("%s\x00%d", aiSystemID, v)
		if _, inBundle := bundleAISystemVersionPairs[key]; !inBundle {
			if s.aiVersionRepo == nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.ai_system_version",
					Message: "ai_system_version validation unavailable: AISystemVersionRepository not configured",
				})
				return
			}
			ver, err := s.aiVersionRepo.GetByIDAndVersion(ctx, aiSystemID, v)
			if err != nil && !errors.Is(err, aisystem.ErrAISystemVersionNotFound) {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.ai_system_version",
					Message: "repository error checking ai system version existence: " + err.Error(),
				})
				return
			}
			if ver == nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.ai_system_version",
					Message: fmt.Sprintf("ai system version (%q, v%d) does not exist", aiSystemID, v),
				})
				return
			}
		}
	}

	// (c) Per-context-field existence.
	if bsID != "" && !s.checkBusinessServiceExists(ctx, doc, bsID, bundleBusinessServiceIDs, entry) {
		return
	}
	if capID != "" && !s.checkCapabilityExists(ctx, doc, capID, bundleCapabilityIDs, entry) {
		return
	}
	if procID != "" && !s.checkProcessExists(ctx, doc, procID, bundleProcessIDs, entry) {
		return
	}
	if surfID != "" {
		if _, inBundle := bundleSurfaceIDs[surfID]; !inBundle {
			if s.surfaceRepo == nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.surface_id",
					Message: "surface_id validation unavailable: SurfaceRepository not configured",
				})
				return
			}
			latest, err := s.surfaceRepo.FindLatestByID(ctx, surfID)
			if err != nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.surface_id",
					Message: "repository error checking surface existence: " + err.Error(),
				})
				return
			}
			if latest == nil {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourcePersistedState
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:    doc.Kind,
					ID:      doc.ID,
					Field:   "spec.surface_id",
					Message: fmt.Sprintf("surface_id %q does not exist", surfID),
				})
				return
			}
		}
	}

	// Rule 2: (surface_id, process_id) consistency. The surface's
	// process_id must equal the binding's process_id when both are set.
	if surfID != "" && procID != "" {
		surfaceProcessID, err := s.resolveSurfaceProcessID(ctx, surfID, bundleSurfaceProcessIDs)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.surface_id",
				Message: "repository error resolving surface.process_id: " + err.Error(),
			})
			return
		}
		if surfaceProcessID != "" && surfaceProcessID != procID {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.process_id",
				Message: fmt.Sprintf("process_id %q does not match surface %q's process_id %q", procID, surfID, surfaceProcessID),
			})
			return
		}
	}

	// Rule 3: (process_id, business_service_id) consistency. The process's
	// business_service_id must equal the binding's business_service_id.
	if procID != "" && bsID != "" {
		procBSID, err := s.resolveProcessBusinessServiceID(ctx, procID, bundleProcessBusinessServiceIDs)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.process_id",
				Message: "repository error resolving process.business_service_id: " + err.Error(),
			})
			return
		}
		if procBSID != "" && procBSID != bsID {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.business_service_id",
				Message: fmt.Sprintf("business_service_id %q does not match process %q's business_service_id %q", bsID, procID, procBSID),
			})
			return
		}
	}

	// Rule 4: (business_service_id, capability_id) consistency. A
	// business_service_capabilities row must link the two.
	if bsID != "" && capID != "" {
		linked, err := s.bscPairLinked(ctx, bsID, capID, bundleBSCPairs)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.capability_id",
				Message: "repository error checking business_service_capability link: " + err.Error(),
			})
			return
		}
		if !linked {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.capability_id",
				Message: fmt.Sprintf("capability %q is not linked to business service %q via business_service_capabilities", capID, bsID),
			})
			return
		}
	}

	// Rule 5: (process_id, capability_id) consistency. The capability must
	// be linked through the process's business service. Implemented as
	// "resolve process's BS, then check BSC(process.BS, capability)".
	if procID != "" && capID != "" && bsID == "" {
		procBSID, err := s.resolveProcessBusinessServiceID(ctx, procID, bundleProcessBusinessServiceIDs)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.process_id",
				Message: "repository error resolving process.business_service_id: " + err.Error(),
			})
			return
		}
		if procBSID == "" {
			// Process exists in bundle but its BS is unresolved — the planner
			// for the Process entry will have already flagged this.
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.process_id",
				Message: fmt.Sprintf("process %q has no resolvable business_service_id; rule 5 cannot be checked", procID),
			})
			return
		}
		linked, err := s.bscPairLinked(ctx, procBSID, capID, bundleBSCPairs)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.capability_id",
				Message: "repository error checking transitive business_service_capability link: " + err.Error(),
			})
			return
		}
		if !linked {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.capability_id",
				Message: fmt.Sprintf("capability %q is not linked to process %q's business service %q via business_service_capabilities", capID, procID, procBSID),
			})
			return
		}
	}

	// ID-collision detection: existing binding with the same metadata.id.
	if s.aiBindingRepo == nil {
		entry.Action = ApplyActionCreate
		entry.DecisionSource = DecisionSourceValidation
		entry.CreateKind = CreateKindNew
		return
	}
	existing, err := s.aiBindingRepo.GetByID(ctx, doc.ID)
	if err != nil && !errors.Is(err, aisystem.ErrAISystemBindingNotFound) {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}
	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"ai system binding %q already exists; junction rows are immutable in the apply path",
			doc.ID,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// checkAISystemExists is a planner helper for ai_system_id cross-references.
// Mirrors checkBusinessServiceExists in shape: bundle map first, repo
// fallback, structured error on the requested field path. Used by both
// AISystemVersion (spec.ai_system_id) and AISystemBinding (spec.ai_system_id).
func (s *Service) checkAISystemExists(
	ctx context.Context,
	doc parser.ParsedDocument,
	aiSystemID string,
	bundleAISystemIDs map[string]struct{},
	entry *ApplyPlanEntry,
	field string,
) bool {
	if aiSystemID == "" {
		// Validator already emitted "required" — defensive guard.
		return false
	}
	if _, inBundle := bundleAISystemIDs[aiSystemID]; inBundle {
		return true
	}
	if s.aiSystemRepo == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   field,
			Message: "ai_system_id validation unavailable: AISystemRepository not configured",
		})
		return false
	}
	exists, err := s.aiSystemRepo.Exists(ctx, aiSystemID)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   field,
			Message: "repository error checking ai system existence: " + err.Error(),
		})
		return false
	}
	if !exists {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   field,
			Message: fmt.Sprintf("ai_system_id %q does not exist", aiSystemID),
		})
		return false
	}
	return true
}

// resolveSurfaceProcessID returns the process_id of the surface with the
// given logical ID, preferring the bundle map and falling back to the
// persisted store. Returns "" with nil error when the surface is not
// resolvable (caller treats this as "rule does not apply").
func (s *Service) resolveSurfaceProcessID(
	ctx context.Context,
	surfaceID string,
	bundleSurfaceProcessIDs map[string]string,
) (string, error) {
	if pid, ok := bundleSurfaceProcessIDs[surfaceID]; ok {
		return pid, nil
	}
	if s.surfaceRepo == nil {
		return "", nil
	}
	latest, err := s.surfaceRepo.FindLatestByID(ctx, surfaceID)
	if err != nil {
		return "", err
	}
	if latest == nil {
		return "", nil
	}
	return latest.ProcessID, nil
}

// resolveProcessBusinessServiceID returns the business_service_id of the
// process with the given logical ID, preferring the bundle map and falling
// back to the persisted store.
func (s *Service) resolveProcessBusinessServiceID(
	ctx context.Context,
	processID string,
	bundleProcessBusinessServiceIDs map[string]string,
) (string, error) {
	if bs, ok := bundleProcessBusinessServiceIDs[processID]; ok {
		return bs, nil
	}
	if s.processRepo == nil {
		return "", nil
	}
	proc, err := s.processRepo.GetByID(ctx, processID)
	if err != nil {
		return "", err
	}
	if proc == nil {
		return "", nil
	}
	return proc.BusinessServiceID, nil
}

// bscPairLinked reports whether the (business_service_id, capability_id)
// pair is linked in the BSC junction. Bundle map preferred; repo fallback.
func (s *Service) bscPairLinked(
	ctx context.Context,
	businessServiceID string,
	capabilityID string,
	bundleBSCPairs map[string]int,
) (bool, error) {
	key := businessServiceID + "\x00" + capabilityID
	if _, inBundle := bundleBSCPairs[key]; inBundle {
		return true, nil
	}
	if s.businessServiceCapabilityRepo == nil {
		return false, nil
	}
	return s.businessServiceCapabilityRepo.Exists(ctx, businessServiceID, capabilityID)
}

// ---------------------------------------------------------------------------
// Executors
// ---------------------------------------------------------------------------

// applyAISystem persists an AISystem document via the apply path's Create
// branch. Emits ActionAISystemCreated to the pendingAudit buffer. AISystem
// is status-honouring; the bundle's spec.status flows through the mapper
// directly into the row. Updates are not yet supported in the framework
// (Conflict-on-existing posture); when ApplyActionUpdate lands, this
// function will gain an Update branch that emits ActionAISystemUpdated.
func (s *Service) applyAISystem(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	aiDoc, ok := doc.Doc.(types.AISystemDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindAISystem)
	}
	sys, err := mapAISystemDocumentToAISystem(aiDoc, now, actor)
	if err != nil {
		return fmt.Errorf("map ai system document: %w", err)
	}
	if err := repos.AISystems.Create(ctx, sys); err != nil {
		return fmt.Errorf("create ai system: %w", err)
	}
	result.AddCreated(doc.Kind, doc.ID)
	*pendingAudit = append(*pendingAudit, controlaudit.NewAISystemCreatedRecord(actor, sys.ID))
	return nil
}

// applyAISystemVersion persists an AISystemVersion document. Emits
// ActionAISystemVersionCreated to the pendingAudit buffer. Status is
// honoured (no review-forcing).
func (s *Service) applyAISystemVersion(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	vDoc, ok := doc.Doc.(types.AISystemVersionDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindAISystemVersion)
	}
	ver, err := mapAISystemVersionDocumentToAISystemVersion(vDoc, now, actor)
	if err != nil {
		return fmt.Errorf("map ai system version document: %w", err)
	}
	if err := repos.AISystemVersions.Create(ctx, ver); err != nil {
		return fmt.Errorf("create ai system version: %w", err)
	}
	result.AddCreated(doc.Kind, doc.ID)
	*pendingAudit = append(*pendingAudit,
		controlaudit.NewAISystemVersionCreatedRecord(actor, ver.AISystemID, ver.Version))
	return nil
}

// applyAISystemBinding persists an AISystemBinding document. No audit
// emission — junction-style entities mirror the BSC/BSR posture.
func (s *Service) applyAISystemBinding(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	bDoc, ok := doc.Doc.(types.AISystemBindingDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindAISystemBinding)
	}
	binding, err := mapAISystemBindingDocumentToAISystemBinding(bDoc, now, actor)
	if err != nil {
		return fmt.Errorf("map ai system binding document: %w", err)
	}
	if err := repos.AISystemBindings.Create(ctx, binding); err != nil {
		return fmt.Errorf("create ai system binding: %w", err)
	}
	result.AddCreated(doc.Kind, doc.ID)
	return nil
}
