package apply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
	"github.com/accept-io/midas/internal/surface"
)

// Service coordinates control-plane apply operations.
//
// Safety model — enforced collectively by the pipeline below, not by any
// single method:
//
//   - Parse runs before anything else (ApplyBundle). A parse failure
//     returns ErrInvalidBundle and cannot produce persistence.
//   - Validation runs before execute (Plan → buildApplyPlan →
//     validate.ValidateBundle). A bundle with any ApplyActionInvalid
//     entry short-circuits executePlan: no resource is persisted.
//   - Execution runs last (executePlan → runApplyLoop). It aborts on the
//     first runtime persistence error; when tx is wired, prior writes in
//     the same bundle are rolled back.
//
// The tx field controls the atomicity posture:
//
//   - tx != nil (postgres-backed production): the mutation loop runs
//     inside a transaction and atomic rollback is guaranteed.
//   - tx == nil (memory-mode dev/test, or any caller that did not wire a
//     TxRunner): the loop still aborts on the first error, but prior
//     writes remain because the repositories auto-commit. This is the
//     best guarantee available without transactional storage and is
//     documented at RepositorySet.Tx.
//
// Control audit is deliberately NOT part of the state-change transaction
// (see ADR-041b). The executor buffers audit records during the loop and
// flushes them via appendControlAudit after the transaction commits.
type Service struct {
	surfaceRepo             SurfaceRepository
	agentRepo               AgentRepository
	profileRepo             ProfileRepository
	grantRepo               GrantRepository
	processRepo             ProcessRepository
	capabilityRepo          CapabilityRepository
	businessServiceRepo     BusinessServiceRepository
	processCapabilityRepo         ProcessCapabilityRepository
	processBusinessServiceRepo    ProcessBusinessServiceRepository
	controlAuditRepo        controlaudit.Repository
	tx                      TxRunner
}

// NewService constructs a new apply service with no repositories configured.
// In this mode, apply performs validation and records all valid resources as
// created without persisting to any backing store.
func NewService() *Service {
	return &Service{}
}

// NewServiceWithRepo constructs a new apply service with a surface repository.
// Agent, Profile, and Grant documents are recorded as created without persisting.
func NewServiceWithRepo(surfaceRepo surface.SurfaceRepository) *Service {
	return &Service{
		surfaceRepo: surfaceRepo,
	}
}

// NewServiceWithRepos constructs an apply service with the full repository set.
// Each repository enables repository-backed planning and execution for its
// resource kind. Nil repository fields fall back to validation-only behaviour
// for that kind. If ControlAudit is nil, audit events are silently skipped.
func NewServiceWithRepos(repos RepositorySet) *Service {
	return &Service{
		surfaceRepo:           repos.Surfaces,
		agentRepo:             repos.Agents,
		profileRepo:           repos.Profiles,
		grantRepo:             repos.Grants,
		processRepo:           repos.Processes,
		capabilityRepo:        repos.Capabilities,
		businessServiceRepo:   repos.BusinessServices,
		processCapabilityRepo:      repos.ProcessCapabilities,
		processBusinessServiceRepo: repos.ProcessBusinessServices,
		controlAuditRepo:      repos.ControlAudit,
		tx:                    repos.Tx,
	}
}

// Plan validates a parsed bundle and returns the ApplyPlan that describes the
// intended action for each document. No persistence occurs.
//
// The returned plan is the same one Apply would execute. Callers can inspect
// every entry's Action, Message, DecisionSource, and ValidationErrors to
// understand what would happen before committing to a write.
func (s *Service) Plan(ctx context.Context, docs []parser.ParsedDocument) ApplyPlan {
	return s.buildApplyPlan(ctx, docs)
}

// PlanBundle parses a raw YAML bundle and returns the ApplyPlan without
// persisting anything.
//
// Behavior:
//   - parse failures return an error wrapping ErrInvalidBundle
//   - successfully parsed bundles always return an ApplyPlan (never nil)
//   - validation failures are represented inside the ApplyPlan entries, not as an error
func (s *Service) PlanBundle(ctx context.Context, yamlBytes []byte) (*ApplyPlan, error) {
	docs, err := parser.ParseYAMLStream(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBundle, err)
	}

	plan := s.Plan(ctx, docs)
	return &plan, nil
}

// PlanResultFromPlan converts an ApplyPlan into a types.PlanResult suitable
// for serialisation in a dry-run HTTP response. The two types are kept separate
// so that the apply package does not export the HTTP wire format and the types
// package does not depend on the apply package.
func PlanResultFromPlan(plan ApplyPlan) types.PlanResult {
	result := types.PlanResult{}

	for _, e := range plan.Entries {
		pe := types.PlanEntry{
			Kind:             e.Kind,
			ID:               e.ID,
			Action:           types.PlanEntryAction(e.Action),
			DocumentIndex:    e.DocumentIndex,
			Message:          e.Message,
			DecisionSource:   types.PlanEntryDecisionSource(e.DecisionSource),
			ValidationErrors: e.ValidationErrors,
			Warnings:         mapPlanWarnings(e.Warnings),
		}
		// create_kind and diff are only meaningful for create entries.
		if e.Action == ApplyActionCreate {
			pe.CreateKind = string(e.CreateKind)
			pe.Diff = mapPlanDiff(e.Diff)
		}
		result.Entries = append(result.Entries, pe)

		switch e.Action {
		case ApplyActionCreate:
			result.CreateCount++
		case ApplyActionConflict:
			result.ConflictCount++
		case ApplyActionInvalid:
			result.InvalidCount++
		}
	}

	result.WouldApply = result.InvalidCount == 0 && result.CreateCount > 0
	return result
}

// mapPlanWarnings converts internal PlanWarning values to their wire
// equivalents. Returns nil when the input is empty so the serialised
// "warnings" field is omitted entirely.
func mapPlanWarnings(ws []PlanWarning) []types.PlanWarning {
	if len(ws) == 0 {
		return nil
	}
	out := make([]types.PlanWarning, 0, len(ws))
	for _, w := range ws {
		out = append(out, types.PlanWarning{
			Code:        string(w.Code),
			Severity:    string(w.Severity),
			Message:     w.Message,
			Field:       w.Field,
			RelatedKind: w.RelatedKind,
			RelatedID:   w.RelatedID,
		})
	}
	return out
}

// mapPlanDiff converts an internal PlanDiff to the wire type. Returns nil
// when the input is nil so the serialised "diff" field is omitted.
func mapPlanDiff(d *PlanDiff) *types.PlanDiff {
	if d == nil {
		return nil
	}
	out := &types.PlanDiff{
		Fields: make([]types.PlanFieldDiff, 0, len(d.Fields)),
	}
	for _, f := range d.Fields {
		out.Fields = append(out.Fields, types.PlanFieldDiff{
			Field:  f.Field,
			Before: f.Before,
			After:  f.After,
		})
	}
	return out
}

// Apply validates a parsed bundle and applies it.
//
// Apply builds the plan via Plan and then executes it. No planning logic is
// duplicated here.
//
// Invariant: validation runs first. Any ApplyActionInvalid entry in the
// plan causes executePlan to short-circuit with no persistence — the
// pipeline is strictly plan-then-execute, not interleaved. See the
// Service type doc for the full safety model.
//
// Behavior:
//   - if validation fails, return validation errors only; no resources are persisted
//   - if a surface repository is configured, the planner inspects persisted state to
//     determine whether each Surface document should be created or is a conflict
//   - if agent, profile, or grant repositories are configured, the planner inspects
//     persisted state to determine whether each document should be created or is a conflict
//   - conflict entries are not persisted; they are reported in the result
//   - if no repositories are configured, all valid resources are recorded as created
//     without persistence (validation-only mode)
//
// actor identifies who initiated the apply. It is recorded in control-plane audit
// entries for each successfully persisted resource. Use ApplyBundle when parsing
// a raw YAML bundle; actor is extracted from the X-MIDAS-ACTOR request header.
func (s *Service) Apply(ctx context.Context, docs []parser.ParsedDocument, actor string) types.ApplyResult {
	plan := s.Plan(ctx, docs)
	return s.executePlan(ctx, plan, actor)
}

// ApplyBundle parses a raw YAML bundle and applies it through the validation
// and apply pipeline.
//
// Invariant: parse runs before anything else. A parse failure returns
// an error wrapping ErrInvalidBundle and cannot produce any persistence —
// the executor is never invoked. This is the first safety boundary of
// the pipeline described on the Service type.
//
// actor identifies who initiated the apply (e.g. from the X-MIDAS-ACTOR header).
// If empty, "system" is used as a fallback actor in audit entries.
//
// Behavior:
//   - parse failures return an error wrapping ErrInvalidBundle
//   - successfully parsed bundles always return an ApplyResult
//   - validation failures are represented inside ApplyResult, not as an error
func (s *Service) ApplyBundle(ctx context.Context, yamlBytes []byte, actor string) (*types.ApplyResult, error) {
	docs, err := parser.ParseYAMLStream(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBundle, err)
	}

	result := s.Apply(ctx, docs, actor)
	return &result, nil
}

// buildApplyPlan validates the bundle and produces an ApplyPlan describing the
// intended action for each document. No persistence occurs in this phase.
//
// Planning proceeds in two sub-phases:
//
//  1. Resource-local planning — each document is validated and its persisted
//     state inspected to determine whether to create or conflict.
//
//  2. Bundle-level referential integrity — cross-document references are
//     resolved. A referenced ID is considered satisfied when it is present in
//     persisted state OR when the same bundle contains a non-invalid,
//     non-conflict entry that will create it. Entries whose references cannot be
//     resolved are marked invalid with a referential integrity error.
//
// Resource-local planning rules:
//
// Surface: the versioning model creates a new governed version on every valid
// submission. A conflict is detected when the latest persisted version is already
// in review status, because applying again before the pending governance review
// is resolved would create an ambiguous state. Unchanged is not supported for
// surfaces: every valid apply enters a new version into the governance pipeline.
//
// Agent: agents are identified by ID and are immutable once created in the
// apply path. A conflict is detected when an agent with the same ID already
// exists in persisted state. Unchanged is not supported: the document schema
// does not carry enough field parity with the domain model to prove equality.
//
// Profile: profiles are identified by ID and are immutable once created in
// the apply path. A conflict is detected when a profile with the same ID
// already exists. Unchanged is not supported for the same reason as Agent.
//
// Grant: grants are identified by ID and are immutable once created in the
// apply path. A conflict is detected when a grant with the same ID already
// exists. Unchanged is not supported for the same reason as Agent.
//
// For all kinds: if the repository lookup fails, the entry is marked invalid
// so that no silent data loss can occur.
func (s *Service) buildApplyPlan(ctx context.Context, docs []parser.ParsedDocument) ApplyPlan {
	var plan ApplyPlan

	validationErrs := validate.ValidateBundle(docs)

	// Index validation errors by (kind, id) so they can be attached to entries.
	type errKey struct{ kind, id string }
	errsByDoc := make(map[errKey][]types.ValidationError)
	for _, ve := range validationErrs {
		k := errKey{ve.Kind, ve.ID}
		errsByDoc[k] = append(errsByDoc[k], ve)
	}

	// Pre-pass: collect the IDs of Capability, Process, and BusinessService
	// documents in this bundle so that cross-document references can be
	// satisfied without a round-trip to the repository.
	//
	// bundleProcessCapabilityIDs maps process ID → capability_id for all
	// Process documents in the bundle, enabling same-capability hierarchy
	// checks without a repo round-trip when parent and child are co-bundled.
	//
	// bundleCapabilityParentIDs maps capability ID → parent_capability_id
	// (empty string when no parent) for all Capability documents in the bundle,
	// enabling bundle-aware cycle detection for the Capability hierarchy.
	//
	// bundleProcessParentIDs maps process ID → parent_process_id (empty string
	// when no parent) for all Process documents in the bundle, enabling
	// bundle-aware cycle detection for the Process hierarchy.
	bundleCapabilityIDs := make(map[string]struct{})
	bundleProcessIDs := make(map[string]struct{})
	bundleBusinessServiceIDs := make(map[string]struct{})
	bundleProcessCapabilityIDs := make(map[string]string)
	bundleCapabilityParentIDs := make(map[string]string)
	bundleProcessParentIDs := make(map[string]string)
	// bundlePCLinks tracks "processID|capabilityID" pairs for all ProcessCapability
	// documents in the bundle, enabling G-10 Option A enforcement: a Process's
	// primary capability must also appear as a ProcessCapability link in the same bundle.
	bundlePCLinks := make(map[string]struct{})
	for _, doc := range docs {
		switch doc.Kind {
		case types.KindCapability:
			if doc.ID != "" {
				bundleCapabilityIDs[doc.ID] = struct{}{}
				if capDoc, ok := doc.Doc.(types.CapabilityDocument); ok {
					bundleCapabilityParentIDs[doc.ID] = strings.TrimSpace(capDoc.Spec.ParentCapabilityID)
				}
			}
		case types.KindProcess:
			if doc.ID != "" {
				bundleProcessIDs[doc.ID] = struct{}{}
				if procDoc, ok := doc.Doc.(types.ProcessDocument); ok {
					bundleProcessCapabilityIDs[doc.ID] = procDoc.Spec.CapabilityID
					bundleProcessParentIDs[doc.ID] = strings.TrimSpace(procDoc.Spec.ParentProcessID)
				}
			}
		case types.KindBusinessService:
			if doc.ID != "" {
				bundleBusinessServiceIDs[doc.ID] = struct{}{}
			}
		case types.KindProcessCapability:
			if pcDoc, ok := doc.Doc.(types.ProcessCapabilityDocument); ok {
				key := strings.TrimSpace(pcDoc.Spec.ProcessID) + "|" + strings.TrimSpace(pcDoc.Spec.CapabilityID)
				bundlePCLinks[key] = struct{}{}
			}
		}
	}

	for i, doc := range docs {
		entry := ApplyPlanEntry{
			Kind:          doc.Kind,
			ID:            doc.ID,
			DocumentIndex: i + 1,
			Doc:           doc,
		}

		k := errKey{doc.Kind, doc.ID}
		if errs, invalid := errsByDoc[k]; invalid {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourceValidation
			entry.ValidationErrors = errs
		} else if denied, missingPerm := authorizeKind(ctx, doc.Kind); denied {
			// Fine-grained per-document authorization. Callers holding
			// controlplane:apply may still lack a specific <kind>:write
			// permission; those documents are marked invalid with the
			// required permission named in the validation error, so the
			// rest of the bundle continues to be planned and the operator
			// sees every denial in one response. No new rejection path is
			// introduced — the existing invalid-entry channel carries the
			// signal.
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourceValidation
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:          doc.Kind,
				ID:            doc.ID,
				Message:       fmt.Sprintf("caller lacks permission %q required to write documents of kind %q", missingPerm, doc.Kind),
				DocumentIndex: entry.DocumentIndex,
			})
		} else {
			switch doc.Kind {
			case types.KindBusinessService:
				if s.businessServiceRepo != nil {
					s.planBusinessServiceEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindCapability:
				if s.capabilityRepo != nil {
					s.planCapabilityEntry(ctx, doc, bundleCapabilityIDs, bundleCapabilityParentIDs, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindProcess:
				if s.processRepo != nil {
					s.planProcessEntry(ctx, doc, bundleCapabilityIDs, bundleBusinessServiceIDs, bundleProcessCapabilityIDs, bundleProcessParentIDs, bundlePCLinks, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindProcessCapability:
				if s.processCapabilityRepo != nil {
					s.planProcessCapabilityEntry(ctx, doc, bundleProcessIDs, bundleCapabilityIDs, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindProcessBusinessService:
				if s.processBusinessServiceRepo != nil {
					s.planProcessBusinessServiceEntry(ctx, doc, bundleProcessIDs, bundleBusinessServiceIDs, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindSurface:
				if s.surfaceRepo != nil {
					s.planSurfaceEntry(ctx, doc, bundleProcessIDs, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindAgent:
				if s.agentRepo != nil {
					s.planAgentEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindProfile:
				if s.profileRepo != nil {
					s.planProfileEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			case types.KindGrant:
				if s.grantRepo != nil {
					s.planGrantEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
					entry.CreateKind = CreateKindNew
				}
			default:
				entry.Action = ApplyActionCreate
				entry.DecisionSource = DecisionSourceValidation
				entry.CreateKind = CreateKindNew
			}
		}

		plan.Entries = append(plan.Entries, entry)
	}

	// Phase 2: resolve cross-document referential integrity. Entries that will
	// be created in this bundle (action == create) are tracked as in-bundle
	// providers. Entries that reference IDs not resolvable from either persisted
	// state or in-bundle providers are marked invalid.
	s.resolveReferentialIntegrity(ctx, &plan)

	return plan
}

// resolveReferentialIntegrity checks cross-document references within the plan
// and marks entries invalid when their references cannot be satisfied by either
// persisted state or another entry in the same bundle that will be created.
//
// References enforced:
//   - Profile.spec.surface_id → Surface (KindSurface)
//   - Grant.spec.agent_id    → Agent   (KindAgent)
//   - Grant.spec.profile_id  → Profile (KindProfile)
//
// A reference is satisfied when:
//   - A non-invalid, non-conflict create entry with the matching (kind, id) exists
//     in the current bundle, OR
//   - A persisted resource with that ID can be located via the relevant repository.
//
// Entries that are already invalid or conflict are skipped: their dependencies
// are irrelevant because they will not be executed.
func (s *Service) resolveReferentialIntegrity(ctx context.Context, plan *ApplyPlan) {
	// Build an index of IDs that will be created within this bundle, keyed by
	// resource kind. Only create-action entries contribute: invalid and conflict
	// entries do not produce persisted resources and cannot satisfy references.
	bundleCreates := make(map[refKey]struct{})
	for _, e := range plan.Entries {
		if e.Action == ApplyActionCreate {
			bundleCreates[refKey{e.Kind, e.ID}] = struct{}{}
		}
	}

	for i := range plan.Entries {
		entry := &plan.Entries[i]

		// Only check entries that are scheduled to be created. Invalid and
		// conflict entries will not be executed and their references are moot.
		if entry.Action != ApplyActionCreate {
			continue
		}

		switch entry.Kind {
		case types.KindProfile:
			profileDoc, ok := entry.Doc.Doc.(types.ProfileDocument)
			if !ok {
				continue
			}
			surfaceID := strings.TrimSpace(profileDoc.Spec.SurfaceID)
			if surfaceID == "" {
				// Structural validation already caught this; skip.
				continue
			}
			satisfied, refSource := s.resolveRefSource(ctx, refKey{types.KindSurface, surfaceID}, bundleCreates)
			if !satisfied {
				entry.Action = ApplyActionInvalid
				entry.DecisionSource = DecisionSourceValidation
				entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
					Kind:          entry.Kind,
					ID:            entry.ID,
					Field:         "spec.surface_id",
					Message:       fmt.Sprintf("%v: profile %q references surface %q which does not exist in persisted state or in this bundle", ErrReferentialIntegrity, entry.ID, surfaceID),
					DocumentIndex: entry.DocumentIndex,
				})
			} else {
				if refSource == DecisionSourceBundleDependency {
					// Reference was satisfied by a same-bundle create entry.
					entry.DecisionSource = DecisionSourceBundleDependency
				}
				// Terminal-target warning: only applies when the reference
				// was resolved against persisted state. A same-bundle
				// provider (bundle_dependency) is a new version created in
				// the same bundle and is not terminal.
				if refSource == DecisionSourcePersistedState {
					s.warnIfSurfaceTerminal(ctx, entry, surfaceID, "spec.surface_id")
				}
			}

		case types.KindGrant:
			grantDoc, ok := entry.Doc.Doc.(types.GrantDocument)
			if !ok {
				continue
			}
			agentID := strings.TrimSpace(grantDoc.Spec.AgentID)
			profileID := strings.TrimSpace(grantDoc.Spec.ProfileID)

			if agentID != "" {
				agentSatisfied, agentRefSource := s.resolveRefSource(ctx, refKey{types.KindAgent, agentID}, bundleCreates)
				if !agentSatisfied {
					entry.Action = ApplyActionInvalid
					entry.DecisionSource = DecisionSourceValidation
					entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
						Kind:          entry.Kind,
						ID:            entry.ID,
						Field:         "spec.agent_id",
						Message:       fmt.Sprintf("%v: grant %q references agent %q which does not exist in persisted state or in this bundle", ErrReferentialIntegrity, entry.ID, agentID),
						DocumentIndex: entry.DocumentIndex,
					})
				} else if agentRefSource == DecisionSourceBundleDependency {
					entry.DecisionSource = DecisionSourceBundleDependency
				}
			}

			// Continue accumulating errors even if the agent check already
			// marked the entry invalid: both missing-agent and missing-profile
			// should be reported together.
			if profileID != "" {
				profileSatisfied, profileRefSource := s.resolveRefSource(ctx, refKey{types.KindProfile, profileID}, bundleCreates)
				if !profileSatisfied {
					entry.Action = ApplyActionInvalid
					entry.DecisionSource = DecisionSourceValidation
					entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
						Kind:          entry.Kind,
						ID:            entry.ID,
						Field:         "spec.profile_id",
						Message:       fmt.Sprintf("%v: grant %q references profile %q which does not exist in persisted state or in this bundle", ErrReferentialIntegrity, entry.ID, profileID),
						DocumentIndex: entry.DocumentIndex,
					})
				} else {
					if profileRefSource == DecisionSourceBundleDependency && entry.Action == ApplyActionCreate {
						entry.DecisionSource = DecisionSourceBundleDependency
					}
					if profileRefSource == DecisionSourcePersistedState {
						s.warnIfProfileTerminal(ctx, entry, profileID, "spec.profile_id")
					}
				}
			}
		}
	}
}

// warnIfSurfaceTerminal attaches a REF_SURFACE_TERMINAL warning to entry
// when the referenced Surface exists in persisted state and its latest
// version is deprecated or retired. No-op when the surface repo is not
// wired or the lookup fails — an unverifiable reference is already
// handled by resolveRefSource's permissive path, and the warning is
// advisory only.
func (s *Service) warnIfSurfaceTerminal(ctx context.Context, entry *ApplyPlanEntry, surfaceID, field string) {
	if s.surfaceRepo == nil {
		return
	}
	ds, err := s.surfaceRepo.FindLatestByID(ctx, surfaceID)
	if err != nil || ds == nil {
		return
	}
	if !isSurfaceStatusTerminal(ds.Status) {
		return
	}
	entry.AddWarning(PlanWarning{
		Code:        WarningRefSurfaceTerminal,
		Severity:    WarningSeverityWarning,
		Message:     fmt.Sprintf("referenced surface %q latest version is %s; referrer will be linked to a terminal-state surface", surfaceID, ds.Status),
		Field:       field,
		RelatedKind: types.KindSurface,
		RelatedID:   surfaceID,
	})
}

// warnIfProfileTerminal attaches a REF_PROFILE_TERMINAL warning to entry
// when the referenced Profile exists in persisted state and its latest
// version is deprecated or retired. No-op when the profile repo is not
// wired or the lookup fails.
func (s *Service) warnIfProfileTerminal(ctx context.Context, entry *ApplyPlanEntry, profileID, field string) {
	if s.profileRepo == nil {
		return
	}
	p, err := s.profileRepo.FindByID(ctx, profileID)
	if err != nil || p == nil {
		return
	}
	if !isProfileStatusTerminal(p.Status) {
		return
	}
	entry.AddWarning(PlanWarning{
		Code:        WarningRefProfileTerminal,
		Severity:    WarningSeverityWarning,
		Message:     fmt.Sprintf("referenced profile %q latest version is %s; referrer will be linked to a terminal-state profile", profileID, p.Status),
		Field:       field,
		RelatedKind: types.KindProfile,
		RelatedID:   profileID,
	})
}

// refKey is a composite key used to index resources by kind and ID during
// bundle-level referential integrity resolution.
type refKey struct {
	kind string
	id   string
}

// resolveRefSource reports whether a reference with the given kind and ID is
// satisfiable, and returns the DecisionSource that explains the resolution.
//
// Resolution order:
//  1. A create entry for that (kind, id) pair is present in the same bundle —
//     returns (true, DecisionSourceBundleDependency).
//  2. The repository for that kind is configured and confirms the resource
//     exists in persisted state — returns (true, DecisionSourcePersistedState).
//  3. The repository for the referenced kind is not configured; the reference
//     cannot be verified so it is allowed to pass — returns
//     (true, DecisionSourcePersistedState) to avoid false negatives.
//
// Full enforcement is only possible when all relevant repositories are wired.
func (s *Service) resolveRefSource(ctx context.Context, key refKey, bundleCreates map[refKey]struct{}) (satisfied bool, source DecisionSource) {
	// A create entry in the same bundle satisfies the reference.
	if _, ok := bundleCreates[key]; ok {
		return true, DecisionSourceBundleDependency
	}

	// Fall back to repository lookup for persisted existence. If no repository
	// is available for this kind we cannot determine whether the resource exists,
	// so we allow the reference to pass rather than produce a false negative.
	switch key.kind {
	case types.KindSurface:
		if s.surfaceRepo == nil {
			return true, DecisionSourcePersistedState // cannot verify; allow
		}
		ds, err := s.surfaceRepo.FindLatestByID(ctx, key.id)
		return err == nil && ds != nil, DecisionSourcePersistedState

	case types.KindAgent:
		if s.agentRepo == nil {
			return true, DecisionSourcePersistedState // cannot verify; allow
		}
		a, err := s.agentRepo.GetByID(ctx, key.id)
		return err == nil && a != nil, DecisionSourcePersistedState

	case types.KindProfile:
		if s.profileRepo == nil {
			return true, DecisionSourcePersistedState // cannot verify; allow
		}
		p, err := s.profileRepo.FindByID(ctx, key.id)
		return err == nil && p != nil, DecisionSourcePersistedState
	}

	return true, DecisionSourcePersistedState // unknown kind; allow
}

// resolveRef is a convenience wrapper around resolveRefSource that discards the
// DecisionSource. Use resolveRefSource directly when the caller needs to know
// how the reference was satisfied.
func (s *Service) resolveRef(ctx context.Context, key refKey, bundleCreates map[refKey]struct{}) bool {
	ok, _ := s.resolveRefSource(ctx, key, bundleCreates)
	return ok
}

// planSurfaceEntry inspects the current persisted state for a Surface document
// and sets the entry action accordingly. If the repository lookup fails, the
// entry is set to ApplyActionInvalid so that no silent data loss can occur.
func (s *Service) planSurfaceEntry(ctx context.Context, doc parser.ParsedDocument, bundleProcessIDs map[string]struct{}, entry *ApplyPlanEntry) {
	surfaceDoc, ok := doc.Doc.(types.SurfaceDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a SurfaceDocument",
		})
		return
	}

	latest, err := s.surfaceRepo.FindLatestByID(ctx, surfaceDoc.Metadata.ID)
	if err != nil {
		// A repository error during planning means we cannot safely determine
		// the action. Mark invalid rather than proceeding with a possibly
		// incorrect create.
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "repository error during planning: " + err.Error(),
		})
		return
	}

	if latest == nil {
		// No existing version: this is a new surface.
		// Check process existence before committing to create.
		if !s.checkProcessExists(ctx, doc, surfaceDoc.Spec.ProcessID, bundleProcessIDs, entry) {
			return
		}
		entry.Action = ApplyActionCreate
		entry.DecisionSource = DecisionSourcePersistedState
		entry.CreateKind = CreateKindNew
		return
	}

	if latest.Status == surface.SurfaceStatusReview {
		// A version is already pending governance review. Submitting a new
		// apply while a version is in review creates an ambiguous governance
		// state. The caller must resolve the existing review before applying
		// a replacement.
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"surface already has version %d pending governance review; resolve the existing review before applying again",
			latest.Version,
		)
		return
	}

	// Latest version is in draft, active, deprecated, or retired status.
	// The versioning model creates a new governed version in each case.
	if !s.checkProcessExists(ctx, doc, surfaceDoc.Spec.ProcessID, bundleProcessIDs, entry) {
		return
	}
	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNewVersion

	// Field-level diff against the persisted baseline (Issue #37). Diff is
	// advisory output and never blocks apply.
	if diff := computeSurfaceDiff(latest, surfaceDoc); diff != nil {
		entry.Diff = diff
	}
}

// checkProcessExists validates that process_id is present and references an
// existing process. Returns true when the check passes. Returns false and marks
// the entry invalid when:
//   - process_id is missing or empty (invariant I-1: Surface must belong to a Process)
//   - no ProcessRepository is configured (cannot validate existence)
//   - the process does not exist in the repository
//   - the repository lookup returns an error
func (s *Service) checkProcessExists(ctx context.Context, doc parser.ParsedDocument, processID string, bundleProcessIDs map[string]struct{}, entry *ApplyPlanEntry) bool {
	processID = strings.TrimSpace(processID)
	if processID == "" {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.process_id",
			Message: "process_id is required",
		})
		return false
	}
	// If the process is being created in the same bundle, the reference is satisfied.
	if _, inBundle := bundleProcessIDs[processID]; inBundle {
		return true
	}
	if s.processRepo == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.process_id",
			Message: "process_id validation unavailable: ProcessRepository not configured",
		})
		return false
	}
	// GetByID (vs Exists) lets us also inspect Status for an advisory
	// warning when the target exists but is in a terminal lifecycle state.
	// No new queries are introduced — this replaces one cheap lookup with
	// another on the same repo.
	proc, err := s.processRepo.GetByID(ctx, processID)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.process_id",
			Message: "repository error checking process existence: " + err.Error(),
		})
		return false
	}
	if proc == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.process_id",
			Message: fmt.Sprintf("process_id %q does not exist", processID),
		})
		return false
	}
	// Advisory: the referenced Process exists but is deprecated. Surface
	// the signal for reviewer attention without blocking apply.
	if isProcessStatusTerminal(proc.Status) {
		entry.AddWarning(PlanWarning{
			Code:        WarningRefProcessTerminal,
			Severity:    WarningSeverityWarning,
			Message:     fmt.Sprintf("referenced process %q is %s; referrer will be linked to a terminal-state process", processID, proc.Status),
			Field:       "spec.process_id",
			RelatedKind: types.KindProcess,
			RelatedID:   processID,
		})
	}
	return true
}

// isProcessStatusTerminal reports whether the given Process.Status value
// should raise a terminal-state warning. "Deprecated" is the only terminal
// state in the Process lifecycle; the Process model has no retired state.
func isProcessStatusTerminal(status string) bool {
	return status == "deprecated"
}

// isCapabilityStatusTerminal reports whether the given Capability.Status
// value should raise a terminal-state warning. Capabilities share the
// Process lifecycle.
func isCapabilityStatusTerminal(status string) bool {
	return status == "deprecated"
}

// isSurfaceStatusTerminal reports whether the given Surface.SurfaceStatus
// value should raise a terminal-state warning. Deprecated and retired
// surfaces both count as terminal for reference-warning purposes.
func isSurfaceStatusTerminal(status surface.SurfaceStatus) bool {
	return status == surface.SurfaceStatusDeprecated || status == surface.SurfaceStatusRetired
}

// isProfileStatusTerminal reports whether the given ProfileStatus value
// should raise a terminal-state warning.
func isProfileStatusTerminal(status authority.ProfileStatus) bool {
	return status == authority.ProfileStatusDeprecated || status == authority.ProfileStatusRetired
}

// planBusinessServiceEntry inspects the current persisted state for a BusinessService
// document and sets the entry action accordingly.
//
// BusinessServices are identified by ID and are immutable once created in the apply path.
// If a business service with the same ID already exists, the entry is marked as conflict.
// If the repository lookup fails, the entry is marked invalid.
func (s *Service) planBusinessServiceEntry(ctx context.Context, doc parser.ParsedDocument, entry *ApplyPlanEntry) {
	bsDoc, ok := doc.Doc.(types.BusinessServiceDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a BusinessServiceDocument",
		})
		return
	}

	existing, err := s.businessServiceRepo.GetByID(ctx, bsDoc.Metadata.ID)
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

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"business service %q already exists; business services are immutable once created in the apply path",
			bsDoc.Metadata.ID,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// planCapabilityEntry inspects the current persisted state for a Capability document
// and sets the entry action accordingly.
//
// Capabilities are identified by ID and are immutable once created in the apply path.
// If a capability with the same ID already exists, the entry is marked as conflict.
// If parent_capability_id is set, its existence is verified and the hierarchy is
// checked for self-parenting and cycles. If the repository lookup fails, the entry
// is marked invalid.
func (s *Service) planCapabilityEntry(ctx context.Context, doc parser.ParsedDocument, bundleCapabilityIDs map[string]struct{}, bundleCapabilityParentIDs map[string]string, entry *ApplyPlanEntry) {
	capDoc, ok := doc.Doc.(types.CapabilityDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a CapabilityDocument",
		})
		return
	}

	existing, err := s.capabilityRepo.GetByID(ctx, capDoc.Metadata.ID)
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

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"capability %q already exists; capabilities are immutable once created in the apply path",
			capDoc.Metadata.ID,
		)
		return
	}

	if parentID := strings.TrimSpace(capDoc.Spec.ParentCapabilityID); parentID != "" {
		if !s.checkCapabilityHierarchy(ctx, doc, capDoc.Metadata.ID, parentID, bundleCapabilityIDs, bundleCapabilityParentIDs, entry) {
			return
		}
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// planProcessEntry inspects the current persisted state for a Process document
// and sets the entry action accordingly.
//
// Processes are identified by ID and are immutable once created in the apply path.
// If a process with the same ID already exists, the entry is marked as conflict.
// The spec.capability_id is also validated for existence. If the repository
// lookup fails, the entry is marked invalid.
func (s *Service) planProcessEntry(ctx context.Context, doc parser.ParsedDocument, bundleCapabilityIDs map[string]struct{}, bundleBusinessServiceIDs map[string]struct{}, bundleProcessCapabilityIDs map[string]string, bundleProcessParentIDs map[string]string, bundlePCLinks map[string]struct{}, entry *ApplyPlanEntry) {
	procDoc, ok := doc.Doc.(types.ProcessDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a ProcessDocument",
		})
		return
	}

	existing, err := s.processRepo.GetByID(ctx, procDoc.Metadata.ID)
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

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"process %q already exists; processes are immutable once created in the apply path",
			procDoc.Metadata.ID,
		)
		return
	}

	if !s.checkCapabilityExists(ctx, doc, procDoc.Spec.CapabilityID, bundleCapabilityIDs, entry) {
		return
	}

	if bsID := strings.TrimSpace(procDoc.Spec.BusinessServiceID); bsID != "" {
		if !s.checkBusinessServiceExists(ctx, doc, bsID, bundleBusinessServiceIDs, entry) {
			return
		}
	}

	if parentID := strings.TrimSpace(procDoc.Spec.ParentProcessID); parentID != "" {
		if !s.checkParentProcessSameCapability(ctx, doc, parentID, strings.TrimSpace(procDoc.Spec.CapabilityID), bundleProcessCapabilityIDs, entry) {
			return
		}
		if !s.checkProcessNoCycle(ctx, doc, procDoc.Metadata.ID, parentID, bundleProcessParentIDs, entry) {
			return
		}
	}

	// G-10 Option A: a Process's primary capability must also appear as a
	// ProcessCapability link in the same bundle. This ensures that the
	// process_capabilities table is the single complete source for all
	// capability memberships of a process, including the primary.
	//
	// This check only applies when a ProcessCapabilityRepository is configured;
	// in validation-only mode (no PC repo) the requirement is not enforced.
	if s.processCapabilityRepo != nil {
		primaryCapID := strings.TrimSpace(procDoc.Spec.CapabilityID)
		linkKey := procDoc.Metadata.ID + "|" + primaryCapID
		if _, hasLink := bundlePCLinks[linkKey]; !hasLink {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourceValidation
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.capability_id",
				Message: fmt.Sprintf("process primary capability %q must also appear as a ProcessCapability link in the same bundle", primaryCapID),
			})
			return
		}
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// checkCapabilityExists validates that capability_id is present and references an
// existing capability. Returns true when the check passes. Returns false and marks
// the entry invalid when:
//   - capability_id is missing or empty
//   - no CapabilityRepository is configured (cannot validate existence)
//   - the capability does not exist in the repository
//   - the repository lookup returns an error
func (s *Service) checkCapabilityExists(ctx context.Context, doc parser.ParsedDocument, capabilityID string, bundleCapabilityIDs map[string]struct{}, entry *ApplyPlanEntry) bool {
	capabilityID = strings.TrimSpace(capabilityID)
	if capabilityID == "" {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.capability_id",
			Message: "capability_id is required",
		})
		return false
	}
	// If the capability is being created in the same bundle, the reference is satisfied.
	if _, inBundle := bundleCapabilityIDs[capabilityID]; inBundle {
		return true
	}
	if s.capabilityRepo == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.capability_id",
			Message: "capability_id validation unavailable: CapabilityRepository not configured",
		})
		return false
	}
	// GetByID (vs Exists) lets us also inspect Status for an advisory
	// warning when the target exists but is in a terminal lifecycle state.
	cap, err := s.capabilityRepo.GetByID(ctx, capabilityID)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.capability_id",
			Message: "repository error checking capability existence: " + err.Error(),
		})
		return false
	}
	if cap == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.capability_id",
			Message: fmt.Sprintf("capability_id %q does not exist", capabilityID),
		})
		return false
	}
	if isCapabilityStatusTerminal(cap.Status) {
		entry.AddWarning(PlanWarning{
			Code:        WarningRefCapabilityTerminal,
			Severity:    WarningSeverityWarning,
			Message:     fmt.Sprintf("referenced capability %q is %s; referrer will be linked to a terminal-state capability", capabilityID, cap.Status),
			Field:       "spec.capability_id",
			RelatedKind: types.KindCapability,
			RelatedID:   capabilityID,
		})
	}
	return true
}

// checkBusinessServiceExists validates that business_service_id references an
// existing BusinessService. Returns true when the check passes. Returns false
// and marks the entry invalid when:
//   - no BusinessServiceRepository is configured (cannot validate existence)
//   - the business service does not exist in the repository
//   - the repository lookup returns an error
//
// Callers are responsible for only invoking this when business_service_id is
// non-empty; this helper does not treat an empty ID as an error.
func (s *Service) checkBusinessServiceExists(ctx context.Context, doc parser.ParsedDocument, businessServiceID string, bundleBusinessServiceIDs map[string]struct{}, entry *ApplyPlanEntry) bool {
	// If the business service is being created in the same bundle, the reference is satisfied.
	if _, inBundle := bundleBusinessServiceIDs[businessServiceID]; inBundle {
		return true
	}
	if s.businessServiceRepo == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.business_service_id",
			Message: "business_service_id validation unavailable: BusinessServiceRepository not configured",
		})
		return false
	}
	exists, err := s.businessServiceRepo.Exists(ctx, businessServiceID)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.business_service_id",
			Message: "repository error checking business service existence: " + err.Error(),
		})
		return false
	}
	if !exists {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.business_service_id",
			Message: fmt.Sprintf("business_service_id %q does not exist", businessServiceID),
		})
		return false
	}
	return true
}

// checkParentProcessSameCapability validates that when a Process document
// specifies a parent_process_id, the referenced parent:
//  1. Exists (in the bundle or in the repository)
//  2. Belongs to the same capability as the child
//
// Returns true when the check passes. Returns false and marks the entry
// invalid otherwise. Callers must only invoke this when parentID is non-empty.
func (s *Service) checkParentProcessSameCapability(ctx context.Context, doc parser.ParsedDocument, parentID, childCapabilityID string, bundleProcessCapabilityIDs map[string]string, entry *ApplyPlanEntry) bool {
	// If the parent is being created in the same bundle, resolve its
	// capability_id from the bundle pre-pass map — no repo round-trip needed.
	if parentCapabilityID, inBundle := bundleProcessCapabilityIDs[parentID]; inBundle {
		if parentCapabilityID != childCapabilityID {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.parent_process_id",
				Message: fmt.Sprintf("parent process %q belongs to capability %q but child belongs to %q; parent and child must share the same capability", parentID, parentCapabilityID, childCapabilityID),
			})
			return false
		}
		return true
	}

	// Parent not in bundle — look it up in the repository.
	if s.processRepo == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.parent_process_id",
			Message: "parent_process_id validation unavailable: ProcessRepository not configured",
		})
		return false
	}

	parent, err := s.processRepo.GetByID(ctx, parentID)
	if err != nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.parent_process_id",
			Message: "repository error checking parent process: " + err.Error(),
		})
		return false
	}
	if parent == nil {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.parent_process_id",
			Message: fmt.Sprintf("parent process %q does not exist", parentID),
		})
		return false
	}
	if parent.CapabilityID != childCapabilityID {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.parent_process_id",
			Message: fmt.Sprintf("parent process %q belongs to capability %q but child belongs to %q; parent and child must share the same capability", parentID, parent.CapabilityID, childCapabilityID),
		})
		return false
	}
	return true
}

// checkProcessNoCycle verifies that adding the parent→child edge does not create
// a cycle in the Process hierarchy. It walks the parent chain (resolving via the
// bundle pre-pass map first, then the repository) and rejects the entry if any
// ancestor equals childID. Self-parenting (childID == parentID) is considered a
// cycle of length 1 and is also rejected here.
//
// Returns true when no cycle is detected; returns false and marks the entry
// invalid with Field: "spec.parent_process_id" otherwise.
func (s *Service) checkProcessNoCycle(ctx context.Context, doc parser.ParsedDocument, childID, parentID string, bundleProcessParentIDs map[string]string, entry *ApplyPlanEntry) bool {
	// Self-parenting is caught by checkParentProcessSameCapability's "parent does not
	// exist" path in most cases, but we check it explicitly here for clarity.
	if parentID == childID {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.parent_process_id",
			Message: fmt.Sprintf("process %q cannot be its own parent", childID),
		})
		return false
	}

	// Walk the ancestor chain. visited tracks all nodes we have seen so far;
	// if we encounter childID again, or any repeated node, there is a cycle.
	visited := map[string]bool{childID: true}
	current := parentID
	for depth := 0; depth < 50 && current != ""; depth++ {
		if visited[current] {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.parent_process_id",
				Message: fmt.Sprintf("process %q would introduce a cycle in the process hierarchy", childID),
			})
			return false
		}
		visited[current] = true

		// Resolve current's parent from bundle or repository.
		if next, inBundle := bundleProcessParentIDs[current]; inBundle {
			current = next
			continue
		}
		if s.processRepo == nil {
			break
		}
		p, err := s.processRepo.GetByID(ctx, current)
		if err != nil || p == nil {
			break
		}
		current = p.ParentProcessID
	}
	return true
}

// checkCapabilityHierarchy verifies that parent_capability_id references an
// existing capability (in the bundle or repository) and that the hierarchy
// does not contain a cycle. Self-parenting is treated as a cycle of length 1.
//
// Returns true when the hierarchy is valid; returns false and marks the entry
// invalid with Field: "spec.parent_capability_id" otherwise.
func (s *Service) checkCapabilityHierarchy(ctx context.Context, doc parser.ParsedDocument, childID, parentID string, bundleCapabilityIDs map[string]struct{}, bundleCapabilityParentIDs map[string]string, entry *ApplyPlanEntry) bool {
	// Self-parenting check.
	if parentID == childID {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourcePersistedState
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "spec.parent_capability_id",
			Message: fmt.Sprintf("capability %q cannot be its own parent", childID),
		})
		return false
	}

	// Verify that the parent exists (in bundle or in repo).
	_, inBundle := bundleCapabilityIDs[parentID]
	if !inBundle {
		if s.capabilityRepo == nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.parent_capability_id",
				Message: "parent_capability_id validation unavailable: CapabilityRepository not configured",
			})
			return false
		}
		exists, err := s.capabilityRepo.Exists(ctx, parentID)
		if err != nil {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.parent_capability_id",
				Message: "repository error checking parent capability: " + err.Error(),
			})
			return false
		}
		if !exists {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.parent_capability_id",
				Message: fmt.Sprintf("parent capability %q does not exist", parentID),
			})
			return false
		}
	}

	// Cycle detection: walk the ancestor chain.
	visited := map[string]bool{childID: true}
	current := parentID
	for depth := 0; depth < 50 && current != ""; depth++ {
		if visited[current] {
			entry.Action = ApplyActionInvalid
			entry.DecisionSource = DecisionSourcePersistedState
			entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
				Kind:    doc.Kind,
				ID:      doc.ID,
				Field:   "spec.parent_capability_id",
				Message: fmt.Sprintf("capability %q would introduce a cycle in the capability hierarchy", childID),
			})
			return false
		}
		visited[current] = true

		// Resolve current's parent from bundle or repository.
		if next, inBundleMap := bundleCapabilityParentIDs[current]; inBundleMap {
			current = next
			continue
		}
		if s.capabilityRepo == nil {
			break
		}
		c, err := s.capabilityRepo.GetByID(ctx, current)
		if err != nil || c == nil {
			break
		}
		current = c.ParentCapabilityID
	}
	return true
}

// planAgentEntry inspects the current persisted state for an Agent document
// and sets the entry action accordingly.
//
// Agents are identified by ID and are immutable once created in the apply path.
// If an agent with the same ID already exists, the entry is marked as conflict:
// there is no update path in the apply flow and silently overwriting a persisted
// agent is not safe. If the repository lookup fails, the entry is marked invalid.
func (s *Service) planAgentEntry(ctx context.Context, doc parser.ParsedDocument, entry *ApplyPlanEntry) {
	agentDoc, ok := doc.Doc.(types.AgentDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not an AgentDocument",
		})
		return
	}

	existing, err := s.agentRepo.GetByID(ctx, agentDoc.Metadata.ID)
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

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"agent %q already exists; agents are immutable once created in the apply path",
			agentDoc.Metadata.ID,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// planProfileEntry inspects the current persisted state for a Profile document
// and sets the entry action and NewVersion accordingly.
//
// Profiles follow a versioned lineage model: applying a profile document whose
// logical ID already exists in persisted state creates a new version rather than
// conflicting. This is the primary profile modification path — the caller
// supplies an updated profile document, and apply appends it as version N+1.
//
// Entry.NewVersion is set to 1 for first-time creates, and to existing.Version+1
// for subsequent creates. The executor reads NewVersion to assign the correct
// version number when persisting. If the repository lookup fails, the entry is
// marked invalid.
func (s *Service) planProfileEntry(ctx context.Context, doc parser.ParsedDocument, entry *ApplyPlanEntry) {
	profileDoc, ok := doc.Doc.(types.ProfileDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a ProfileDocument",
		})
		return
	}

	existing, err := s.profileRepo.FindByID(ctx, profileDoc.Metadata.ID)
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

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState

	if existing != nil {
		// Append a new version to the existing profile lineage.
		entry.NewVersion = existing.Version + 1
		entry.CreateKind = CreateKindNewVersion
		entry.Message = fmt.Sprintf(
			"profile %q exists at version %d; will create version %d",
			profileDoc.Metadata.ID, existing.Version, existing.Version+1,
		)
		// Field-level diff against the persisted baseline (Issue #37).
		// Diff is advisory output and never blocks apply.
		if diff := computeProfileDiff(existing, profileDoc); diff != nil {
			entry.Diff = diff
		}
	} else {
		entry.NewVersion = 1
		entry.CreateKind = CreateKindNew
	}
}

// planGrantEntry inspects the current persisted state for a Grant document
// and sets the entry action accordingly.
//
// Grants are identified by ID and are immutable once created in the apply path.
// If a grant with the same ID already exists, the entry is marked as conflict:
// there is no update path for grants in the current apply flow. If the repository
// lookup fails, the entry is marked invalid.
func (s *Service) planGrantEntry(ctx context.Context, doc parser.ParsedDocument, entry *ApplyPlanEntry) {
	grantDoc, ok := doc.Doc.(types.GrantDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a GrantDocument",
		})
		return
	}

	existing, err := s.grantRepo.FindByID(ctx, grantDoc.Metadata.ID)
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

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"grant %q already exists; grants are immutable once created in the apply path",
			grantDoc.Metadata.ID,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// executePlan carries out the actions decided by buildApplyPlan, persisting
// resources and building an ApplyResult. The executor does not re-decide
// business meaning: each entry's Action drives its execution path.
//
// Execution rules:
//   - ApplyActionInvalid: the whole bundle is rejected; only validation errors
//     are returned and no resources are persisted.
//   - ApplyActionConflict: the resource is not persisted; a conflict result entry
//     is recorded with the message from the plan.
//   - ApplyActionCreate: the resource is persisted via its backing repository
//     if one is configured; otherwise it is recorded as created (validation-only mode).
//   - ApplyActionUnchanged: not produced by the current planner for any resource
//     type; included here for completeness if future callers construct such a plan.
//
// Dependency-respecting execution order for create entries:
//
//	Surface → Agent → Profile (depends on Surface) → Grant (depends on Agent and Profile)
//
// Conflict and unchanged entries are emitted in document order after creates.
// This ordering ensures that resources are available to their dependants when
// the backing store is inspected by subsequent steps in the same transaction.
func (s *Service) executePlan(ctx context.Context, plan ApplyPlan, actor string) types.ApplyResult {
	var result types.ApplyResult

	if actor == "" {
		actor = "system"
	}

	// If any entry is invalid the entire bundle is rejected — validation errors
	// are surfaced and no resources are persisted.
	if plan.HasInvalid() {
		for _, entry := range plan.Entries {
			if entry.Action == ApplyActionInvalid {
				result.ValidationErrors = append(result.ValidationErrors, entry.ValidationErrors...)
			}
		}
		return result
	}

	// Validation-only mode: no repositories configured. Record all entries as
	// created (conflicts are still reported; unchanged entries are skipped).
	// Dependency order is maintained for consistency even in this mode.
	if s.surfaceRepo == nil && s.agentRepo == nil && s.profileRepo == nil && s.grantRepo == nil &&
		s.processRepo == nil && s.capabilityRepo == nil && s.businessServiceRepo == nil &&
		s.processCapabilityRepo == nil {
		for _, entry := range orderedEntries(plan.Entries) {
			switch entry.Action {
			case ApplyActionConflict:
				result.AddConflict(entry.Kind, entry.ID, entry.Message)
			case ApplyActionUnchanged:
				result.AddUnchanged(entry.Kind, entry.ID)
			default:
				result.AddCreated(entry.Kind, entry.ID)
			}
		}
		return result
	}

	// Atomicity (see issue: Atomic control-plane apply):
	//
	// The execution loop either completes in full or leaves no persisted
	// mutations behind. On the first persistence error it aborts, discards
	// any in-memory state accumulated for this bundle, and — when a
	// TxRunner is wired — rolls back every write that occurred so far in
	// the same bundle.
	//
	// Audit records are buffered INSIDE the loop and flushed AFTER the
	// transaction commits. This preserves the ADR-041b posture that
	// control audit sits outside the state-change transaction boundary
	// while avoiding ghost audit rows for mutations that were rolled
	// back.
	//
	// When s.tx is nil (e.g. the memory store in dev mode) the loop still
	// aborts on the first error, but prior writes cannot be rolled back
	// because the underlying repositories auto-commit. This is the best
	// guarantee available without transactional storage.
	now := time.Now().UTC()
	var pendingAudit []*controlaudit.ControlAuditRecord

	loop := func(scoped *RepositorySet) error {
		return s.runApplyLoop(ctx, plan, actor, now, scoped, &pendingAudit, &result)
	}

	if s.tx != nil {
		if err := s.tx.WithTx(ctx, "control_plane_apply", loop); err != nil {
			// The transaction has rolled back. runApplyLoop has already
			// recorded the triggering error into result.Results; any
			// buffered audit records correspond to mutations that did not
			// persist and must be discarded. Deliberately do not surface
			// the raw tx error separately — the per-resource error entry
			// already names the failing operation.
			pendingAudit = nil
		}
	} else {
		ownRepos := s.ownRepositorySet()
		_ = loop(ownRepos)
	}

	for _, rec := range pendingAudit {
		s.appendControlAudit(ctx, rec)
	}

	return result
}

// runApplyLoop is the iteration body shared by the transactional and non-
// transactional execution paths. It aborts on the first persistence error
// and, when aborting, records the error into result before returning it to
// the caller (so the caller — and, in the transactional case, the TxRunner
// — can surface the rollback to the user via result.Results).
func (s *Service) runApplyLoop(
	ctx context.Context,
	plan ApplyPlan,
	actor string,
	now time.Time,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	result *types.ApplyResult,
) error {
	for _, entry := range orderedEntries(plan.Entries) {
		switch entry.Action {
		case ApplyActionConflict:
			// Conflict was detected at planning time; do not persist.
			result.AddConflict(entry.Kind, entry.ID, entry.Message)

		case ApplyActionUnchanged:
			// Unchanged entries are skipped; no persistence occurs.
			result.AddUnchanged(entry.Kind, entry.ID)

		case ApplyActionCreate:
			if err := s.applyCreateEntry(ctx, repos, pendingAudit, entry, now, actor, result); err != nil {
				// Abort: the caller either owns a transaction that will
				// roll back, or is running without transactional
				// storage. Either way, no further entries are attempted.
				result.AddError(entry.Kind, entry.ID, err.Error())
				return err
			}

		default:
			// Unknown action — record as an error and abort to surface
			// the planning gap without leaving the bundle half-applied.
			err := fmt.Errorf("unexpected plan action: %s", entry.Action)
			result.AddError(entry.Kind, entry.ID, err.Error())
			return err
		}
	}
	return nil
}

// applyCreateEntry dispatches a single create entry to the kind-specific
// helper. Returns the first error unmodified so the caller can abort the
// bundle.
func (s *Service) applyCreateEntry(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	entry ApplyPlanEntry,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	switch entry.Kind {
	case types.KindBusinessService:
		if repos.BusinessServices == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyBusinessService(ctx, repos, entry.Doc, now, result)
	case types.KindCapability:
		if repos.Capabilities == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyCapability(ctx, repos, entry.Doc, now, actor, result)
	case types.KindProcess:
		if repos.Processes == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyProcess(ctx, repos, entry.Doc, now, actor, result)
	case types.KindProcessCapability:
		if repos.ProcessCapabilities == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyProcessCapability(ctx, repos, entry.Doc, now, result)
	case types.KindProcessBusinessService:
		if repos.ProcessBusinessServices == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyProcessBusinessService(ctx, repos, entry.Doc, now, result)
	case types.KindSurface:
		if repos.Surfaces == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applySurface(ctx, repos, pendingAudit, entry.Doc, now, actor, result)
	case types.KindAgent:
		if repos.Agents == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyAgent(ctx, repos, pendingAudit, entry.Doc, now, actor, result)
	case types.KindProfile:
		if repos.Profiles == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyProfile(ctx, repos, pendingAudit, entry.Doc, now, actor, entry.NewVersion, result)
	case types.KindGrant:
		if repos.Grants == nil {
			result.AddCreated(entry.Kind, entry.ID)
			return nil
		}
		return s.applyGrant(ctx, repos, pendingAudit, entry.Doc, now, actor, result)
	default:
		result.AddCreated(entry.Kind, entry.ID)
		return nil
	}
}

// ownRepositorySet returns a RepositorySet view of the Service's own
// configured repositories. It is used by the non-transactional execution
// path so that runApplyLoop can read through a uniform *RepositorySet
// regardless of whether a TxRunner is wired.
func (s *Service) ownRepositorySet() *RepositorySet {
	return &RepositorySet{
		Surfaces:                s.surfaceRepo,
		Agents:                  s.agentRepo,
		Profiles:                s.profileRepo,
		Grants:                  s.grantRepo,
		Processes:               s.processRepo,
		Capabilities:            s.capabilityRepo,
		BusinessServices:        s.businessServiceRepo,
		ProcessCapabilities:     s.processCapabilityRepo,
		ProcessBusinessServices: s.processBusinessServiceRepo,
	}
}

// appendControlAudit appends a control-plane audit record. It is a no-op when
// the controlAuditRepo is nil, preserving existing behaviour for callers that
// do not configure the control audit repository.
//
// Per ADR-041b, control audit is best-effort and sits outside the
// state-change transaction. Append errors are deliberately swallowed:
// a failed audit write must not fail the control-plane action, because
// the action has already committed (or would be rolled back for unrelated
// reasons). Structured logging of the swallow is a named follow-up in
// ADR-041b and is not implemented in this issue.
//
// Called only from the post-commit flush path in executePlan: records
// for a rolled-back bundle are discarded before this is reached.
func (s *Service) appendControlAudit(ctx context.Context, rec *controlaudit.ControlAuditRecord) {
	if s.controlAuditRepo == nil {
		return
	}
	_ = s.controlAuditRepo.Append(ctx, rec)
}

// orderedEntries returns plan entries sorted into dependency-respecting execution
// order. Create entries are emitted in the following sequence:
//
//  1. BusinessService         — no dependencies
//  2. Capability              — no dependencies
//  3. Process                 — depends on Capability
//  4. ProcessCapability       — depends on Process and Capability
//  4. ProcessBusinessService  — depends on Process and BusinessService (same tier)
//  5. Surface                 — depends on Process
//  6. Agent                   — no dependencies
//  7. Profile                 — depends on Surface
//  8. Grant                   — depends on Agent and Profile
//  9. Other                   — unknown kinds, emitted after known kinds
//
// Within each tier, relative document order is preserved. Conflict and unchanged
// entries are emitted after all create entries, in their original document order,
// because they produce no persisted output and carry no dependency implications.
func orderedEntries(entries []ApplyPlanEntry) []ApplyPlanEntry {
	kindOrder := map[string]int{
		types.KindBusinessService:       0,
		types.KindCapability:            1,
		types.KindProcess:               2,
		types.KindProcessCapability:     3,
		types.KindProcessBusinessService: 3,
		types.KindSurface:               4,
		types.KindAgent:                 5,
		types.KindProfile:               6,
		types.KindGrant:                 7,
	}

	// Separate create entries (must respect order) from non-create entries.
	creates := make([][]ApplyPlanEntry, 9) // 8 known tiers + 1 unknown
	var nonCreates []ApplyPlanEntry

	for _, e := range entries {
		if e.Action != ApplyActionCreate {
			nonCreates = append(nonCreates, e)
			continue
		}
		tier, known := kindOrder[e.Kind]
		if !known {
			tier = 8
		}
		creates[tier] = append(creates[tier], e)
	}

	ordered := make([]ApplyPlanEntry, 0, len(entries))
	for _, tier := range creates {
		ordered = append(ordered, tier...)
	}
	ordered = append(ordered, nonCreates...)
	return ordered
}

// applySurface creates a governed surface version in review state.
// New applies always create a new versioned record; they do not update in place.
//
// The scoped repos and pendingAudit buffer are threaded from executePlan so
// that mutations participate in the caller's transaction and audit records
// are deferred until after commit (see executePlan for the audit-boundary
// rationale).
func (s *Service) applySurface(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	surfaceDoc, ok := doc.Doc.(types.SurfaceDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindSurface)
	}

	latest, err := repos.Surfaces.FindLatestByID(ctx, surfaceDoc.Metadata.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("find latest surface by id: %w", err)
	}

	version := 1
	if latest != nil {
		version = latest.Version + 1
	}

	ds, err := mapSurfaceDocumentToDecisionSurface(surfaceDoc, now, actor, version)
	if err != nil {
		return fmt.Errorf("map surface document: %w", err)
	}

	if err := repos.Surfaces.Create(ctx, ds); err != nil {
		return fmt.Errorf("create surface version: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	*pendingAudit = append(*pendingAudit, controlaudit.NewSurfaceCreatedRecord(actor, ds.ID, ds.Version))
	return nil
}

// applyAgent maps an AgentDocument to an Agent domain model and persists it.
func (s *Service) applyAgent(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	agentDoc, ok := doc.Doc.(types.AgentDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindAgent)
	}

	a, err := mapAgentDocumentToAgent(agentDoc, now)
	if err != nil {
		return fmt.Errorf("map agent document: %w", err)
	}

	if err := repos.Agents.Create(ctx, a); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	*pendingAudit = append(*pendingAudit, controlaudit.NewAgentCreatedRecord(actor, a.ID))
	return nil
}

// applyProfile maps a ProfileDocument to an AuthorityProfile domain model and persists it.
// version is the planned version number assigned by planProfileEntry: 1 for a
// first-time create, N+1 when appending to an existing profile lineage.
func (s *Service) applyProfile(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	version int,
	result *types.ApplyResult,
) error {
	profileDoc, ok := doc.Doc.(types.ProfileDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindProfile)
	}

	p, err := mapProfileDocumentToAuthorityProfile(profileDoc, now, actor, version)
	if err != nil {
		return fmt.Errorf("map profile document: %w", err)
	}

	if err := repos.Profiles.Create(ctx, p); err != nil {
		return fmt.Errorf("create profile: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)

	if version == 1 {
		*pendingAudit = append(*pendingAudit, controlaudit.NewProfileCreatedRecord(actor, p.ID, p.SurfaceID, version))
	} else {
		*pendingAudit = append(*pendingAudit, controlaudit.NewProfileVersionedRecord(actor, p.ID, p.SurfaceID, version))
	}
	return nil
}

// applyGrant maps a GrantDocument to an AuthorityGrant domain model and persists it.
func (s *Service) applyGrant(
	ctx context.Context,
	repos *RepositorySet,
	pendingAudit *[]*controlaudit.ControlAuditRecord,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	grantDoc, ok := doc.Doc.(types.GrantDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindGrant)
	}

	g, err := mapGrantDocumentToAuthorityGrant(grantDoc, now)
	if err != nil {
		return fmt.Errorf("map grant document: %w", err)
	}

	if err := repos.Grants.Create(ctx, g); err != nil {
		return fmt.Errorf("create grant: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	*pendingAudit = append(*pendingAudit, controlaudit.NewGrantCreatedRecord(actor, g.ID))
	return nil
}

// applyCapability maps a CapabilityDocument to a Capability domain model and persists it.
func (s *Service) applyCapability(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	capDoc, ok := doc.Doc.(types.CapabilityDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindCapability)
	}

	c := mapCapabilityDocumentToCapability(capDoc, now, actor)

	if err := repos.Capabilities.Create(ctx, c); err != nil {
		return fmt.Errorf("create capability: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// planProcessCapabilityEntry inspects the current persisted state for a
// ProcessCapability document and sets the entry action accordingly.
//
// ProcessCapability links are identified by the (process_id, capability_id) pair.
// If a link with the same pair already exists, the entry is marked as conflict.
// The metadata.id is a synthetic control-plane handle and is not stored.
func (s *Service) planProcessCapabilityEntry(ctx context.Context, doc parser.ParsedDocument, bundleProcessIDs map[string]struct{}, bundleCapabilityIDs map[string]struct{}, entry *ApplyPlanEntry) {
	pcDoc, ok := doc.Doc.(types.ProcessCapabilityDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a ProcessCapabilityDocument",
		})
		return
	}

	if !s.checkProcessExists(ctx, doc, pcDoc.Spec.ProcessID, bundleProcessIDs, entry) {
		return
	}
	if !s.checkCapabilityExists(ctx, doc, pcDoc.Spec.CapabilityID, bundleCapabilityIDs, entry) {
		return
	}

	existing, err := s.processCapabilityRepo.ListByProcessID(ctx, pcDoc.Spec.ProcessID)
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

	for _, pc := range existing {
		if pc.CapabilityID == pcDoc.Spec.CapabilityID {
			entry.Action = ApplyActionConflict
			entry.DecisionSource = DecisionSourcePersistedState
			entry.Message = fmt.Sprintf(
				"process capability link between process %q and capability %q already exists",
				pcDoc.Spec.ProcessID, pcDoc.Spec.CapabilityID,
			)
			return
		}
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// applyProcessCapability maps a ProcessCapabilityDocument to a ProcessCapability
// domain model and persists it.
func (s *Service) applyProcessCapability(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	result *types.ApplyResult,
) error {
	pcDoc, ok := doc.Doc.(types.ProcessCapabilityDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindProcessCapability)
	}

	pc := mapProcessCapabilityDocumentToProcessCapability(pcDoc, now)
	if err := repos.ProcessCapabilities.Create(ctx, pc); err != nil {
		return fmt.Errorf("create process capability: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// planProcessBusinessServiceEntry inspects the current persisted state for a
// ProcessBusinessService document and sets the entry action accordingly.
//
// ProcessBusinessService links are identified by the (process_id, business_service_id) pair.
// If a link with the same pair already exists, the entry is marked as conflict.
// The metadata.id is a synthetic control-plane handle and is not stored.
func (s *Service) planProcessBusinessServiceEntry(ctx context.Context, doc parser.ParsedDocument, bundleProcessIDs map[string]struct{}, bundleBusinessServiceIDs map[string]struct{}, entry *ApplyPlanEntry) {
	pbsDoc, ok := doc.Doc.(types.ProcessBusinessServiceDocument)
	if !ok {
		entry.Action = ApplyActionInvalid
		entry.DecisionSource = DecisionSourceValidation
		entry.ValidationErrors = append(entry.ValidationErrors, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: "document payload is not a ProcessBusinessServiceDocument",
		})
		return
	}

	if !s.checkProcessExists(ctx, doc, pbsDoc.Spec.ProcessID, bundleProcessIDs, entry) {
		return
	}
	if !s.checkBusinessServiceExists(ctx, doc, pbsDoc.Spec.BusinessServiceID, bundleBusinessServiceIDs, entry) {
		return
	}

	existing, err := s.processBusinessServiceRepo.ListByProcessID(ctx, pbsDoc.Spec.ProcessID)
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

	for _, pbs := range existing {
		if pbs.BusinessServiceID == pbsDoc.Spec.BusinessServiceID {
			entry.Action = ApplyActionConflict
			entry.DecisionSource = DecisionSourcePersistedState
			entry.Message = fmt.Sprintf(
				"process business service link between process %q and business service %q already exists",
				pbsDoc.Spec.ProcessID, pbsDoc.Spec.BusinessServiceID,
			)
			return
		}
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
	entry.CreateKind = CreateKindNew
}

// applyProcessBusinessService maps a ProcessBusinessServiceDocument to a ProcessBusinessService
// domain model and persists it.
func (s *Service) applyProcessBusinessService(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	result *types.ApplyResult,
) error {
	pbsDoc, ok := doc.Doc.(types.ProcessBusinessServiceDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindProcessBusinessService)
	}

	pbs := mapProcessBusinessServiceDocumentToProcessBusinessService(pbsDoc, now)
	if err := repos.ProcessBusinessServices.Create(ctx, pbs); err != nil {
		return fmt.Errorf("create process business service: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// applyBusinessService maps a BusinessServiceDocument to a BusinessService domain model and persists it.
func (s *Service) applyBusinessService(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	result *types.ApplyResult,
) error {
	bsDoc, ok := doc.Doc.(types.BusinessServiceDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindBusinessService)
	}

	bs := mapBusinessServiceDocumentToBusinessService(bsDoc, now)

	if err := repos.BusinessServices.Create(ctx, bs); err != nil {
		return fmt.Errorf("create business service: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// applyProcess maps a ProcessDocument to a Process domain model and persists it.
func (s *Service) applyProcess(
	ctx context.Context,
	repos *RepositorySet,
	doc parser.ParsedDocument,
	now time.Time,
	actor string,
	result *types.ApplyResult,
) error {
	procDoc, ok := doc.Doc.(types.ProcessDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindProcess)
	}

	p := mapProcessDocumentToProcess(procDoc, now, actor)

	if err := repos.Processes.Create(ctx, p); err != nil {
		return fmt.Errorf("create process: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}
