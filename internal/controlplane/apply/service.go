package apply

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/controlplane/validate"
	"github.com/accept-io/midas/internal/surface"
)

// Service coordinates control-plane apply operations.
type Service struct {
	surfaceRepo SurfaceRepository
	agentRepo   AgentRepository
	profileRepo ProfileRepository
	grantRepo   GrantRepository
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
// for that kind.
func NewServiceWithRepos(repos RepositorySet) *Service {
	return &Service{
		surfaceRepo: repos.Surfaces,
		agentRepo:   repos.Agents,
		profileRepo: repos.Profiles,
		grantRepo:   repos.Grants,
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

// Apply validates a parsed bundle and applies it.
//
// Apply builds the plan via Plan and then executes it. No planning logic is
// duplicated here.
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
func (s *Service) Apply(ctx context.Context, docs []parser.ParsedDocument) types.ApplyResult {
	plan := s.Plan(ctx, docs)
	return s.executePlan(ctx, plan)
}

// ApplyBundle parses a raw YAML bundle and applies it through the validation
// and apply pipeline.
//
// Behavior:
//   - parse failures return an error wrapping ErrInvalidBundle
//   - successfully parsed bundles always return an ApplyResult
//   - validation failures are represented inside ApplyResult, not as an error
func (s *Service) ApplyBundle(ctx context.Context, yamlBytes []byte) (*types.ApplyResult, error) {
	docs, err := parser.ParseYAMLStream(yamlBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBundle, err)
	}

	result := s.Apply(ctx, docs)
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
		} else {
			switch doc.Kind {
			case types.KindSurface:
				if s.surfaceRepo != nil {
					s.planSurfaceEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
				}
			case types.KindAgent:
				if s.agentRepo != nil {
					s.planAgentEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
				}
			case types.KindProfile:
				if s.profileRepo != nil {
					s.planProfileEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
				}
			case types.KindGrant:
				if s.grantRepo != nil {
					s.planGrantEntry(ctx, doc, &entry)
				} else {
					entry.Action = ApplyActionCreate
					entry.DecisionSource = DecisionSourceValidation
				}
			default:
				entry.Action = ApplyActionCreate
				entry.DecisionSource = DecisionSourceValidation
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
			} else if refSource == DecisionSourceBundleDependency {
				// Reference was satisfied by a same-bundle create entry.
				entry.DecisionSource = DecisionSourceBundleDependency
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
				} else if profileRefSource == DecisionSourceBundleDependency && entry.Action == ApplyActionCreate {
					entry.DecisionSource = DecisionSourceBundleDependency
				}
			}
		}
	}
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
func (s *Service) planSurfaceEntry(ctx context.Context, doc parser.ParsedDocument, entry *ApplyPlanEntry) {
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
		entry.Action = ApplyActionCreate
		entry.DecisionSource = DecisionSourcePersistedState
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
	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
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
}

// planProfileEntry inspects the current persisted state for a Profile document
// and sets the entry action accordingly.
//
// Profiles are identified by ID and are immutable once created in the apply path.
// If a profile with the same ID already exists, the entry is marked as conflict:
// there is no version-increment or update path for profiles in the current apply
// flow. If the repository lookup fails, the entry is marked invalid.
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

	if existing != nil {
		entry.Action = ApplyActionConflict
		entry.DecisionSource = DecisionSourcePersistedState
		entry.Message = fmt.Sprintf(
			"profile %q already exists; profiles are immutable once created in the apply path",
			profileDoc.Metadata.ID,
		)
		return
	}

	entry.Action = ApplyActionCreate
	entry.DecisionSource = DecisionSourcePersistedState
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
func (s *Service) executePlan(ctx context.Context, plan ApplyPlan) types.ApplyResult {
	var result types.ApplyResult

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
	if s.surfaceRepo == nil && s.agentRepo == nil && s.profileRepo == nil && s.grantRepo == nil {
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

	now := time.Now().UTC()
	createdBy := "system"

	for _, entry := range orderedEntries(plan.Entries) {
		switch entry.Action {
		case ApplyActionConflict:
			// Conflict was detected at planning time; do not persist.
			result.AddConflict(entry.Kind, entry.ID, entry.Message)

		case ApplyActionUnchanged:
			// Unchanged entries are skipped; no persistence occurs.
			result.AddUnchanged(entry.Kind, entry.ID)

		case ApplyActionCreate:
			switch entry.Kind {
			case types.KindSurface:
				if s.surfaceRepo != nil {
					if err := s.applySurface(ctx, entry.Doc, now, createdBy, &result); err != nil {
						result.AddError(entry.Kind, entry.ID, err.Error())
					}
				} else {
					result.AddCreated(entry.Kind, entry.ID)
				}
			case types.KindAgent:
				if s.agentRepo != nil {
					if err := s.applyAgent(ctx, entry.Doc, now, &result); err != nil {
						result.AddError(entry.Kind, entry.ID, err.Error())
					}
				} else {
					result.AddCreated(entry.Kind, entry.ID)
				}
			case types.KindProfile:
				if s.profileRepo != nil {
					if err := s.applyProfile(ctx, entry.Doc, now, createdBy, &result); err != nil {
						result.AddError(entry.Kind, entry.ID, err.Error())
					}
				} else {
					result.AddCreated(entry.Kind, entry.ID)
				}
			case types.KindGrant:
				if s.grantRepo != nil {
					if err := s.applyGrant(ctx, entry.Doc, now, &result); err != nil {
						result.AddError(entry.Kind, entry.ID, err.Error())
					}
				} else {
					result.AddCreated(entry.Kind, entry.ID)
				}
			default:
				result.AddCreated(entry.Kind, entry.ID)
			}

		default:
			// Unknown action — record as an error to surface the planning gap.
			result.AddError(entry.Kind, entry.ID, "unexpected plan action: "+string(entry.Action))
		}
	}

	return result
}

// orderedEntries returns plan entries sorted into dependency-respecting execution
// order. Create entries are emitted in the following sequence:
//
//  1. Surface  — no dependencies
//  2. Agent    — no dependencies
//  3. Profile  — depends on Surface
//  4. Grant    — depends on Agent and Profile
//  5. Other    — unknown kinds, emitted after known kinds
//
// Within each tier, relative document order is preserved. Conflict and unchanged
// entries are emitted after all create entries, in their original document order,
// because they produce no persisted output and carry no dependency implications.
func orderedEntries(entries []ApplyPlanEntry) []ApplyPlanEntry {
	kindOrder := map[string]int{
		types.KindSurface: 0,
		types.KindAgent:   1,
		types.KindProfile: 2,
		types.KindGrant:   3,
	}

	// Separate create entries (must respect order) from non-create entries.
	creates := make([][]ApplyPlanEntry, 5) // 4 known tiers + 1 unknown
	var nonCreates []ApplyPlanEntry

	for _, e := range entries {
		if e.Action != ApplyActionCreate {
			nonCreates = append(nonCreates, e)
			continue
		}
		tier, known := kindOrder[e.Kind]
		if !known {
			tier = 4
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
func (s *Service) applySurface(
	ctx context.Context,
	doc parser.ParsedDocument,
	now time.Time,
	createdBy string,
	result *types.ApplyResult,
) error {
	surfaceDoc, ok := doc.Doc.(types.SurfaceDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindSurface)
	}

	latest, err := s.surfaceRepo.FindLatestByID(ctx, surfaceDoc.Metadata.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("find latest surface by id: %w", err)
	}

	version := 1
	if latest != nil {
		version = latest.Version + 1
	}

	ds, err := mapSurfaceDocumentToDecisionSurface(surfaceDoc, now, createdBy, version)
	if err != nil {
		return fmt.Errorf("map surface document: %w", err)
	}

	if err := s.surfaceRepo.Create(ctx, ds); err != nil {
		return fmt.Errorf("create surface version: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// applyAgent maps an AgentDocument to an Agent domain model and persists it.
func (s *Service) applyAgent(
	ctx context.Context,
	doc parser.ParsedDocument,
	now time.Time,
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

	if err := s.agentRepo.Create(ctx, a); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// applyProfile maps a ProfileDocument to an AuthorityProfile domain model and persists it.
func (s *Service) applyProfile(
	ctx context.Context,
	doc parser.ParsedDocument,
	now time.Time,
	createdBy string,
	result *types.ApplyResult,
) error {
	profileDoc, ok := doc.Doc.(types.ProfileDocument)
	if !ok {
		return fmt.Errorf("%w: invalid document payload for kind %q", ErrInvalidBundle, types.KindProfile)
	}

	p, err := mapProfileDocumentToAuthorityProfile(profileDoc, now, createdBy)
	if err != nil {
		return fmt.Errorf("map profile document: %w", err)
	}

	if err := s.profileRepo.Create(ctx, p); err != nil {
		return fmt.Errorf("create profile: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}

// applyGrant maps a GrantDocument to an AuthorityGrant domain model and persists it.
func (s *Service) applyGrant(
	ctx context.Context,
	doc parser.ParsedDocument,
	now time.Time,
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

	if err := s.grantRepo.Create(ctx, g); err != nil {
		return fmt.Errorf("create grant: %w", err)
	}

	result.AddCreated(doc.Kind, doc.ID)
	return nil
}
