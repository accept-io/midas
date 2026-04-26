package apply

// Tests for Issue #37 — improved control-plane dry-run output:
//   - structured referential warnings for terminal-state references
//   - resource-level create_kind classification
//   - field-level diff for Surface/Profile versioned creates
//
// All three are additive, output-only, and must not change apply semantics,
// would_apply, invalid_count, or conflict_count.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Local test doubles for capability- and process-centric terminal-state tests
// ---------------------------------------------------------------------------

// stubCapabilityRepo implements CapabilityRepository via function fields so
// each test can dictate the status returned for a specific capability ID.
type stubCapabilityRepo struct {
	getByIDFn   func(ctx context.Context, id string) (*capability.Capability, error)
	existsFn    func(ctx context.Context, id string) (bool, error)
	createCalls int
}

func (r *stubCapabilityRepo) Exists(ctx context.Context, id string) (bool, error) {
	if r.existsFn != nil {
		return r.existsFn(ctx, id)
	}
	c, err := r.GetByID(ctx, id)
	return c != nil, err
}

func (r *stubCapabilityRepo) GetByID(ctx context.Context, id string) (*capability.Capability, error) {
	if r.getByIDFn != nil {
		return r.getByIDFn(ctx, id)
	}
	return nil, nil
}

func (r *stubCapabilityRepo) Create(_ context.Context, _ *capability.Capability) error {
	r.createCalls++
	return nil
}


// findEntry returns the plan entry for (kind, id) or fails.
func findEntry(t *testing.T, plan ApplyPlan, kind, id string) ApplyPlanEntry {
	t.Helper()
	for _, e := range plan.Entries {
		if e.Kind == kind && e.ID == id {
			return e
		}
	}
	t.Fatalf("no plan entry found for %s/%s", kind, id)
	return ApplyPlanEntry{}
}

// hasWarning returns the first warning on entry whose Code matches, or nil.
func findWarning(entry ApplyPlanEntry, code WarningCode) *PlanWarning {
	for i := range entry.Warnings {
		if entry.Warnings[i].Code == code {
			return &entry.Warnings[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Warning case 1 — Profile → Surface (deprecated)
// ---------------------------------------------------------------------------

func TestPlan_ProfileReferencesDeprecatedSurface_EmitsWarning(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:      id,
				Version: 3,
				Status:  surface.SurfaceStatusDeprecated,
			}, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		validProfileWithSurface("profile-x", "surf-deprecated"),
	})

	entry := findEntry(t, plan, types.KindProfile, "profile-x")
	if entry.Action != ApplyActionCreate {
		t.Fatalf("warning must not change action; want create, got %s", entry.Action)
	}
	w := findWarning(entry, WarningRefSurfaceTerminal)
	if w == nil {
		t.Fatalf("expected %s warning, got warnings=%+v", WarningRefSurfaceTerminal, entry.Warnings)
	}
	if w.Severity != WarningSeverityWarning {
		t.Errorf("severity = %q, want %q", w.Severity, WarningSeverityWarning)
	}
	if w.Field != "spec.surface_id" {
		t.Errorf("field = %q, want %q", w.Field, "spec.surface_id")
	}
	if w.RelatedKind != types.KindSurface || w.RelatedID != "surf-deprecated" {
		t.Errorf("related = %s/%s, want Surface/surf-deprecated", w.RelatedKind, w.RelatedID)
	}

	// Wire mapping: would_apply unchanged, invalid_count = 0, warning flows through.
	result := PlanResultFromPlan(plan)
	if result.InvalidCount != 0 || result.ConflictCount != 0 {
		t.Errorf("warning must not change counts: invalid=%d conflict=%d", result.InvalidCount, result.ConflictCount)
	}
	if !result.WouldApply {
		t.Errorf("would_apply must remain true when only warnings are present")
	}
	if len(result.Entries[0].Warnings) == 0 {
		t.Errorf("warnings did not survive wire mapping")
	}
}

// ---------------------------------------------------------------------------
// Warning case 2 — Grant → Profile (deprecated)
// ---------------------------------------------------------------------------

func TestPlan_GrantReferencesDeprecatedProfile_EmitsWarning(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, id string) (*agent.Agent, error) {
			return &agent.Agent{ID: id}, nil
		},
	}
	// We need the agent to resolve referentially. But to keep planGrantEntry
	// from marking the grant as conflict we want NO existing grant by ID.
	// Profile lookup must return a deprecated profile so the warning fires.
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{
				ID:      id,
				Version: 2,
				Status:  authority.ProfileStatusDeprecated,
			}, nil
		},
	}
	grantRepo := &controlledGrantRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityGrant, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{
		Agents:   agentRepo,
		Profiles: profileRepo,
		Grants:   grantRepo,
	})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		validGrantWithRefs("grant-x", "agent-existing", "profile-deprecated"),
	})

	entry := findEntry(t, plan, types.KindGrant, "grant-x")
	if entry.Action != ApplyActionCreate {
		t.Fatalf("warning must not change action; want create, got %s; errors=%+v", entry.Action, entry.ValidationErrors)
	}
	w := findWarning(entry, WarningRefProfileTerminal)
	if w == nil {
		t.Fatalf("expected %s warning, got warnings=%+v", WarningRefProfileTerminal, entry.Warnings)
	}
	if w.Field != "spec.profile_id" {
		t.Errorf("field = %q, want %q", w.Field, "spec.profile_id")
	}
	if w.RelatedKind != types.KindProfile || w.RelatedID != "profile-deprecated" {
		t.Errorf("related = %s/%s, want Profile/profile-deprecated", w.RelatedKind, w.RelatedID)
	}

	result := PlanResultFromPlan(plan)
	if result.InvalidCount != 0 || result.ConflictCount != 0 {
		t.Errorf("warning must not change counts: invalid=%d conflict=%d", result.InvalidCount, result.ConflictCount)
	}
}

// ---------------------------------------------------------------------------
// Warning case 3 — Surface → Process (deprecated)
// ---------------------------------------------------------------------------

func TestPlan_SurfaceReferencesDeprecatedProcess_EmitsWarning(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, _ string) (*surface.DecisionSurface, error) {
			return nil, nil // new surface
		},
	}
	procRepo := &controlledProcessRepo{
		getByIDFn: func(_ context.Context, id string) (*process.Process, error) {
			return &process.Process{ID: id, Status: "deprecated"}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{
		Surfaces:  surfaceRepo,
		Processes: procRepo,
	})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		surfaceDocWithProcessID("surf-x", "proc-deprecated"),
	})

	entry := findEntry(t, plan, types.KindSurface, "surf-x")
	if entry.Action != ApplyActionCreate {
		t.Fatalf("warning must not change action; want create, got %s; errors=%+v", entry.Action, entry.ValidationErrors)
	}
	w := findWarning(entry, WarningRefProcessTerminal)
	if w == nil {
		t.Fatalf("expected %s warning, got warnings=%+v", WarningRefProcessTerminal, entry.Warnings)
	}
	if w.Field != "spec.process_id" {
		t.Errorf("field = %q, want %q", w.Field, "spec.process_id")
	}
	if w.RelatedKind != types.KindProcess || w.RelatedID != "proc-deprecated" {
		t.Errorf("related = %s/%s, want Process/proc-deprecated", w.RelatedKind, w.RelatedID)
	}

	result := PlanResultFromPlan(plan)
	if result.InvalidCount != 0 || result.ConflictCount != 0 {
		t.Errorf("warning must not change counts: invalid=%d conflict=%d", result.InvalidCount, result.ConflictCount)
	}
}

// ---------------------------------------------------------------------------
// Warning case 4 — Process → Capability (deprecated)
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Warning regression — non-terminal targets do not emit warnings
// ---------------------------------------------------------------------------

func TestPlan_NonTerminalTargets_NoWarnings(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Version: 1, Status: surface.SurfaceStatusActive}, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		validProfileWithSurface("profile-ok", "surf-active"),
	})

	entry := findEntry(t, plan, types.KindProfile, "profile-ok")
	if len(entry.Warnings) != 0 {
		t.Errorf("expected no warnings for active target; got %+v", entry.Warnings)
	}
}

// ---------------------------------------------------------------------------
// create_kind — plain new
// ---------------------------------------------------------------------------

func TestPlan_NewProfile_CreateKindNew(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{validProfile("profile-first")})
	entry := findEntry(t, plan, types.KindProfile, "profile-first")

	if entry.Action != ApplyActionCreate {
		t.Fatalf("expected action=create, got %s", entry.Action)
	}
	if entry.CreateKind != CreateKindNew {
		t.Errorf("expected create_kind=%s, got %s", CreateKindNew, entry.CreateKind)
	}
	if entry.Diff != nil {
		t.Errorf("plain new creates must not carry a diff; got %+v", entry.Diff)
	}

	result := PlanResultFromPlan(plan)
	if result.Entries[0].CreateKind != string(CreateKindNew) {
		t.Errorf("wire create_kind = %q, want %q", result.Entries[0].CreateKind, CreateKindNew)
	}
	if result.Entries[0].Diff != nil {
		t.Errorf("wire diff must be nil for plain new; got %+v", result.Entries[0].Diff)
	}
}

// ---------------------------------------------------------------------------
// create_kind — versioned Surface (new_version)
// ---------------------------------------------------------------------------

func TestPlan_VersionedSurface_CreateKindNewVersion_EmitsDiff(t *testing.T) {
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:                 id,
				Version:            2,
				Status:             surface.SurfaceStatusActive,
				Name:               "Old Name",
				Description:        "Old description",
				Domain:             "financial_services",
				MinimumConfidence:  0.5,
				DecisionType:       surface.DecisionTypeOperational,
				ReversibilityClass: surface.ReversibilityConditionallyReversible,
				FailureMode:        surface.FailureModeClosed,
				BusinessOwner:      "old-owner@example.com",
				TechnicalOwner:     "old-tech@example.com",
				ProcessID:          "test.process",
			}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Processes: processRepoAlwaysExists()})

	// Build a surface document whose mutable fields differ from the baseline.
	doc := parser.ParsedDocument{
		Kind: types.KindSurface,
		ID:   "surf-v3",
		Doc: types.SurfaceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindSurface,
			Metadata:   types.DocumentMetadata{ID: "surf-v3", Name: "New Name"},
			Spec: types.SurfaceSpec{
				Description:    "New description",
				Category:       "financial",
				RiskTier:       "high",
				Status:         "active",
				ProcessID:      "test.process",
				BusinessOwner:  "new-owner@example.com",
				TechnicalOwner: "new-tech@example.com",
			},
		},
	}

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{doc})
	entry := findEntry(t, plan, types.KindSurface, "surf-v3")

	if entry.Action != ApplyActionCreate {
		t.Fatalf("expected action=create, got %s", entry.Action)
	}
	if entry.CreateKind != CreateKindNewVersion {
		t.Fatalf("expected create_kind=%s, got %s", CreateKindNewVersion, entry.CreateKind)
	}
	if entry.Diff == nil || len(entry.Diff.Fields) == 0 {
		t.Fatalf("expected non-empty diff for new_version; got %+v", entry.Diff)
	}

	changed := make(map[string]FieldDiff)
	for _, f := range entry.Diff.Fields {
		changed[f.Field] = f
	}
	for _, want := range []string{"spec.name", "spec.description", "spec.business_owner", "spec.technical_owner"} {
		if _, ok := changed[want]; !ok {
			t.Errorf("expected diff to include %q; got fields=%v", want, keysOf(changed))
		}
	}
	if f, ok := changed["spec.name"]; ok {
		if f.Before != "Old Name" || f.After != "New Name" {
			t.Errorf("spec.name diff wrong: before=%v after=%v", f.Before, f.After)
		}
	}

	// Wire mapping must preserve create_kind and diff.
	result := PlanResultFromPlan(plan)
	if result.Entries[0].CreateKind != string(CreateKindNewVersion) {
		t.Errorf("wire create_kind = %q, want %q", result.Entries[0].CreateKind, CreateKindNewVersion)
	}
	if result.Entries[0].Diff == nil || len(result.Entries[0].Diff.Fields) == 0 {
		t.Errorf("wire diff must be populated; got %+v", result.Entries[0].Diff)
	}
}

func TestPlan_VersionedSurface_NoFieldChanges_NoDiff(t *testing.T) {
	// Build a baseline that exactly matches the document after normalisation.
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{
				ID:                 id,
				Version:            1,
				Status:             surface.SurfaceStatusActive,
				Name:               "Valid Surface",
				Description:        "",
				Domain:             "default",
				Category:           "financial",
				MinimumConfidence:  0,
				DecisionType:       surface.DecisionTypeOperational,
				ReversibilityClass: surface.ReversibilityConditionallyReversible,
				FailureMode:        surface.FailureModeClosed,
				BusinessOwner:      "unassigned",
				TechnicalOwner:     "unassigned",
				ProcessID:          "test.process",
			}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Processes: processRepoAlwaysExists()})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{validSurface("surf-same")})
	entry := findEntry(t, plan, types.KindSurface, "surf-same")

	if entry.CreateKind != CreateKindNewVersion {
		t.Fatalf("want create_kind=%s, got %s", CreateKindNewVersion, entry.CreateKind)
	}
	if entry.Diff != nil {
		t.Errorf("no-change new_version must emit nil diff; got %+v", entry.Diff)
	}
}

// ---------------------------------------------------------------------------
// create_kind — versioned Profile (new_version)
// ---------------------------------------------------------------------------

func TestPlan_VersionedProfile_CreateKindNewVersion_EmitsDiff(t *testing.T) {
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, id string) (*authority.AuthorityProfile, error) {
			return &authority.AuthorityProfile{
				ID:                   id,
				Version:              1,
				Status:               authority.ProfileStatusActive,
				Name:                 "Old Profile Name",
				SurfaceID:            "payment.execute",
				ConfidenceThreshold:  0.5,
				PolicyReference:      "rego://old",
				FailMode:             authority.FailModeClosed,
			}, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Profiles: profileRepo})

	// Build a profile whose name, confidence threshold, and policy reference
	// differ from the baseline.
	doc := parser.ParsedDocument{
		Kind: types.KindProfile,
		ID:   "profile-v2",
		Doc: types.ProfileDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProfile,
			Metadata:   types.DocumentMetadata{ID: "profile-v2", Name: "New Profile Name"},
			Spec: types.ProfileSpec{
				SurfaceID: "payment.execute",
				Authority: types.ProfileAuthority{
					DecisionConfidenceThreshold: 0.9,
					ConsequenceThreshold: types.ConsequenceThreshold{
						Type: "monetary", Amount: 10000, Currency: "USD",
					},
				},
				Policy: types.ProfilePolicy{Reference: "rego://new", FailMode: "closed"},
			},
		},
	}

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{doc})
	entry := findEntry(t, plan, types.KindProfile, "profile-v2")

	if entry.Action != ApplyActionCreate {
		t.Fatalf("expected action=create, got %s", entry.Action)
	}
	if entry.CreateKind != CreateKindNewVersion {
		t.Fatalf("expected create_kind=%s, got %s", CreateKindNewVersion, entry.CreateKind)
	}
	if entry.Diff == nil || len(entry.Diff.Fields) == 0 {
		t.Fatalf("expected non-empty diff for versioned profile; got %+v", entry.Diff)
	}

	changed := make(map[string]FieldDiff)
	for _, f := range entry.Diff.Fields {
		changed[f.Field] = f
	}
	for _, want := range []string{
		"metadata.name",
		"spec.authority.decision_confidence_threshold",
		"spec.policy.reference",
	} {
		if _, ok := changed[want]; !ok {
			t.Errorf("expected diff to include %q; got fields=%v", want, keysOf(changed))
		}
	}

	result := PlanResultFromPlan(plan)
	if result.Entries[0].CreateKind != string(CreateKindNewVersion) {
		t.Errorf("wire create_kind = %q, want %q", result.Entries[0].CreateKind, CreateKindNewVersion)
	}
	if result.Entries[0].Diff == nil {
		t.Errorf("wire diff must be populated for versioned Profile")
	}
}

// ---------------------------------------------------------------------------
// diff — non-Surface/Profile kinds do not carry a diff
// ---------------------------------------------------------------------------

func TestPlan_NonSurfaceProfile_CreateHasNoDiff(t *testing.T) {
	agentRepo := &controlledAgentRepo{
		getByIDFn: func(_ context.Context, _ string) (*agent.Agent, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Agents: agentRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{validAgent("agent-new")})
	entry := findEntry(t, plan, types.KindAgent, "agent-new")

	if entry.Action != ApplyActionCreate {
		t.Fatalf("expected create, got %s", entry.Action)
	}
	if entry.CreateKind != CreateKindNew {
		t.Errorf("expected create_kind=new, got %s", entry.CreateKind)
	}
	if entry.Diff != nil {
		t.Errorf("Agent create must have no diff; got %+v", entry.Diff)
	}

	result := PlanResultFromPlan(plan)
	if result.Entries[0].Diff != nil {
		t.Errorf("wire diff must be nil for Agent create; got %+v", result.Entries[0].Diff)
	}
}

// ---------------------------------------------------------------------------
// Wire regression — create_kind and diff omitted on non-create entries
// ---------------------------------------------------------------------------

func TestPlanResultFromPlan_NonCreate_OmitsCreateKindAndDiff(t *testing.T) {
	// Manually build a plan with conflict and invalid entries that happen to
	// carry CreateKind and Diff (as if internal state were set). The wire
	// mapping must drop them for non-create entries.
	plan := ApplyPlan{
		Entries: []ApplyPlanEntry{
			{
				Kind:       types.KindSurface,
				ID:         "surf-conflict",
				Action:     ApplyActionConflict,
				CreateKind: CreateKindNewVersion,
				Diff:       &PlanDiff{Fields: []FieldDiff{{Field: "spec.name", Before: "a", After: "b"}}},
			},
			{
				Kind:       types.KindSurface,
				ID:         "surf-invalid",
				Action:     ApplyActionInvalid,
				CreateKind: CreateKindNew,
				Diff:       &PlanDiff{Fields: []FieldDiff{{Field: "spec.name", Before: "a", After: "b"}}},
			},
		},
	}
	result := PlanResultFromPlan(plan)

	for _, e := range result.Entries {
		if e.CreateKind != "" {
			t.Errorf("%s entry must not carry create_kind; got %q", e.Action, e.CreateKind)
		}
		if e.Diff != nil {
			t.Errorf("%s entry must not carry diff; got %+v", e.Action, e.Diff)
		}
	}
}

// ---------------------------------------------------------------------------
// Regression — existing action/decision_source/counts unchanged by feature
// ---------------------------------------------------------------------------

func TestPlan_WarningOnly_WouldApplyUnchanged(t *testing.T) {
	// Pure advisory: a single profile referencing a deprecated surface. The
	// plan must be applyable (would_apply=true, create_count=1, no invalid,
	// no conflict) and carry the warning.
	surfaceRepo := &controlledSurfaceRepo{
		findLatestFn: func(_ context.Context, id string) (*surface.DecisionSurface, error) {
			return &surface.DecisionSurface{ID: id, Status: surface.SurfaceStatusRetired}, nil
		},
	}
	profileRepo := &controlledProfileRepo{
		findByIDFn: func(_ context.Context, _ string) (*authority.AuthorityProfile, error) {
			return nil, nil
		},
	}
	svc := NewServiceWithRepos(RepositorySet{Surfaces: surfaceRepo, Profiles: profileRepo})

	plan := svc.Plan(context.Background(), []parser.ParsedDocument{
		validProfileWithSurface("profile-w", "surf-retired"),
	})
	result := PlanResultFromPlan(plan)

	if !result.WouldApply {
		t.Errorf("would_apply must be true when plan has only warnings")
	}
	if result.InvalidCount != 0 {
		t.Errorf("invalid_count = %d, want 0", result.InvalidCount)
	}
	if result.ConflictCount != 0 {
		t.Errorf("conflict_count = %d, want 0", result.ConflictCount)
	}
	if result.CreateCount != 1 {
		t.Errorf("create_count = %d, want 1", result.CreateCount)
	}
	if len(result.Entries[0].Warnings) == 0 {
		t.Errorf("expected at least one warning on warning-only plan")
	}
}

// keysOf is a small helper used for clearer failure messages.
func keysOf(m map[string]FieldDiff) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
