package authoritygraph

// projection_test.go — wire-shape and constant pins for the Authority
// Graph projection. Pins:
//
//   - JSON tag set for Projection / Node / Edge / NodeRef
//   - presence of the six MVP node-kind constants and only the six
//   - presence of the six MVP edge-kind constants and only the six
//   - forbidden node kinds (process, ai_system, ai_system_binding,
//     coverage, authority_summary) and forbidden edge kinds
//     (has_capability, has_process, has_surface, relates_to,
//     bound_to, system_of, summarises, reports_coverage) MUST NOT
//     exist in this package — exposed via reflection over the
//     package's exported constants
//   - ParseDepth defaulting, clamping, and validation
//   - ConsequenceThreshold + RuleCountByClass wire shape

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestProjection_JSONShape pins the wire-format field set for
// Projection — every key the handler ships must be present and no
// stray keys may slip in.
func TestProjection_JSONShape(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 4,
		Nodes: []Node{},
		Edges: []Edge{},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{`"root":`, `"view":`, `"depth":`, `"nodes":`, `"edges":`} {
		if !strings.Contains(s, want) {
			t.Errorf("Projection JSON must contain %q; got %s", want, s)
		}
	}
	// Forbidden keys — Authority Graph does NOT carry Phase 2A
	// concepts the Context Graph used.
	for _, illegal := range []string{`"attrs"`, `"truncated"`, `"authority_summary"`, `"coverage"`} {
		if strings.Contains(s, illegal) {
			t.Errorf("Projection JSON must NOT contain %q; got %s", illegal, s)
		}
	}
}

// TestNode_TypedSlots_OmittedWhenAbsent pins that the optional typed
// data pointers collapse via omitempty when nil. This is load-bearing
// for the compact-per-kind wire shape: a business_service node must
// not carry a stray `"authority_profile": null` field.
func TestNode_TypedSlots_OmittedWhenAbsent(t *testing.T) {
	n := Node{Kind: NodeKindBusinessService, ID: "bs-1", Label: "BS One"}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{`"kind":"business_service"`, `"id":"bs-1"`, `"label":"BS One"`} {
		if !strings.Contains(s, want) {
			t.Errorf("Node JSON missing required key %q; got %s", want, s)
		}
	}
	for _, illegal := range []string{
		`"business_service":null`,
		`"decision_surface":`,
		`"authority_profile":`,
		`"authority_grant":`,
		`"agent":`,
		`"fail_mode_policy":`,
		`"escalation_target":`,
	} {
		if strings.Contains(s, illegal) {
			t.Errorf("Node JSON must NOT contain %q when typed data is absent; got %s", illegal, s)
		}
	}
}

// TestNode_BusinessServiceData_Populated confirms that when the
// typed pointer IS set, the slot serialises and other slots remain
// omitted.
func TestNode_BusinessServiceData_Populated(t *testing.T) {
	n := Node{
		Kind:  NodeKindBusinessService,
		ID:    "bs-1",
		Label: "BS One",
		BusinessService: &BusinessServiceData{
			ID:          "bs-1",
			Name:        "BS One",
			Status:      "active",
			ServiceType: "internal",
		},
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"business_service":{`) {
		t.Errorf("business_service typed-data slot missing; got %s", s)
	}
	if !strings.Contains(s, `"service_type":"internal"`) {
		t.Errorf("BS typed data missing service_type; got %s", s)
	}
}

// TestNodeKindConstants pins exactly the seven Authority Graph node
// kinds (D31l added escalation_target as the seventh).
func TestNodeKindConstants(t *testing.T) {
	got := []string{
		NodeKindBusinessService,
		NodeKindDecisionSurface,
		NodeKindAuthorityProfile,
		NodeKindAuthorityGrant,
		NodeKindAgent,
		NodeKindFailModePolicy,
		NodeKindEscalationTarget,
	}
	want := []string{
		"business_service",
		"decision_surface",
		"authority_profile",
		"authority_grant",
		"agent",
		"fail_mode_policy",
		"escalation_target",
	}
	if len(got) != 7 {
		t.Fatalf("NodeKind constants: want exactly 7, got %d", len(got))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("NodeKind[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

// TestEdgeKindConstants pins exactly the seven Authority Graph edge
// kinds (D31l added profile_escalates_to as the seventh).
func TestEdgeKindConstants(t *testing.T) {
	got := []string{
		EdgeKindBusinessServiceHasSurface,
		EdgeKindSurfaceUsesProfile,
		EdgeKindProfileHasGrant,
		EdgeKindGrantAuthorisesAgent,
		EdgeKindSurfaceHasFailModePolicy,
		EdgeKindBusinessServiceHasFailModePolicy,
		EdgeKindProfileEscalatesTo,
	}
	want := []string{
		"business_service_has_surface",
		"surface_uses_profile",
		"profile_has_grant",
		"grant_authorises_agent",
		"surface_has_fail_mode_policy",
		"business_service_has_fail_mode_policy",
		"profile_escalates_to",
	}
	if len(got) != 7 {
		t.Fatalf("EdgeKind constants: want exactly 7, got %d", len(got))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("EdgeKind[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

// TestEdgeLabelConstants pins the two fail-mode-policy edge labels.
func TestEdgeLabelConstants(t *testing.T) {
	if EdgeLabelDefault != "default" {
		t.Errorf("EdgeLabelDefault: want %q, got %q", "default", EdgeLabelDefault)
	}
	if EdgeLabelOverride != "override" {
		t.Errorf("EdgeLabelOverride: want %q, got %q", "override", EdgeLabelOverride)
	}
}

// TestDepthConstants pins MVP defaults.
func TestDepthConstants(t *testing.T) {
	if DefaultDepth != 4 {
		t.Errorf("DefaultDepth: want 4, got %d", DefaultDepth)
	}
	if MaxDepth != 5 {
		t.Errorf("MaxDepth: want 5, got %d", MaxDepth)
	}
	if ViewService != "service" {
		t.Errorf("ViewService: want %q, got %q", "service", ViewService)
	}
}

// TestParseDepth pins the default-and-clamp behaviour.
func TestParseDepth(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", DefaultDepth, false},
		{"0", 0, false},
		{"3", 3, false},
		{"5", 5, false},
		{"6", MaxDepth, false},   // clamped silently
		{"999", MaxDepth, false}, // clamped silently
		{"-1", 0, true},
		{"abc", 0, true},
		{"1.5", 0, true},
	}
	for _, c := range cases {
		got, err := ParseDepth(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseDepth(%q): want error, got nil (n=%d)", c.in, got)
			}
			if !errors.Is(err, ErrInvalidDepth) {
				t.Errorf("ParseDepth(%q): want ErrInvalidDepth, got %v", c.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDepth(%q): unexpected error %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseDepth(%q): want %d, got %d", c.in, c.want, got)
		}
	}
}

// TestErrorsHaveAuthorityGraphPrefix pins that sentinel error
// strings carry the package's prefix so operators / log consumers
// can identify the source.
func TestErrorsHaveAuthorityGraphPrefix(t *testing.T) {
	for _, e := range []error{ErrInvalidView, ErrInvalidID, ErrInvalidDepth, ErrNotFound} {
		if !strings.HasPrefix(e.Error(), "authoritygraph:") {
			t.Errorf("error %q must have prefix %q", e.Error(), "authoritygraph:")
		}
	}
}

// TestConsequenceThresholdData_OmitEmpty pins that an unset
// consequence threshold marshals to the zero shape rather than
// stuffing zero defaults.
func TestConsequenceThresholdData_OmitEmpty(t *testing.T) {
	c := ConsequenceThresholdData{Type: "monetary", Amount: 50000, Currency: "USD"}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"type":"monetary"`) {
		t.Errorf("ConsequenceThresholdData missing type; got %s", s)
	}
	if !strings.Contains(s, `"amount":50000`) {
		t.Errorf("ConsequenceThresholdData missing amount; got %s", s)
	}
	if strings.Contains(s, `"risk_rating":""`) {
		t.Errorf("ConsequenceThresholdData must omit empty risk_rating; got %s", s)
	}
}

// TestRuleCountByClass_AllClasses pins that the count struct has
// exactly the five correctness-class fields.
func TestRuleCountByClass_AllClasses(t *testing.T) {
	r := RuleCountByClassData{
		GovernanceIntegrity: 1,
		Persistence:         2,
		Input:               3,
		Resource:            4,
		Consistency:         5,
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"governance_integrity":1`,
		`"persistence":2`,
		`"input":3`,
		`"resource":4`,
		`"consistency":5`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("RuleCountByClass JSON missing %q; got %s", want, s)
		}
	}
}

// ---------------------------------------------------------------------------
// D31g — Summary, Diagnostics, validity/source constants
// ---------------------------------------------------------------------------

// TestProjection_Summary_OmitEmpty pins that Summary is omitted from
// the wire payload when nil — keeps the response compact for clients
// that don't yet consume it.
func TestProjection_Summary_OmitEmpty(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 4,
		Nodes: []Node{},
		Edges: []Edge{},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), `"summary":`) {
		t.Errorf("Projection JSON must omit summary when nil; got %s", b)
	}
	if strings.Contains(string(b), `"diagnostics":`) {
		t.Errorf("Projection JSON must omit diagnostics when empty; got %s", b)
	}
}

// TestProjection_Summary_Populated pins the wire shape of every
// summary field when populated.
func TestProjection_Summary_Populated(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 4,
		Nodes: []Node{},
		Edges: []Edge{},
		Summary: &Summary{
			SurfaceCount:                           3,
			ActiveProfileCount:                     2,
			ActiveGrantCount:                       2,
			ActiveAgentCount:                       1,
			FailModePolicyCount:                    1,
			CompleteAuthorityPaths:                 1,
			IncompleteAuthorityPaths:               2,
			SurfacesWithPolicyOverride:             0,
			SurfacesInheritingBSPolicy:             3,
			SurfacesWithoutProfiles:                []NodeRef{{Kind: NodeKindDecisionSurface, ID: "surf-x"}},
			ProfilesWithoutGrants:                  []NodeRef{{Kind: NodeKindAuthorityProfile, ID: "prof-x"}},
			GrantsWithoutAgents:                    []NodeRef{{Kind: NodeKindAuthorityGrant, ID: "grant-x"}},
			SurfacesWithoutEffectiveFailModePolicy: []NodeRef{{Kind: NodeKindDecisionSurface, ID: "surf-y"}},
			PoliciesMissingActiveVersion:           []NodeRef{{Kind: NodeKindFailModePolicy, ID: "fmp-missing"}},
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"summary":{`,
		`"surface_count":3`,
		`"active_profile_count":2`,
		`"active_grant_count":2`,
		`"active_agent_count":1`,
		`"fail_mode_policy_count":1`,
		`"complete_authority_paths":1`,
		`"incomplete_authority_paths":2`,
		`"surfaces_with_policy_override":0`,
		`"surfaces_inheriting_bs_policy":3`,
		`"surfaces_without_profiles":[`,
		`"profiles_without_grants":[`,
		`"grants_without_agents":[`,
		`"surfaces_without_effective_fail_mode_policy":[`,
		`"policies_missing_active_version":[`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary JSON missing %q; got %s", want, s)
		}
	}
}

// TestProjection_Diagnostics_WireShape pins the Diagnostic JSON tags.
func TestProjection_Diagnostics_WireShape(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 4,
		Nodes: []Node{},
		Edges: []Edge{},
		Diagnostics: []Diagnostic{
			{
				Kind:     DiagnosticKindBusinessServiceHasNoActiveSurface,
				Severity: DiagnosticSeverityWarning,
				NodeRefs: []NodeRef{{Kind: NodeKindBusinessService, ID: "bs-1"}},
				Message:  "no active surfaces",
			},
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"diagnostics":[`,
		`"kind":"business_service_has_no_active_surface"`,
		`"severity":"warning"`,
		`"node_refs":[`,
		`"message":"no active surfaces"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Diagnostic JSON missing %q; got %s", want, s)
		}
	}
}

// TestDiagnosticSeverityConstants pins the three operator-facing
// severities. The OpenAPI enum and the diagnostic sort comparator
// both depend on these exact values.
func TestDiagnosticSeverityConstants(t *testing.T) {
	if DiagnosticSeverityCritical != "critical" {
		t.Errorf("DiagnosticSeverityCritical: want %q, got %q", "critical", DiagnosticSeverityCritical)
	}
	if DiagnosticSeverityWarning != "warning" {
		t.Errorf("DiagnosticSeverityWarning: want %q, got %q", "warning", DiagnosticSeverityWarning)
	}
	if DiagnosticSeverityInfo != "info" {
		t.Errorf("DiagnosticSeverityInfo: want %q, got %q", "info", DiagnosticSeverityInfo)
	}
}

// TestDiagnosticKindConstants pins the diagnostic kinds emitted by
// the Authority Graph projection. D31i added grant_has_no_capabilities
// and D31l added profile_has_no_escalation_target +
// escalation_target_reference_dangling.
func TestDiagnosticKindConstants(t *testing.T) {
	got := []string{
		DiagnosticKindBusinessServiceHasNoActiveSurface,
		DiagnosticKindSurfaceHasNoActiveProfile,
		DiagnosticKindProfileHasNoActiveGrant,
		DiagnosticKindGrantReferencesMissingAgent,
		DiagnosticKindGrantReferencesInactiveAgent,
		DiagnosticKindFailModePolicyReferenceDangling,
		DiagnosticKindSurfaceInheritsBusinessServicePolicy,
		DiagnosticKindSurfaceOverridesBusinessServiceDefault,
		DiagnosticKindSurfaceOverrideMatchesBSDefault,
		DiagnosticKindProfileFutureDated,
		DiagnosticKindProfileExpired,
		DiagnosticKindGrantFutureDated,
		DiagnosticKindGrantExpired,
		DiagnosticKindDuplicateActiveProfileVersionsForSurface,
		DiagnosticKindGrantHasNoCapabilities,
		DiagnosticKindProfileHasNoEscalationTarget,
		DiagnosticKindEscalationTargetReferenceDangling,
	}
	want := []string{
		"business_service_has_no_active_surface",
		"surface_has_no_active_profile",
		"profile_has_no_active_grant",
		"grant_references_missing_agent",
		"grant_references_inactive_agent",
		"fail_mode_policy_reference_dangling",
		"surface_inherits_business_service_policy",
		"surface_overrides_business_service_default",
		"surface_override_matches_business_service_default",
		"profile_future_dated",
		"profile_expired",
		"grant_future_dated",
		"grant_expired",
		"duplicate_active_profile_versions_for_surface",
		"grant_has_no_capabilities",
		"profile_has_no_escalation_target",
		"escalation_target_reference_dangling",
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("DiagnosticKind[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

// TestEscalationTargetData_WireShape pins the JSON shape of the
// D31l EscalationTargetData typed-data payload. Operators rely on
// these keys to read the resolved target without traversing the
// authority spine.
func TestEscalationTargetData_WireShape(t *testing.T) {
	approved := "2026-01-15T12:00:00Z"
	until := "2026-12-31T00:00:00Z"
	d := EscalationTargetData{
		ID:             "et-1",
		Version:        2,
		Name:           "Governance Approver",
		Description:    "primary",
		Kind:           "role",
		Handle:         "governance.approver",
		Status:         "active",
		EffectiveDate:  &approved,
		EffectiveUntil: &until,
		BusinessOwner:  "ops",
		TechnicalOwner: "platform",
		ApprovedBy:     "alice",
		ApprovedAt:     &approved,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"id":"et-1"`,
		`"version":2`,
		`"name":"Governance Approver"`,
		`"description":"primary"`,
		`"kind":"role"`,
		`"handle":"governance.approver"`,
		`"status":"active"`,
		`"effective_date":"2026-01-15T12:00:00Z"`,
		`"effective_until":"2026-12-31T00:00:00Z"`,
		`"business_owner":"ops"`,
		`"technical_owner":"platform"`,
		`"approved_by":"alice"`,
		`"approved_at":"2026-01-15T12:00:00Z"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("EscalationTargetData JSON missing %q; got %s", want, s)
		}
	}
}

// TestEscalationTargetData_OmitEmpty pins that omitempty-tagged
// fields collapse when zero.
func TestEscalationTargetData_OmitEmpty(t *testing.T) {
	d := EscalationTargetData{
		ID: "et-1", Version: 1, Name: "n", Kind: "role", Handle: "h", Status: "active",
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, illegal := range []string{
		`"description":""`,
		`"effective_date":`,
		`"effective_until":`,
		`"business_owner":""`,
		`"technical_owner":""`,
		`"approved_by":""`,
		`"approved_at":`,
	} {
		if strings.Contains(s, illegal) {
			t.Errorf("EscalationTargetData JSON must omit %q when empty; got %s", illegal, s)
		}
	}
}

// TestAuthorityProfileData_EscalationTargetIDField pins the D31l
// additive escalation_target_id property on the profile typed data.
func TestAuthorityProfileData_EscalationTargetIDField(t *testing.T) {
	d := AuthorityProfileData{
		ID:                 "prof-1",
		Version:            1,
		SurfaceID:          "surf-1",
		Name:               "Profile",
		Status:             "active",
		ConfidenceThreshold: 0.8,
		EscalationTargetID: "et-approver",
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"escalation_target_id":"et-approver"`) {
		t.Errorf("AuthorityProfileData JSON missing escalation_target_id; got %s", b)
	}
	empty := AuthorityProfileData{
		ID: "p", Version: 1, SurfaceID: "s", Name: "n", Status: "active",
		ConfidenceThreshold: 0.5,
	}
	be, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if strings.Contains(string(be), `"escalation_target_id"`) {
		t.Errorf("escalation_target_id must omitempty when empty; got %s", be)
	}
}

// TestSummary_EscalationTargetFields pins the D31l summary JSON tags.
func TestSummary_EscalationTargetFields(t *testing.T) {
	sm := Summary{
		EscalationTargetCount:        2,
		ProfilesWithEscalationTarget: 3,
		ProfilesWithoutEscalationTarget: []NodeRef{
			{Kind: NodeKindAuthorityProfile, ID: "prof-a"},
		},
		ProfilesWithDanglingEscalationTarget: []NodeRef{
			{Kind: NodeKindAuthorityProfile, ID: "prof-b"},
		},
	}
	b, err := json.Marshal(sm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"escalation_target_count":2`,
		`"profiles_with_escalation_target":3`,
		`"profiles_without_escalation_target":[`,
		`"profiles_with_dangling_escalation_target":[`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Summary JSON missing %q; got %s", want, s)
		}
	}
}

// TestValidityStatusConstants pins the three validity values.
func TestValidityStatusConstants(t *testing.T) {
	if ValidityStatusEffective != "effective" {
		t.Errorf("ValidityStatusEffective: want %q, got %q", "effective", ValidityStatusEffective)
	}
	if ValidityStatusFutureDated != "future_dated" {
		t.Errorf("ValidityStatusFutureDated: want %q, got %q", "future_dated", ValidityStatusFutureDated)
	}
	if ValidityStatusExpired != "expired" {
		t.Errorf("ValidityStatusExpired: want %q, got %q", "expired", ValidityStatusExpired)
	}
}

// TestEffectivePolicySourceConstants pins the three effective-policy
// source values.
func TestEffectivePolicySourceConstants(t *testing.T) {
	if EffectivePolicySourceOverride != "override" {
		t.Errorf("EffectivePolicySourceOverride: want %q, got %q", "override", EffectivePolicySourceOverride)
	}
	if EffectivePolicySourceBusinessServiceDefault != "business_service_default" {
		t.Errorf("EffectivePolicySourceBusinessServiceDefault: want %q, got %q", "business_service_default", EffectivePolicySourceBusinessServiceDefault)
	}
	if EffectivePolicySourceNone != "none" {
		t.Errorf("EffectivePolicySourceNone: want %q, got %q", "none", EffectivePolicySourceNone)
	}
}

// TestDecisionSurfaceData_EffectivePolicyFields pins the new
// effective-policy fields on the surface typed data and their
// omitempty behaviour.
func TestDecisionSurfaceData_EffectivePolicyFields(t *testing.T) {
	d := DecisionSurfaceData{
		ID: "surf-1", Version: 1, Name: "Surface", Status: "active",
		EffectivePolicySource:  EffectivePolicySourceOverride,
		EffectivePolicyID:      "fmp-1",
		EffectivePolicyVersion: 2,
		InheritsBSPolicy:       false,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"effective_policy_source":"override"`,
		`"effective_policy_id":"fmp-1"`,
		`"effective_policy_version":2`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("DecisionSurfaceData JSON missing %q; got %s", want, s)
		}
	}
	// inherits_bs_policy is false (zero) and omitempty — must NOT appear.
	if strings.Contains(s, `"inherits_bs_policy":false`) {
		t.Errorf("inherits_bs_policy must omit zero value; got %s", s)
	}
}

// TestAuthorityProfileData_ValidityFields pins effective_until +
// validity_status on profile data.
func TestAuthorityProfileData_ValidityFields(t *testing.T) {
	now := "2026-06-01T00:00:00Z"
	d := AuthorityProfileData{
		ID:                  "prof-1",
		Version:             1,
		SurfaceID:           "surf-1",
		Name:                "Profile",
		Status:              "active",
		EffectiveUntil:      &now,
		ValidityStatus:      ValidityStatusEffective,
		ConfidenceThreshold: 0.8,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"effective_until":"2026-06-01T00:00:00Z"`,
		`"validity_status":"effective"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("AuthorityProfileData JSON missing %q; got %s", want, s)
		}
	}
}

// TestAuthorityGrantData_ValidityField pins validity_status on grant data.
func TestAuthorityGrantData_ValidityField(t *testing.T) {
	d := AuthorityGrantData{
		ID: "grant-1", ProfileID: "prof-1", AgentID: "agent-1",
		Status:         "active",
		ValidityStatus: ValidityStatusEffective,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"validity_status":"effective"`) {
		t.Errorf("AuthorityGrantData JSON missing validity_status; got %s", b)
	}
}

// TestFailModePolicyData_EffectiveUntilField pins effective_until on
// policy data.
func TestFailModePolicyData_EffectiveUntilField(t *testing.T) {
	now := "2026-12-31T00:00:00Z"
	d := FailModePolicyData{
		ID: "fmp-1", Version: 1, Name: "Policy", Status: "active",
		EffectiveUntil: &now,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"effective_until":"2026-12-31T00:00:00Z"`) {
		t.Errorf("FailModePolicyData JSON missing effective_until; got %s", b)
	}
}

// ---------------------------------------------------------------------------
// D31m — DiagnosticSummary + SurfaceAuthorityPosture
// ---------------------------------------------------------------------------

// TestHighestSeverityConstants pins the four-value rollup vocabulary.
// The three non-"none" values share string identity with the
// existing diagnostic severities so the rollup uses canonical
// literals.
func TestHighestSeverityConstants(t *testing.T) {
	if HighestSeverityNone != "none" {
		t.Errorf("HighestSeverityNone: want %q, got %q", "none", HighestSeverityNone)
	}
	if HighestSeverityInfo != DiagnosticSeverityInfo {
		t.Errorf("HighestSeverityInfo must alias DiagnosticSeverityInfo")
	}
	if HighestSeverityWarning != DiagnosticSeverityWarning {
		t.Errorf("HighestSeverityWarning must alias DiagnosticSeverityWarning")
	}
	if HighestSeverityCritical != DiagnosticSeverityCritical {
		t.Errorf("HighestSeverityCritical must alias DiagnosticSeverityCritical")
	}
}

// TestAuthorityStatusConstants pins the exact four-value vocabulary.
func TestAuthorityStatusConstants(t *testing.T) {
	got := []string{
		AuthorityStatusComplete,
		AuthorityStatusIncomplete,
		AuthorityStatusDegraded,
		AuthorityStatusUncovered,
	}
	want := []string{"complete", "incomplete", "degraded", "uncovered"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("AuthorityStatus[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

func TestProfileStatusConstants(t *testing.T) {
	if ProfileStatusCovered != "covered" {
		t.Errorf("ProfileStatusCovered: want %q, got %q", "covered", ProfileStatusCovered)
	}
	if ProfileStatusMissing != "missing" {
		t.Errorf("ProfileStatusMissing: want %q, got %q", "missing", ProfileStatusMissing)
	}
}

func TestGrantStatusConstants(t *testing.T) {
	if GrantStatusCovered != "covered" {
		t.Errorf("GrantStatusCovered: want %q, got %q", "covered", GrantStatusCovered)
	}
	if GrantStatusMissing != "missing" {
		t.Errorf("GrantStatusMissing: want %q, got %q", "missing", GrantStatusMissing)
	}
}

func TestAgentStatusConstants(t *testing.T) {
	if AgentStatusCovered != "covered" {
		t.Errorf("AgentStatusCovered: want %q, got %q", "covered", AgentStatusCovered)
	}
	if AgentStatusMissing != "missing" {
		t.Errorf("AgentStatusMissing: want %q, got %q", "missing", AgentStatusMissing)
	}
	if AgentStatusBlocked != "blocked" {
		t.Errorf("AgentStatusBlocked: want %q, got %q", "blocked", AgentStatusBlocked)
	}
}

func TestFailModePolicyStatusConstants(t *testing.T) {
	if FailModePolicyStatusOverride != "override" {
		t.Errorf("FailModePolicyStatusOverride: want %q, got %q", "override", FailModePolicyStatusOverride)
	}
	if FailModePolicyStatusInherited != "inherited" {
		t.Errorf("FailModePolicyStatusInherited: want %q, got %q", "inherited", FailModePolicyStatusInherited)
	}
	if FailModePolicyStatusMissing != "missing" {
		t.Errorf("FailModePolicyStatusMissing: want %q, got %q", "missing", FailModePolicyStatusMissing)
	}
	if FailModePolicyStatusDangling != "dangling" {
		t.Errorf("FailModePolicyStatusDangling: want %q, got %q", "dangling", FailModePolicyStatusDangling)
	}
}

func TestEscalationStatusConstants(t *testing.T) {
	if EscalationStatusTargeted != "targeted" {
		t.Errorf("EscalationStatusTargeted: want %q, got %q", "targeted", EscalationStatusTargeted)
	}
	if EscalationStatusNotTargeted != "not_targeted" {
		t.Errorf("EscalationStatusNotTargeted: want %q, got %q", "not_targeted", EscalationStatusNotTargeted)
	}
	if EscalationStatusDangling != "dangling" {
		t.Errorf("EscalationStatusDangling: want %q, got %q", "dangling", EscalationStatusDangling)
	}
}

// TestProjection_D31mFields_OmittedWhenAbsent pins the omitempty
// behaviour for the new top-level fields. A bare projection (no
// posture, no rollup) must NOT carry the keys.
func TestProjection_D31mFields_OmittedWhenAbsent(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 4,
		Nodes: []Node{},
		Edges: []Edge{},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, illegal := range []string{`"diagnostic_summary":`, `"surface_posture":`} {
		if strings.Contains(s, illegal) {
			t.Errorf("Projection JSON must omit %q when absent; got %s", illegal, s)
		}
	}
}

// TestProjection_D31mFields_PresentWhenPopulated pins that the
// fields serialise with the expected JSON keys when set.
func TestProjection_D31mFields_PresentWhenPopulated(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 4,
		Nodes: []Node{},
		Edges: []Edge{},
		DiagnosticSummary: &DiagnosticSummary{
			Info: 1, Warning: 2, Critical: 0,
			HighestSeverity: HighestSeverityWarning,
			ByKind:          map[string]int{"grant_has_no_capabilities": 1, "profile_has_no_escalation_target": 1, "fail_mode_policy_reference_dangling": 1},
		},
		SurfacePosture: []SurfaceAuthorityPosture{
			{
				Surface:              NodeRef{Kind: NodeKindDecisionSurface, ID: "surf-1"},
				AuthorityStatus:      AuthorityStatusDegraded,
				ProfileStatus:        ProfileStatusCovered,
				GrantStatus:          GrantStatusCovered,
				AgentStatus:          AgentStatusCovered,
				FailModePolicyStatus: FailModePolicyStatusInherited,
				EscalationStatus:     EscalationStatusTargeted,
				CompletePaths:        1,
				IncompletePaths:      0,
				HighestSeverity:      HighestSeverityWarning,
				DiagnosticKinds:      []string{"fail_mode_policy_reference_dangling"},
			},
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"diagnostic_summary":{`,
		`"info":1`,
		`"warning":2`,
		`"critical":0`,
		`"highest_severity":"warning"`,
		`"by_kind":{`,
		`"surface_posture":[`,
		`"surface":{"kind":"decision_surface","id":"surf-1"}`,
		`"authority_status":"degraded"`,
		`"profile_status":"covered"`,
		`"grant_status":"covered"`,
		`"agent_status":"covered"`,
		`"fail_mode_policy_status":"inherited"`,
		`"escalation_status":"targeted"`,
		`"complete_paths":1`,
		`"incomplete_paths":0`,
		`"diagnostic_kinds":[`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Projection JSON missing %q; got %s", want, s)
		}
	}
}

// TestDiagnosticSummary_OmitByKindWhenEmpty pins that ByKind
// collapses via omitempty when empty.
func TestDiagnosticSummary_OmitByKindWhenEmpty(t *testing.T) {
	d := DiagnosticSummary{HighestSeverity: HighestSeverityNone}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if strings.Contains(s, `"by_kind"`) {
		t.Errorf("DiagnosticSummary JSON must omit by_kind when empty; got %s", s)
	}
	for _, want := range []string{
		`"info":0`,
		`"warning":0`,
		`"critical":0`,
		`"highest_severity":"none"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("DiagnosticSummary JSON missing %q; got %s", want, s)
		}
	}
}
