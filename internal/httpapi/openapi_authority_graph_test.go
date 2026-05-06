package httpapi

// openapi_authority_graph_test.go — contract assertions for the
// authority-graph schemas added to api/openapi/v1.yaml in Phase 1.
//
// Mirrors openapi_governance_map_test.go's coverage shape: path
// declared, all referenced component schemas defined, top-level
// projection schema references node/edge schemas via $ref, node/edge
// kind enums constrained to the Phase 1 allowed sets.

import (
	"testing"
)

// TestOpenAPIContract_AuthorityGraphPathDeclared asserts the new
// operation is declared at /v1/authority-graph.
func TestOpenAPIContract_AuthorityGraphPathDeclared(t *testing.T) {
	specPaths := loadSpecPaths(t)
	const want = "/v1/authority-graph"
	for _, p := range specPaths {
		if p == want {
			return
		}
	}
	t.Errorf("OpenAPI spec missing path %q", want)
}

// TestOpenAPIContract_AuthorityGraphSchemasDefined asserts every
// component schema the projection references is declared. A renamed
// or missing schema fails this test rather than producing a broken
// $ref at runtime. Phase 2A added per-kind typed-data schemas; the
// list below is the union of Phase 1 and Phase 2A schema names.
func TestOpenAPIContract_AuthorityGraphSchemasDefined(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	want := []string{
		// Phase 1 core.
		"AuthorityGraphProjection",
		"AuthorityGraphNode",
		"AuthorityGraphEdge",
		"AuthorityGraphNodeRef",
		// Phase 2A typed-data schemas (one per node kind, plus shared
		// external_ref).
		"AuthorityGraphExternalRefData",
		"AuthorityGraphBusinessServiceData",
		"AuthorityGraphRelatedBusinessServiceData",
		// Phase 2B Step 6: per-direction sub-row schema for related
		// business services. Used by both outgoing and incoming
		// pointers on AuthorityGraphRelatedBusinessServiceData.
		"AuthorityGraphRelatedBusinessServiceRow",
		"AuthorityGraphCapabilityData",
		"AuthorityGraphProcessData",
		"AuthorityGraphDecisionSurfaceData",
		"AuthorityGraphAISystemData",
		"AuthorityGraphAISystemBindingData",
		"AuthorityGraphAuthoritySummaryData",
		"AuthorityGraphCoverageData",
	}
	for _, name := range want {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphNode_TypedDataSlots asserts that
// AuthorityGraphNode declares one optional typed-data property per
// allowed kind, each referencing its dedicated schema. The properties
// are NOT marked required (exactly one is populated per node, matched
// to kind).
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
		"business_service":         "#/components/schemas/AuthorityGraphBusinessServiceData",
		"related_business_service": "#/components/schemas/AuthorityGraphRelatedBusinessServiceData",
		"capability":               "#/components/schemas/AuthorityGraphCapabilityData",
		"process":                  "#/components/schemas/AuthorityGraphProcessData",
		"decision_surface":         "#/components/schemas/AuthorityGraphDecisionSurfaceData",
		"ai_system":                "#/components/schemas/AuthorityGraphAISystemData",
		"ai_system_binding":        "#/components/schemas/AuthorityGraphAISystemBindingData",
		"authority_summary":        "#/components/schemas/AuthorityGraphAuthoritySummaryData",
		"coverage":                 "#/components/schemas/AuthorityGraphCoverageData",
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

	// The typed-data slots must NOT appear in `required`; exactly one is
	// populated per node, gated by `kind`.
	if req, ok := node["required"].([]any); ok {
		for _, slot := range []string{
			"business_service", "related_business_service",
			"capability", "process", "decision_surface",
			"ai_system", "ai_system_binding",
			"authority_summary", "coverage",
		} {
			for _, r := range req {
				if r == slot {
					t.Errorf("AuthorityGraphNode.required must NOT include typed-data slot %q (it is gated by kind)", slot)
				}
			}
		}
	}
}

// TestOpenAPIContract_AuthorityGraphCoverageData_FieldNames pins the
// four count fields the synthetic `coverage` node carries. These are
// the explicit Phase 9 names; the spec MUST NOT regress to the
// pre-Phase-9 ambiguous fields.
func TestOpenAPIContract_AuthorityGraphCoverageData_FieldNames(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	cov, ok := schemas["AuthorityGraphCoverageData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphCoverageData schema not a map")
	}
	props, ok := cov["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphCoverageData has no properties map")
	}
	for _, want := range []string{
		"surface_count",
		"surfaces_with_direct_ai_binding",
		"surfaces_with_scoped_ai_binding",
		"surfaces_with_no_ai_binding",
	} {
		if _, ok := props[want].(map[string]any); !ok {
			t.Errorf("AuthorityGraphCoverageData.properties.%s missing", want)
		}
	}
	// Forbidden Phase-9 ambiguous names.
	for _, illegal := range []string{
		"surfaces_with_ai_binding",
		"surfaces_without_ai_binding",
	} {
		if _, ok := props[illegal]; ok {
			t.Errorf("AuthorityGraphCoverageData.properties.%s must NOT exist (Phase 9 ambiguous name)", illegal)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphAuthoritySummaryData_FieldNames
// pins the four count fields the synthetic `authority_summary` node
// carries.
func TestOpenAPIContract_AuthorityGraphAuthoritySummaryData_FieldNames(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	auth, ok := schemas["AuthorityGraphAuthoritySummaryData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthoritySummaryData schema not a map")
	}
	props, ok := auth["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphAuthoritySummaryData has no properties map")
	}
	for _, want := range []string{
		"surface_count",
		"active_profile_count",
		"active_grant_count",
		"active_agent_count",
	} {
		if _, ok := props[want].(map[string]any); !ok {
			t.Errorf("AuthorityGraphAuthoritySummaryData.properties.%s missing", want)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphRelatedBusinessService_NestedRows
// pins the Phase 2B Step 6 shape: outgoing and incoming sub-rows on
// AuthorityGraphRelatedBusinessServiceData via $ref to
// AuthorityGraphRelatedBusinessServiceRow. The old flat fields
// (`relationship_type`, `direction`) MUST NOT be present at the
// data-level — they belong on the row schema.
func TestOpenAPIContract_AuthorityGraphRelatedBusinessService_NestedRows(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)

	data, ok := schemas["AuthorityGraphRelatedBusinessServiceData"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphRelatedBusinessServiceData schema not a map")
	}
	props, ok := data["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphRelatedBusinessServiceData has no properties map")
	}
	for _, want := range []string{"id", "name", "outgoing", "incoming"} {
		if _, ok := props[want].(map[string]any); !ok {
			t.Errorf("AuthorityGraphRelatedBusinessServiceData.properties.%s missing", want)
		}
	}
	// Forbidden Phase-2A flat fields — must NOT appear on the data-
	// level schema; they live on the row schema.
	for _, illegal := range []string{"relationship_type", "direction", "relationship_id", "description"} {
		if _, ok := props[illegal]; ok {
			t.Errorf("AuthorityGraphRelatedBusinessServiceData.properties.%s must NOT exist (moved to RelatedBusinessServiceRow)", illegal)
		}
	}
	// Verify outgoing/incoming reference the row schema.
	for _, slot := range []string{"outgoing", "incoming"} {
		s, _ := props[slot].(map[string]any)
		if ref, _ := s["$ref"].(string); ref != "#/components/schemas/AuthorityGraphRelatedBusinessServiceRow" {
			t.Errorf("AuthorityGraphRelatedBusinessServiceData.properties.%s.$ref = %q, want AuthorityGraphRelatedBusinessServiceRow", slot, ref)
		}
	}

	row, ok := schemas["AuthorityGraphRelatedBusinessServiceRow"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphRelatedBusinessServiceRow schema not a map")
	}
	rowProps, ok := row["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphRelatedBusinessServiceRow has no properties map")
	}
	for _, want := range []string{"relationship_id", "relationship_type", "description"} {
		if _, ok := rowProps[want].(map[string]any); !ok {
			t.Errorf("AuthorityGraphRelatedBusinessServiceRow.properties.%s missing", want)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphPhase2BFields pins the five
// fields added in Phase 2B for Explorer parity. A regression that
// removed any of them would break the Explorer migration's
// USE_AUTHORITY_GRAPH=true path.
func TestOpenAPIContract_AuthorityGraphPhase2BFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)

	mustHaveProp := func(schemaName, propName string) {
		t.Helper()
		s, ok := schemas[schemaName].(map[string]any)
		if !ok {
			t.Fatalf("%s schema not a map", schemaName)
		}
		props, ok := s["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no properties map", schemaName)
		}
		if _, ok := props[propName].(map[string]any); !ok {
			t.Errorf("%s.properties.%s missing", schemaName, propName)
		}
	}

	mustNotBeRequired := func(schemaName, propName string) {
		t.Helper()
		s, ok := schemas[schemaName].(map[string]any)
		if !ok {
			return
		}
		req, _ := s["required"].([]any)
		for _, r := range req {
			if rs, ok := r.(string); ok && rs == propName {
				t.Errorf("%s.required must NOT include %s — Phase 2B fields are optional", schemaName, propName)
			}
		}
	}

	// BusinessServiceData: service_type, regulatory_scope (both optional).
	mustHaveProp("AuthorityGraphBusinessServiceData", "service_type")
	mustHaveProp("AuthorityGraphBusinessServiceData", "regulatory_scope")
	mustNotBeRequired("AuthorityGraphBusinessServiceData", "service_type")
	mustNotBeRequired("AuthorityGraphBusinessServiceData", "regulatory_scope")

	// AISystemBindingData: role, description (both optional).
	mustHaveProp("AuthorityGraphAISystemBindingData", "role")
	mustHaveProp("AuthorityGraphAISystemBindingData", "description")
	mustNotBeRequired("AuthorityGraphAISystemBindingData", "role")
	mustNotBeRequired("AuthorityGraphAISystemBindingData", "description")

	// AISystemData: active_version_status (optional).
	mustHaveProp("AuthorityGraphAISystemData", "active_version_status")
	mustNotBeRequired("AuthorityGraphAISystemData", "active_version_status")
}

// TestOpenAPIContract_AuthorityGraphNode_NoAttrsField pins that the
// node schema has NO `attrs` field. Phase 2A's contract is "typed
// per-kind data, never a generic attrs map". A regression that added
// `attrs: { type: object, additionalProperties: true }` would surface
// here loudly.
func TestOpenAPIContract_AuthorityGraphNode_NoAttrsField(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	node, ok := schemas["AuthorityGraphNode"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode schema not a map")
	}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphNode has no properties map")
	}
	if _, ok := props["attrs"]; ok {
		t.Error("AuthorityGraphNode.properties.attrs must NOT exist — Phase 2A forbids generic attrs maps")
	}
	// Also verify no `additionalProperties: true` slipped in.
	if ap, ok := node["additionalProperties"]; ok {
		if b, isBool := ap.(bool); isBool && b {
			t.Error("AuthorityGraphNode.additionalProperties must not be `true` — typed data is closed by kind")
		}
	}
}

// TestOpenAPIContract_AuthorityGraphProjection_ReferencesNodeAndEdge
// pins that the top-level projection schema references the node and
// edge schemas via $ref through the items shape — a regression that
// inlined the schema would surface here.
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

	// nodes -> array -> items -> $ref
	checkArrayItemsRef(t, props, "nodes", "#/components/schemas/AuthorityGraphNode")
	checkArrayItemsRef(t, props, "edges", "#/components/schemas/AuthorityGraphEdge")

	// root -> object -> $ref
	rootProp, ok := props["root"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.root not a map")
	}
	if got := rootProp["$ref"]; got != "#/components/schemas/AuthorityGraphNodeRef" {
		t.Errorf("root.$ref: want AuthorityGraphNodeRef, got %v", got)
	}

	// view -> enum: [service, ai_system]
	viewProp, ok := props["view"].(map[string]any)
	if !ok {
		t.Fatal("AuthorityGraphProjection.properties.view not a map")
	}
	enum, ok := viewProp["enum"].([]any)
	if !ok {
		t.Fatalf("view.enum not a slice, got %v", viewProp["enum"])
	}
	gotEnum := map[string]bool{}
	for _, e := range enum {
		if s, ok := e.(string); ok {
			gotEnum[s] = true
		}
	}
	for _, want := range []string{"service", "ai_system"} {
		if !gotEnum[want] {
			t.Errorf("view.enum missing %q (got %v)", want, enum)
		}
	}
	if len(enum) != 2 {
		t.Errorf("view.enum: want exactly 2 entries (service, ai_system), got %d (%v)", len(enum), enum)
	}
}

// TestOpenAPIContract_AuthorityGraphNode_KindEnum pins the Phase 1
// node-kind allow-list. Adding a kind in Phase 2 must update both the
// Go constants and this enum; dropping a kind here without updating
// the projection would produce a wire-shape regression.
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
		"business_service":         true,
		"related_business_service": true,
		"capability":               true,
		"process":                  true,
		"decision_surface":         true,
		"ai_system":                true,
		"ai_system_binding":        true,
		"authority_summary":        true,
		"coverage":                 true,
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
	// Forbidden Phase 1 kinds.
	for _, illegal := range []string{"authority_profile", "authority_grant", "agent", "ai_system_version"} {
		if gotSet[illegal] {
			t.Errorf("node kind enum must NOT include forbidden Phase 1 kind %q", illegal)
		}
	}
}

// TestOpenAPIContract_AuthorityGraphEdge_KindEnum pins the Phase 1
// edge-kind allow-list. Same regression-guard role as the node test.
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
		"relates_to":       true,
		"has_capability":   true,
		"has_process":      true,
		"has_surface":      true,
		"bound_to":         true,
		"system_of":        true,
		"summarises":       true,
		"reports_coverage": true,
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
	for _, illegal := range []string{"governed_by", "has_grant", "granted_to", "has_active_version"} {
		if gotSet[illegal] {
			t.Errorf("edge kind enum must NOT include forbidden Phase 1 kind %q", illegal)
		}
	}

	// Edge.src and Edge.dst must reference NodeRef.
	for _, side := range []string{"src", "dst"} {
		sideProp, ok := props[side].(map[string]any)
		if !ok {
			t.Fatalf("AuthorityGraphEdge.properties.%s not a map", side)
		}
		if got := sideProp["$ref"]; got != "#/components/schemas/AuthorityGraphNodeRef" {
			t.Errorf("AuthorityGraphEdge.%s.$ref: want AuthorityGraphNodeRef, got %v", side, got)
		}
	}
}

// checkArrayItemsRef asserts that props[name] is { type: array, items: { $ref: ... } }.
func checkArrayItemsRef(t *testing.T, props map[string]any, name, wantRef string) {
	t.Helper()
	prop, ok := props[name].(map[string]any)
	if !ok {
		t.Fatalf("AuthorityGraphProjection.properties.%s not a map", name)
	}
	if got := prop["type"]; got != "array" {
		t.Errorf("AuthorityGraphProjection.%s.type: want array, got %v", name, got)
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		t.Fatalf("AuthorityGraphProjection.%s.items not a map", name)
	}
	if got := items["$ref"]; got != wantRef {
		t.Errorf("AuthorityGraphProjection.%s.items.$ref: want %s, got %v", name, wantRef, got)
	}
}
