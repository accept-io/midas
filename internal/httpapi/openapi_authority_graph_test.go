package httpapi

// openapi_authority_graph_test.go — contract assertions for the
// Authority Graph schemas added to api/openapi/v1.yaml in D31f.
//
// Mirrors the openapi_context_graph_test.go coverage shape: path
// declared, all referenced component schemas defined, top-level
// projection schema references node/edge schemas via $ref, node/edge
// kind enums constrained to the MVP allowed sets, plus boundary
// regression pins against the prior D31d state.

import (
	"os"
	"strings"
	"testing"
)

// TestOpenAPIContract_AuthorityGraphPathDeclared pins that the new
// /v1/graphs/authority operation exists with the expected
// operationId.
func TestOpenAPIContract_AuthorityGraphPathDeclared(t *testing.T) {
	specPaths := loadSpecPaths(t)
	const want = "/v1/graphs/authority"
	for _, p := range specPaths {
		if p == want {
			return
		}
	}
	t.Errorf("OpenAPI spec missing path %q", want)
}

// TestOpenAPIContract_AuthorityGraphOperationID asserts the
// operationId is getAuthorityGraph (the symmetric counterpart of
// getContextGraph).
func TestOpenAPIContract_AuthorityGraphOperationID(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read v1.yaml: %v", err)
	}
	if !strings.Contains(string(body), "operationId: getAuthorityGraph") {
		t.Error("OpenAPI spec missing `operationId: getAuthorityGraph`")
	}
}

// TestOpenAPIContract_AuthorityGraphSchemasDeclared asserts every
// AuthorityGraph* component schema the projection references is
// declared. A missing schema would produce a broken $ref at
// runtime.
func TestOpenAPIContract_AuthorityGraphSchemasDeclared(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	want := []string{
		// Core projection wrappers.
		"AuthorityGraphProjection",
		"AuthorityGraphNode",
		"AuthorityGraphEdge",
		"AuthorityGraphNodeRef",
		// Per-kind typed data (one per Authority Graph node kind).
		"AuthorityGraphBusinessServiceData",
		"AuthorityGraphDecisionSurfaceData",
		"AuthorityGraphAuthorityProfileData",
		"AuthorityGraphAuthorityGrantData",
		"AuthorityGraphAgentData",
		"AuthorityGraphFailModePolicyData",
		"AuthorityGraphEscalationTargetData", // D31l
		// Supporting embedded schemas.
		"AuthorityGraphConsequenceThreshold",
		"AuthorityGraphRuleCountByClass",
		"AuthorityGraphExternalRefData",
	}
	for _, name := range want {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphNode_KindEnum pins the MVP
// node-kind allow-list. Adding a kind in a future tranche must
// update both the Go constants and this enum; dropping a kind here
// without updating the projection would produce a wire-shape
// regression.
func TestOpenAPIContract_AuthorityGraphNode_KindEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	node, ok := schemas["AuthorityGraphNode"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode schema not a map")
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode has no properties map")
	}
	kindProp, ok := props["kind"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode.properties.kind not a map")
	}
	enum, ok := kindProp["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphNode.properties.kind.enum missing")
	}
	wantSet := map[string]bool{
		"business_service":   true,
		"decision_surface":   true,
		"authority_profile":  true,
		"authority_grant":    true,
		"agent":              true,
		"fail_mode_policy":   true,
		"escalation_target":  true,
	}
	gotSet := map[string]bool{}
	for _, v := range enum {
		s, _ := v.(string)
		gotSet[s] = true
	}
	for want := range wantSet {
		if !gotSet[want] {
			t.Errorf("node kind enum missing %q", want)
		}
	}
	if len(enum) != 7 {
		t.Errorf("node kind enum must have exactly 7 entries (D31l added escalation_target); got %d (%v)", len(enum), enum)
	}
	// Forbidden kinds — these belong in Context Graph, not Authority.
	for _, illegal := range []string{
		"process", "ai_system", "ai_system_binding",
		"related_business_service", "capability",
		"authority_summary", "coverage",
		"escalation_policy", "escalation_rule",
	} {
		if gotSet[illegal] {
			t.Errorf("Authority Graph node kind enum must NOT include forbidden kind %q", illegal)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphEdge_KindEnum pins the MVP
// edge-kind allow-list.
func TestOpenAPIContract_AuthorityGraphEdge_KindEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	edge, ok := schemas["AuthorityGraphEdge"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphEdge schema not a map")
	}
	props, ok := edge["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphEdge has no properties map")
	}
	kindProp, ok := props["kind"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphEdge.properties.kind not a map")
	}
	enum, ok := kindProp["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphEdge.properties.kind.enum missing")
	}
	wantSet := map[string]bool{
		"business_service_has_surface":          true,
		"surface_uses_profile":                  true,
		"profile_has_grant":                     true,
		"grant_authorises_agent":                true,
		"surface_has_fail_mode_policy":          true,
		"business_service_has_fail_mode_policy": true,
		"profile_escalates_to":                  true,
	}
	gotSet := map[string]bool{}
	for _, v := range enum {
		s, _ := v.(string)
		gotSet[s] = true
	}
	for want := range wantSet {
		if !gotSet[want] {
			t.Errorf("edge kind enum missing %q", want)
		}
	}
	if len(enum) != 7 {
		t.Errorf("edge kind enum must have exactly 7 entries (D31l added profile_escalates_to); got %d (%v)", len(enum), enum)
	}
	// Forbidden edge kinds (Context Graph vocabulary).
	for _, illegal := range []string{
		"relates_to", "has_capability", "has_process", "has_surface",
		"bound_to", "system_of", "summarises", "reports_coverage",
	} {
		if gotSet[illegal] {
			t.Errorf("Authority Graph edge kind enum must NOT include forbidden kind %q", illegal)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphNode_TypedDataSlots asserts that
// AuthorityGraphNode declares one optional typed-data property per
// MVP node kind, each referencing its dedicated schema. The
// properties are NOT marked required (exactly one is populated per
// node, matched to kind).
func TestOpenAPIContract_AuthorityGraphNode_TypedDataSlots(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	node, ok := schemas["AuthorityGraphNode"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode schema not a map")
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode has no properties map")
	}

	wantSlots := map[string]string{
		"business_service":   "#/components/schemas/AuthorityGraphBusinessServiceData",
		"decision_surface":   "#/components/schemas/AuthorityGraphDecisionSurfaceData",
		"authority_profile":  "#/components/schemas/AuthorityGraphAuthorityProfileData",
		"authority_grant":    "#/components/schemas/AuthorityGraphAuthorityGrantData",
		"agent":              "#/components/schemas/AuthorityGraphAgentData",
		"fail_mode_policy":   "#/components/schemas/AuthorityGraphFailModePolicyData",
		"escalation_target":  "#/components/schemas/AuthorityGraphEscalationTargetData",
	}
	for slot, wantRef := range wantSlots {
		prop, ok := props[slot].(map[string]any)
		if !ok {
			t.Errorf("AuthorityGraphNode.properties.%s missing or not a map", slot)
			continue
		}
		if got := prop["$ref"]; got != wantRef {
			t.Errorf("AuthorityGraphNode.%s.$ref: want %s, got %v", slot, wantRef, got)
		}
	}

	// Typed-data slots must NOT appear in `required`; exactly one is
	// populated per node, gated by `kind`.
	if req, ok := node["required"].([]any); ok {
		for _, slot := range []string{
			"business_service", "decision_surface", "authority_profile",
			"authority_grant", "agent", "fail_mode_policy",
			"escalation_target",
		} {
			for _, r := range req {
				if r == slot {
					t.Errorf("AuthorityGraphNode.required must NOT include typed-data slot %q (gated by kind)", slot)
				}
			}
		}
	}
}

// TestOpenAPIContract_AuthorityGraphProjection_ReferencesNodeAndEdge
// pins that the top-level projection schema references node, edge,
// and node-ref schemas via $ref — a regression that inlined the
// schema would surface here.
func TestOpenAPIContract_AuthorityGraphProjection_ReferencesNodeAndEdge(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	root, ok := schemas["AuthorityGraphProjection"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection schema not a map")
	}
	props, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection has no properties map")
	}

	checkArrayItemsRef(t, props, "nodes", "#/components/schemas/AuthorityGraphNode")
	checkArrayItemsRef(t, props, "edges", "#/components/schemas/AuthorityGraphEdge")

	rootProp, ok := props["root"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.root not a map")
	}
	if got := rootProp["$ref"]; got != "#/components/schemas/AuthorityGraphNodeRef" {
		t.Errorf("root.$ref: want AuthorityGraphNodeRef, got %v", got)
	}

	viewProp, ok := props["view"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.view not a map")
	}
	enum, ok := viewProp["enum"].([]any)
	if !ok {
		t.Fatalf("view.enum not a slice, got %v", viewProp["enum"])
	}
	// MVP: only `service` is supported.
	if len(enum) != 1 {
		t.Errorf("view.enum must have exactly 1 entry (service); got %d (%v)", len(enum), enum)
	}
	if len(enum) > 0 {
		if s, _ := enum[0].(string); s != "service" {
			t.Errorf("view.enum: want [service], got %v", enum)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphFailModePolicyData_RuleCountByClass
// pins the rule-count summary shape — five fields, one per
// CorrectnessClass value, all integers.
func TestOpenAPIContract_AuthorityGraphFailModePolicyData_RuleCountByClass(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	r, ok := schemas["AuthorityGraphRuleCountByClass"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphRuleCountByClass schema not a map")
	}
	props, ok := r["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphRuleCountByClass has no properties map")
	}
	for _, want := range []string{
		"governance_integrity", "persistence", "input", "resource", "consistency",
	} {
		p, ok := props[want].(map[string]any)
		if !ok {
			t.Errorf("AuthorityGraphRuleCountByClass.properties.%s missing", want)
			continue
		}
		if got := p["type"]; got != "integer" {
			t.Errorf("AuthorityGraphRuleCountByClass.%s.type: want integer, got %v", want, got)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphReservationCommentRemoved
// confirms the D31d "future Authority Graph" reservation note in
// the spec has been updated to reflect D31f's implementation. The
// implementation comment must mention D31f and authority spine.
func TestOpenAPIContract_AuthorityGraphReservationCommentRemoved(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read v1.yaml: %v", err)
	}
	src := string(body)
	// D31d phrase "future Authority Graph (D31e+)" must be gone now
	// that the endpoint is implemented.
	if strings.Contains(src, "future Authority Graph (D31e+)") {
		t.Error("OpenAPI spec still describes Authority Graph as future; D31f implemented it")
	}
	if !strings.Contains(src, "implemented (D31f)") {
		t.Error("OpenAPI spec should announce D31f as the implementation tranche")
	}
}

// TestOpenAPIContract_KnowledgeGraphStillReserved confirms the
// /v1/graphs/knowledge reservation is still ONLY a comment — no
// path, no schema. Boundary check against scope creep.
func TestOpenAPIContract_KnowledgeGraphStillReserved(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read v1.yaml: %v", err)
	}
	src := string(body)
	// Reservation note must still be present.
	if !strings.Contains(src, "/v1/graphs/knowledge") {
		t.Error("Knowledge Graph reservation comment missing from OpenAPI")
	}
	// No path entry under paths:.
	specPaths := loadSpecPaths(t)
	for _, p := range specPaths {
		if p == "/v1/graphs/knowledge" {
			t.Error("/v1/graphs/knowledge must NOT be a declared path (Knowledge Graph deferred)")
		}
	}
	// No schemas named KnowledgeGraph*.
	schemas := loadSpecComponentSchemas(t)
	for name := range schemas {
		if strings.HasPrefix(name, "KnowledgeGraph") {
			t.Errorf("Knowledge Graph schema %q must not exist (deferred)", name)
		}
	}
}

// TestOpenAPIContract_ContextGraphStillExists confirms D31f did not
// regress D31d's Context Graph endpoint or schema family.
func TestOpenAPIContract_ContextGraphStillExists(t *testing.T) {
	specPaths := loadSpecPaths(t)
	var found bool
	for _, p := range specPaths {
		if p == "/v1/graphs/context" {
			found = true
			break
		}
	}
	if !found {
		t.Error("D31d Context Graph path /v1/graphs/context must still be declared")
	}
	schemas := loadSpecComponentSchemas(t)
	if _, ok := schemas["ContextGraphProjection"]; !ok {
		t.Error("D31d ContextGraphProjection schema must still be declared")
	}
}

// TestOpenAPIContract_NoLegacyGraphPaths confirms D31d's removals
// of /v1/authority-graph and /v1/businessservices/{id}/governance-map
// have NOT been reverted.
func TestOpenAPIContract_NoLegacyGraphPaths(t *testing.T) {
	specPaths := loadSpecPaths(t)
	for _, p := range specPaths {
		if p == "/v1/authority-graph" {
			t.Error("legacy path /v1/authority-graph must remain absent (removed in D31d)")
		}
		if p == "/v1/businessservices/{id}/governance-map" {
			t.Error("legacy path /v1/businessservices/{id}/governance-map must remain absent (removed in D31d)")
		}
	}
}

// ---------------------------------------------------------------------------
// D31g — Summary, Diagnostics, validity, effective-policy
// ---------------------------------------------------------------------------

// TestOpenAPIContract_AuthorityGraphD31gSchemasDeclared confirms the
// D31g additions to the AuthorityGraph schema family are all present.
func TestOpenAPIContract_AuthorityGraphD31gSchemasDeclared(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	want := []string{
		"AuthorityGraphSummary",
		"AuthorityGraphDiagnostic",
		"AuthorityGraphDiagnosticSeverity",
		"AuthorityGraphDiagnosticKind",
		"AuthorityGraphValidityStatus",
		"AuthorityGraphEffectivePolicySource",
	}
	for _, name := range want {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphProjection_ReferencesSummaryAndDiagnostics
// pins that AuthorityGraphProjection adds optional summary and
// diagnostics fields via $ref.
func TestOpenAPIContract_AuthorityGraphProjection_ReferencesSummaryAndDiagnostics(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	root, ok := schemas["AuthorityGraphProjection"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection schema not a map")
	}
	props, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection has no properties map")
	}
	summary, ok := props["summary"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.summary missing")
	}
	if got := summary["$ref"]; got != "#/components/schemas/AuthorityGraphSummary" {
		t.Errorf("summary.$ref: want AuthorityGraphSummary, got %v", got)
	}
	diagnostics, ok := props["diagnostics"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.diagnostics missing")
	}
	items, ok := diagnostics["items"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.diagnostics.items missing")
	}
	if got := items["$ref"]; got != "#/components/schemas/AuthorityGraphDiagnostic" {
		t.Errorf("diagnostics.items.$ref: want AuthorityGraphDiagnostic, got %v", got)
	}
	// summary + diagnostics must NOT appear in required — both are
	// omitempty on the Go side and operators must not assume their
	// presence on the wire.
	if req, ok := root["required"].([]any); ok {
		for _, r := range req {
			if r == "summary" {
				t.Error("AuthorityGraphProjection.required must NOT include summary")
			}
			if r == "diagnostics" {
				t.Error("AuthorityGraphProjection.required must NOT include diagnostics")
			}
		}
	}
}

// TestOpenAPIContract_AuthorityGraphDiagnosticSeverity_Enum pins the
// exact three severity values.
func TestOpenAPIContract_AuthorityGraphDiagnosticSeverity_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	sev, ok := schemas["AuthorityGraphDiagnosticSeverity"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphDiagnosticSeverity schema not a map")
	}
	enum, ok := sev["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphDiagnosticSeverity.enum missing")
	}
	want := map[string]bool{"info": true, "warning": true, "critical": true}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	if len(enum) != 3 {
		t.Errorf("severity enum must have exactly 3 entries; got %d (%v)", len(enum), enum)
	}
	for w := range want {
		if !got[w] {
			t.Errorf("severity enum missing %q", w)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphValidityStatus_Enum pins the
// exact three validity-status values.
func TestOpenAPIContract_AuthorityGraphValidityStatus_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	vs, ok := schemas["AuthorityGraphValidityStatus"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphValidityStatus schema not a map")
	}
	enum, ok := vs["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphValidityStatus.enum missing")
	}
	want := map[string]bool{"effective": true, "future_dated": true, "expired": true}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	if len(enum) != 3 {
		t.Errorf("validity-status enum must have exactly 3 entries; got %d (%v)", len(enum), enum)
	}
	for w := range want {
		if !got[w] {
			t.Errorf("validity-status enum missing %q", w)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphEffectivePolicySource_Enum pins
// the exact three effective-policy-source values.
func TestOpenAPIContract_AuthorityGraphEffectivePolicySource_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	src, ok := schemas["AuthorityGraphEffectivePolicySource"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphEffectivePolicySource schema not a map")
	}
	enum, ok := src["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphEffectivePolicySource.enum missing")
	}
	want := map[string]bool{"override": true, "business_service_default": true, "none": true}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	if len(enum) != 3 {
		t.Errorf("effective-policy-source enum must have exactly 3 entries; got %d (%v)", len(enum), enum)
	}
	for w := range want {
		if !got[w] {
			t.Errorf("effective-policy-source enum missing %q", w)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphDiagnosticKind_Enum pins the
// complete set of 14 diagnostic kinds.
func TestOpenAPIContract_AuthorityGraphDiagnosticKind_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	dk, ok := schemas["AuthorityGraphDiagnosticKind"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphDiagnosticKind schema not a map")
	}
	enum, ok := dk["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphDiagnosticKind.enum missing")
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
		"profile_has_no_escalation_target",       // D31l
		"escalation_target_reference_dangling", // D31l
	}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("diagnostic-kind enum missing %q", w)
		}
	}
	if len(enum) != len(want) {
		t.Errorf("diagnostic-kind enum size: want %d, got %d (%v)", len(want), len(enum), enum)
	}
}

// TestOpenAPIContract_AuthorityGraphDecisionSurfaceData_EffectivePolicyFields
// pins the four new D31g fields on the surface schema.
func TestOpenAPIContract_AuthorityGraphDecisionSurfaceData_EffectivePolicyFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	ds, ok := schemas["AuthorityGraphDecisionSurfaceData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphDecisionSurfaceData schema not a map")
	}
	props, ok := ds["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphDecisionSurfaceData has no properties map")
	}
	for _, name := range []string{
		"effective_policy_source",
		"effective_policy_id",
		"effective_policy_version",
		"inherits_bs_policy",
	} {
		if _, ok := props[name].(map[string]any); !ok {
			t.Errorf("AuthorityGraphDecisionSurfaceData.properties.%s missing", name)
		}
	}
	// effective_policy_source must $ref the enum schema.
	src, _ := props["effective_policy_source"].(map[string]any)
	if got := src["$ref"]; got != "#/components/schemas/AuthorityGraphEffectivePolicySource" {
		t.Errorf("effective_policy_source.$ref: want AuthorityGraphEffectivePolicySource, got %v", got)
	}
}

// TestOpenAPIContract_AuthorityGraphAuthorityProfileData_ValidityFields
// pins effective_until + validity_status on profile data.
func TestOpenAPIContract_AuthorityGraphAuthorityProfileData_ValidityFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	d, ok := schemas["AuthorityGraphAuthorityProfileData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityProfileData schema not a map")
	}
	props, ok := d["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityProfileData has no properties map")
	}
	if _, ok := props["effective_until"].(map[string]any); !ok {
		t.Error("AuthorityGraphAuthorityProfileData.properties.effective_until missing")
	}
	vs, ok := props["validity_status"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityProfileData.properties.validity_status missing")
	}
	if got := vs["$ref"]; got != "#/components/schemas/AuthorityGraphValidityStatus" {
		t.Errorf("validity_status.$ref: want AuthorityGraphValidityStatus, got %v", got)
	}
}

// TestOpenAPIContract_AuthorityGraphAuthorityGrantData_ValidityField
// pins validity_status on grant data.
func TestOpenAPIContract_AuthorityGraphAuthorityGrantData_ValidityField(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	d, ok := schemas["AuthorityGraphAuthorityGrantData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData schema not a map")
	}
	props, ok := d["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData has no properties map")
	}
	vs, ok := props["validity_status"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData.properties.validity_status missing")
	}
	if got := vs["$ref"]; got != "#/components/schemas/AuthorityGraphValidityStatus" {
		t.Errorf("validity_status.$ref: want AuthorityGraphValidityStatus, got %v", got)
	}
}

// TestOpenAPIContract_AuthorityGraphFailModePolicyData_EffectiveUntilField
// pins effective_until on policy data.
func TestOpenAPIContract_AuthorityGraphFailModePolicyData_EffectiveUntilField(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	d, ok := schemas["AuthorityGraphFailModePolicyData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphFailModePolicyData schema not a map")
	}
	props, ok := d["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphFailModePolicyData has no properties map")
	}
	if _, ok := props["effective_until"].(map[string]any); !ok {
		t.Error("AuthorityGraphFailModePolicyData.properties.effective_until missing")
	}
}

// TestOpenAPIContract_AuthorityGraphNodeKindEnum_ExactCount pins the
// Authority Graph node-kind enum size at exactly seven. D31g did not
// add new node kinds (the inherited-policy concept lives on
// DecisionSurfaceData); D31l intentionally added escalation_target
// as the seventh. EscalationPolicy / EscalationRule remain deferred.
func TestOpenAPIContract_AuthorityGraphNodeKindEnum_ExactCount(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	node, ok := schemas["AuthorityGraphNode"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode schema not a map")
	}
	props, _ := node["properties"].(map[string]any)
	kindProp, _ := props["kind"].(map[string]any)
	enum, ok := kindProp["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphNode.properties.kind.enum missing")
	}
	if len(enum) != 7 {
		t.Errorf("AuthorityGraphNode.kind.enum: want exactly 7 entries (D31l added escalation_target); got %d (%v)", len(enum), enum)
	}
}

// TestOpenAPIContract_AuthorityGraphEdgeKindEnum_ExactCount pins the
// edge-kind enum size at exactly seven. D31l added
// profile_escalates_to as the seventh.
func TestOpenAPIContract_AuthorityGraphEdgeKindEnum_ExactCount(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	edge, ok := schemas["AuthorityGraphEdge"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphEdge schema not a map")
	}
	props, _ := edge["properties"].(map[string]any)
	kindProp, _ := props["kind"].(map[string]any)
	enum, ok := kindProp["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityGraphEdge.properties.kind.enum missing")
	}
	if len(enum) != 7 {
		t.Errorf("AuthorityGraphEdge.kind.enum: want exactly 7 entries (D31l added profile_escalates_to); got %d (%v)", len(enum), enum)
	}
}

// ---------------------------------------------------------------------------
// D31i — Capability + Constraint additions
// ---------------------------------------------------------------------------

// TestOpenAPIContract_AuthorityGraphD31iSchemasDeclared pins that the
// new D31i schemas exist as component schemas. Missing entries would
// produce broken $refs at runtime.
func TestOpenAPIContract_AuthorityGraphD31iSchemasDeclared(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	for _, name := range []string{
		"AuthorityCapability",
		"AuthorityConstraint",
		"AuthorityConstraintKind",
	} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

// TestOpenAPIContract_AuthorityCapability_Enum pins the canonical
// five capability values. Adding a new value here requires a domain
// change (internal/authority/capability.go) and an orchestrator
// branch — this test catches drift in either direction.
func TestOpenAPIContract_AuthorityCapability_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	cap, ok := schemas["AuthorityCapability"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityCapability schema not a map")
	}
	enum, ok := cap["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityCapability.enum missing")
	}
	want := map[string]bool{
		"recommend": true,
		"approve":   true,
		"reject":    true,
		"escalate":  true,
		"stop":      true,
	}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	for w := range want {
		if !got[w] {
			t.Errorf("AuthorityCapability enum missing %q", w)
		}
	}
	if len(enum) != len(want) {
		t.Errorf("AuthorityCapability enum size: want %d, got %d (%v)", len(want), len(enum), enum)
	}
}

// TestOpenAPIContract_AuthorityConstraintKind_Enum pins the
// canonical five constraint-kind values.
func TestOpenAPIContract_AuthorityConstraintKind_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	k, ok := schemas["AuthorityConstraintKind"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityConstraintKind schema not a map")
	}
	enum, ok := k["enum"].([]any)
	if !ok {
		t.Fatal("AuthorityConstraintKind.enum missing")
	}
	want := map[string]bool{
		"confidence_threshold_min":  true,
		"consequence_threshold_max": true,
		"human_only":                true,
		"ai_only":                   true,
		"time_window":               true,
	}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	for w := range want {
		if !got[w] {
			t.Errorf("AuthorityConstraintKind enum missing %q", w)
		}
	}
	if len(enum) != len(want) {
		t.Errorf("AuthorityConstraintKind enum size: want %d, got %d (%v)", len(want), len(enum), enum)
	}
}

// TestOpenAPIContract_AuthorityConstraint_Shape pins the Constraint
// schema carries kind (required, $ref to AuthorityConstraintKind)
// plus the per-kind payload fields.
func TestOpenAPIContract_AuthorityConstraint_Shape(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	c, ok := schemas["AuthorityConstraint"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityConstraint schema not a map")
	}
	if req, ok := c["required"].([]any); ok {
		found := false
		for _, r := range req {
			if r == "kind" {
				found = true
			}
		}
		if !found {
			t.Error("AuthorityConstraint.required must include kind")
		}
	}
	props, ok := c["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityConstraint has no properties")
	}
	for _, name := range []string{
		"kind", "min_confidence", "max_consequence", "start_time", "end_time",
	} {
		if _, ok := props[name].(map[string]any); !ok {
			t.Errorf("AuthorityConstraint.properties.%s missing", name)
		}
	}
	kind, _ := props["kind"].(map[string]any)
	if got := kind["$ref"]; got != "#/components/schemas/AuthorityConstraintKind" {
		t.Errorf("kind.$ref: want AuthorityConstraintKind, got %v", got)
	}
}

// TestOpenAPIContract_AuthorityGraphAuthorityGrantData_CapabilityFields
// pins the new D31i fields on the grant data schema.
func TestOpenAPIContract_AuthorityGraphAuthorityGrantData_CapabilityFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	d, ok := schemas["AuthorityGraphAuthorityGrantData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData schema not a map")
	}
	props, ok := d["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData has no properties")
	}
	caps, ok := props["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData.properties.capabilities missing")
	}
	if got := caps["type"]; got != "array" {
		t.Errorf("capabilities.type: want array, got %v", got)
	}
	capsItems, _ := caps["items"].(map[string]any)
	if got := capsItems["$ref"]; got != "#/components/schemas/AuthorityCapability" {
		t.Errorf("capabilities.items.$ref: want AuthorityCapability, got %v", got)
	}
	cons, ok := props["constraints"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityGrantData.properties.constraints missing")
	}
	if got := cons["type"]; got != "array" {
		t.Errorf("constraints.type: want array, got %v", got)
	}
	consItems, _ := cons["items"].(map[string]any)
	if got := consItems["$ref"]; got != "#/components/schemas/AuthorityConstraint" {
		t.Errorf("constraints.items.$ref: want AuthorityConstraint, got %v", got)
	}
}

// TestOpenAPIContract_AuthorityGraphSummary_D31iFields pins the
// stop-capability + constraint counts on the Summary schema.
func TestOpenAPIContract_AuthorityGraphSummary_D31iFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	s, ok := schemas["AuthorityGraphSummary"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphSummary schema not a map")
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphSummary has no properties")
	}
	for _, name := range []string{
		"grants_with_stop_capability",
		"grants_with_constraints",
		"grants_without_capabilities",
	} {
		if _, ok := props[name].(map[string]any); !ok {
			t.Errorf("AuthorityGraphSummary.properties.%s missing", name)
		}
	}
	// stop/constraints counts must be in required.
	req, _ := s["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range req {
		if rs, ok := r.(string); ok {
			reqSet[rs] = true
		}
	}
	for _, n := range []string{"grants_with_stop_capability", "grants_with_constraints"} {
		if !reqSet[n] {
			t.Errorf("AuthorityGraphSummary.required must include %q", n)
		}
	}
}

// ---------------------------------------------------------------------------
// D31l — Escalation Target projection additions
// ---------------------------------------------------------------------------

// TestOpenAPIContract_AuthorityGraphEscalationTargetData_Schema pins
// the new typed-data schema and its required fields.
func TestOpenAPIContract_AuthorityGraphEscalationTargetData_Schema(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	d, ok := schemas["AuthorityGraphEscalationTargetData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphEscalationTargetData schema not present")
	}
	req, _ := d["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range req {
		if rs, ok := r.(string); ok {
			reqSet[rs] = true
		}
	}
	for _, want := range []string{"id", "version", "name", "kind", "handle", "status"} {
		if !reqSet[want] {
			t.Errorf("AuthorityGraphEscalationTargetData.required must include %q", want)
		}
	}
	props, _ := d["properties"].(map[string]any)
	for _, name := range []string{
		"id", "version", "name", "description",
		"kind", "handle", "status",
		"effective_date", "effective_until",
		"business_owner", "technical_owner",
		"approved_by", "approved_at",
	} {
		if _, ok := props[name].(map[string]any); !ok {
			t.Errorf("AuthorityGraphEscalationTargetData.properties.%s missing", name)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphAuthorityProfileData_EscalationTargetID
// pins the D31l additive profile property.
func TestOpenAPIContract_AuthorityGraphAuthorityProfileData_EscalationTargetID(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	d, ok := schemas["AuthorityGraphAuthorityProfileData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthorityProfileData schema not present")
	}
	props, _ := d["properties"].(map[string]any)
	if _, ok := props["escalation_target_id"].(map[string]any); !ok {
		t.Errorf("AuthorityGraphAuthorityProfileData.properties.escalation_target_id missing")
	}
}

// TestOpenAPIContract_AuthorityGraphSummary_D31lFields pins the four
// D31l escalation-target rollups on the summary schema and asserts
// the two scalar counts are required.
func TestOpenAPIContract_AuthorityGraphSummary_D31lFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	s, ok := schemas["AuthorityGraphSummary"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphSummary schema not present")
	}
	props, _ := s["properties"].(map[string]any)
	for _, name := range []string{
		"escalation_target_count",
		"profiles_with_escalation_target",
		"profiles_without_escalation_target",
		"profiles_with_dangling_escalation_target",
	} {
		if _, ok := props[name].(map[string]any); !ok {
			t.Errorf("AuthorityGraphSummary.properties.%s missing", name)
		}
	}
	req, _ := s["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range req {
		if rs, ok := r.(string); ok {
			reqSet[rs] = true
		}
	}
	for _, n := range []string{"escalation_target_count", "profiles_with_escalation_target"} {
		if !reqSet[n] {
			t.Errorf("AuthorityGraphSummary.required must include %q", n)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphDiagnosticKind_D31lAdditions
// pins the two new diagnostic kinds.
func TestOpenAPIContract_AuthorityGraphDiagnosticKind_D31lAdditions(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	dk, ok := schemas["AuthorityGraphDiagnosticKind"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphDiagnosticKind schema not present")
	}
	enum, _ := dk["enum"].([]any)
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	for _, want := range []string{
		"profile_has_no_escalation_target",
		"escalation_target_reference_dangling",
	} {
		if !got[want] {
			t.Errorf("AuthorityGraphDiagnosticKind enum missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// D31m — DiagnosticSummary + SurfaceAuthorityPosture + examples
// ---------------------------------------------------------------------------

// TestOpenAPIContract_D31mSchemasDeclared pins the new schemas exist.
func TestOpenAPIContract_D31mSchemasDeclared(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	for _, name := range []string{
		"AuthorityGraphDiagnosticSummary",
		"AuthorityGraphSurfaceAuthorityPosture",
		"AuthorityGraphSeverityRollupValue",
		"AuthorityGraphAuthorityStatus",
		"AuthorityGraphProfileStatus",
		"AuthorityGraphGrantStatus",
		"AuthorityGraphAgentStatus",
		"AuthorityGraphFailModePolicyStatus",
		"AuthorityGraphEscalationStatus",
	} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphProjection_ReferencesD31mFields
// pins the projection schema references the D31m additions.
func TestOpenAPIContract_AuthorityGraphProjection_ReferencesD31mFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	proj, ok := schemas["AuthorityGraphProjection"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection schema not present")
	}
	props, _ := proj["properties"].(map[string]any)
	ds, ok := props["diagnostic_summary"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.diagnostic_summary missing")
	}
	if got := ds["$ref"]; got != "#/components/schemas/AuthorityGraphDiagnosticSummary" {
		t.Errorf("diagnostic_summary.$ref: want AuthorityGraphDiagnosticSummary; got %v", got)
	}
	sp, ok := props["surface_posture"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.surface_posture missing")
	}
	if got := sp["type"]; got != "array" {
		t.Errorf("surface_posture.type: want array, got %v", got)
	}
	items, _ := sp["items"].(map[string]any)
	if got := items["$ref"]; got != "#/components/schemas/AuthorityGraphSurfaceAuthorityPosture" {
		t.Errorf("surface_posture.items.$ref: want AuthorityGraphSurfaceAuthorityPosture; got %v", got)
	}
}

func enumValuesOf(t *testing.T, schemas map[string]any, name string) []string {
	t.Helper()
	s, ok := schemas[name].(map[string]any)
	if !ok {
		t.Fatalf("%s schema not present", name)
	}
	enum, ok := s["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum missing", name)
	}
	out := make([]string, 0, len(enum))
	for _, v := range enum {
		if vs, ok := v.(string); ok {
			out = append(out, vs)
		}
	}
	return out
}

func assertEnumSetExact(t *testing.T, name string, got, want []string) {
	t.Helper()
	gotSet := map[string]bool{}
	for _, v := range got {
		gotSet[v] = true
	}
	wantSet := map[string]bool{}
	for _, v := range want {
		wantSet[v] = true
	}
	if len(gotSet) != len(wantSet) {
		t.Errorf("%s enum size: want %d, got %d (%v)", name, len(want), len(got), got)
	}
	for v := range wantSet {
		if !gotSet[v] {
			t.Errorf("%s enum missing %q", name, v)
		}
	}
	for v := range gotSet {
		if !wantSet[v] {
			t.Errorf("%s enum has unexpected %q", name, v)
		}
	}
}

func TestOpenAPIContract_AuthorityStatusEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphAuthorityStatus",
		enumValuesOf(t, schemas, "AuthorityGraphAuthorityStatus"),
		[]string{"complete", "incomplete", "degraded", "uncovered"})
}

func TestOpenAPIContract_ProfileStatusEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphProfileStatus",
		enumValuesOf(t, schemas, "AuthorityGraphProfileStatus"),
		[]string{"covered", "missing"})
}

func TestOpenAPIContract_GrantStatusEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphGrantStatus",
		enumValuesOf(t, schemas, "AuthorityGraphGrantStatus"),
		[]string{"covered", "missing"})
}

func TestOpenAPIContract_AgentStatusEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphAgentStatus",
		enumValuesOf(t, schemas, "AuthorityGraphAgentStatus"),
		[]string{"covered", "missing", "blocked"})
}

func TestOpenAPIContract_FailModePolicyStatusEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphFailModePolicyStatus",
		enumValuesOf(t, schemas, "AuthorityGraphFailModePolicyStatus"),
		[]string{"override", "inherited", "missing", "dangling"})
}

func TestOpenAPIContract_EscalationStatusEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphEscalationStatus",
		enumValuesOf(t, schemas, "AuthorityGraphEscalationStatus"),
		[]string{"targeted", "not_targeted", "dangling"})
}

func TestOpenAPIContract_SeverityRollupValueEnum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	assertEnumSetExact(t, "AuthorityGraphSeverityRollupValue",
		enumValuesOf(t, schemas, "AuthorityGraphSeverityRollupValue"),
		[]string{"none", "info", "warning", "critical"})
}

// TestOpenAPIContract_AuthorityGraphDescription_NodeKindsCountSeven
// pins the corrected "(7)" wording. D31i + D31l intentionally bumped
// node and edge kinds to 7 each.
func TestOpenAPIContract_AuthorityGraphDescription_NodeKindsCountSeven(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read v1.yaml: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "Node kinds (7)") {
		t.Error("OpenAPI must declare 'Node kinds (7)' for /v1/graphs/authority")
	}
	if !strings.Contains(s, "Edge kinds (7") {
		t.Error("OpenAPI must declare 'Edge kinds (7' for /v1/graphs/authority")
	}
	if strings.Contains(s, "Node kinds (6)") {
		t.Error("OpenAPI must NOT still say 'Node kinds (6)'")
	}
	if strings.Contains(s, "Edge kinds (6") {
		t.Error("OpenAPI must NOT still say 'Edge kinds (6'")
	}
}

// TestOpenAPIContract_AuthorityGraphExamples_Declared pins the three
// required example names and a minimum set of top-level keys per
// example. Examples are parsed as YAML strings — the test does not
// validate against the schema (the Go schema-validator is not wired
// in this package) but it verifies the contract names exist.
func TestOpenAPIContract_AuthorityGraphExamples_Declared(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read v1.yaml: %v", err)
	}
	s := string(body)
	for _, name := range []string{
		"healthy_authority_graph",
		"mixed_diagnostics_authority_graph",
		"dangling_escalation_target_authority_graph",
	} {
		if !strings.Contains(s, name+":") {
			t.Errorf("OpenAPI examples must include %q", name)
		}
	}
	// Each example payload must reference every top-level
	// Projection field. Use unique substrings that only appear
	// inside the value: blocks (and not in description prose).
	for _, want := range []string{
		"\n                    root:",
		"\n                    view: service",
		"\n                    depth: 4",
		"\n                    nodes:",
		"\n                    edges:",
		"\n                    summary:",
		"\n                    diagnostics:",
		"\n                    diagnostic_summary:",
		"\n                    surface_posture:",
	} {
		// Each substring must appear at least once for each of the
		// three examples (3 total occurrences in the file).
		if c := strings.Count(s, want); c < 3 {
			t.Errorf("OpenAPI examples missing key %q: want >= 3 occurrences, got %d", want, c)
		}
	}
}
