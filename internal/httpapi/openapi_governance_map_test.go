package httpapi

// openapi_governance_map_test.go — contract assertions for the
// governance map schemas added to api/openapi/v1.yaml in Epic 1, PR 4.
//
// These tests pin the load-bearing properties of the wire model so a
// regression at the spec layer surfaces as a named test failure rather
// than client-visible drift. Coverage:
//
//   - Operation registered at the expected path (TestOpenAPIContract_PathSymmetry
//     handles this generically; this file's GovernanceMapPathDeclared test pins
//     the specific operationId for diagnostic clarity).
//   - All 12 component schemas defined.
//   - GovernanceMapResponse references each section schema.
//   - GovernanceMapResponse intentionally OMITS recent_decisions (Step 0.5
//     deferral marker — PR 8 will add it as a non-breaking addition).
//   - external_ref references on the BusinessService and AISystem nodes
//     follow the PR 3 nullable-with-allOf pattern.

import (
	"testing"
)

// TestOpenAPIContract_GovernanceMapPathDeclared asserts the new operation is
// declared in the spec (in addition to the path-symmetry test that proves
// every spec path has a registered handler).
func TestOpenAPIContract_GovernanceMapPathDeclared(t *testing.T) {
	specPaths := loadSpecPaths(t)
	want := "/v1/businessservices/{id}/governance-map"
	for _, p := range specPaths {
		if p == want {
			return
		}
	}
	t.Errorf("OpenAPI spec missing path %q", want)
}

// TestOpenAPIContract_GovernanceMapSchemasDefined asserts every component
// schema the governance map response references is declared. A renamed
// or missing schema fails this test rather than producing a broken $ref
// at runtime.
func TestOpenAPIContract_GovernanceMapSchemasDefined(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	want := []string{
		"GovernanceMapResponse",
		"GovernanceMapBusinessService",
		"GovernanceMapRelationships",
		"GovernanceMapRelationship",
		"GovernanceMapCapability",
		"GovernanceMapProcess",
		"GovernanceMapSurface",
		"GovernanceMapAISystem",
		"GovernanceMapAISystemVersion",
		"GovernanceMapAISystemBinding",
		"GovernanceMapAuthoritySummary",
		"GovernanceMapCoverage",
	}
	for _, name := range want {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

// TestOpenAPIContract_GovernanceMapResponseReferencesAllSections asserts the
// root response schema's properties reference each section schema by $ref.
// A future contributor adding a new section to the response shape must add
// the matching $ref here; one that drops a section breaks this test.
func TestOpenAPIContract_GovernanceMapResponseReferencesAllSections(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	root, ok := schemas["GovernanceMapResponse"].(map[string]any)
	if !ok {
		t.Fatal("GovernanceMapResponse schema not a map")
	}
	props, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatal("GovernanceMapResponse has no properties map")
	}

	// Each section must reference the corresponding schema. Sections
	// holding scalar arrays (capabilities, processes, surfaces,
	// ai_systems) reference via items.$ref; the four object-typed
	// sections reference directly via $ref.
	expected := map[string]string{
		"business_service":  "#/components/schemas/GovernanceMapBusinessService",
		"relationships":     "#/components/schemas/GovernanceMapRelationships",
		"capabilities":      "#/components/schemas/GovernanceMapCapability",
		"processes":         "#/components/schemas/GovernanceMapProcess",
		"surfaces":          "#/components/schemas/GovernanceMapSurface",
		"ai_systems":        "#/components/schemas/GovernanceMapAISystem",
		"authority_summary": "#/components/schemas/GovernanceMapAuthoritySummary",
		"coverage":          "#/components/schemas/GovernanceMapCoverage",
	}

	for propName, wantRef := range expected {
		t.Run(propName, func(t *testing.T) {
			prop, ok := props[propName]
			if !ok {
				t.Fatalf("GovernanceMapResponse.properties.%s missing", propName)
			}
			if !containsRef(prop, wantRef) {
				t.Errorf("GovernanceMapResponse.properties.%s does not reference %q",
					propName, wantRef)
			}
		})
	}
}

// TestOpenAPIContract_GovernanceMapResponse_DoesNotDeclareRecentDecisions is
// the load-bearing marker for the Step 0.5 deferral. The wire response
// omits the field entirely (no key, no null). The OpenAPI schema's
// `properties` map must NOT declare `recent_decisions`. When PR 8 adds
// the field as a non-breaking addition, that PR is the one that should
// extend this test (or remove it).
func TestOpenAPIContract_GovernanceMapResponse_DoesNotDeclareRecentDecisions(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	root, ok := schemas["GovernanceMapResponse"].(map[string]any)
	if !ok {
		t.Fatal("GovernanceMapResponse schema not a map")
	}
	props, _ := root["properties"].(map[string]any)
	if _, present := props["recent_decisions"]; present {
		t.Error("GovernanceMapResponse must not declare recent_decisions in PR 4 (Step 0.5 deferral); the field lands in PR 8")
	}
	required, _ := root["required"].([]any)
	for _, r := range required {
		if s, ok := r.(string); ok && s == "recent_decisions" {
			t.Error("GovernanceMapResponse.required must not list recent_decisions in PR 4")
		}
	}
}

// TestOpenAPIContract_GovernanceMapAISystem_ReferencesExternalRef asserts the
// AI system node uses the canonical PR 3 nullable-with-allOf pattern for
// external_ref. A regression that drops the $ref or removes nullability
// fails this test rather than producing client-visible drift.
func TestOpenAPIContract_GovernanceMapAISystem_ReferencesExternalRef(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	for _, schemaName := range []string{
		"GovernanceMapBusinessService",
		"GovernanceMapAISystem",
	} {
		t.Run(schemaName, func(t *testing.T) {
			schema, ok := schemas[schemaName].(map[string]any)
			if !ok {
				t.Fatalf("schema %q missing or not a map", schemaName)
			}
			props, _ := schema["properties"].(map[string]any)
			extProp, ok := props["external_ref"].(map[string]any)
			if !ok {
				t.Fatalf("%s.properties.external_ref missing", schemaName)
			}
			if !containsRef(extProp, "#/components/schemas/ExternalRef") {
				t.Errorf("%s.external_ref must $ref ExternalRef", schemaName)
			}
			if nullable, _ := extProp["nullable"].(bool); !nullable {
				t.Errorf("%s.external_ref must be nullable: true", schemaName)
			}
		})
	}
}

// TestOpenAPIContract_GovernanceMapAISystemBinding_NullablePointerFields
// asserts the four context-id fields and ai_system_version are declared
// nullable, matching the wire helper's *string / *int pointer rendering.
func TestOpenAPIContract_GovernanceMapAISystemBinding_NullablePointerFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	bind, ok := schemas["GovernanceMapAISystemBinding"].(map[string]any)
	if !ok {
		t.Fatal("GovernanceMapAISystemBinding schema not a map")
	}
	props, _ := bind["properties"].(map[string]any)
	for _, propName := range []string{
		"ai_system_version",
		"business_service_id",
		"capability_id",
		"process_id",
		"surface_id",
	} {
		t.Run(propName, func(t *testing.T) {
			prop, ok := props[propName].(map[string]any)
			if !ok {
				t.Fatalf("%s missing", propName)
			}
			if nullable, _ := prop["nullable"].(bool); !nullable {
				t.Errorf("%s must be nullable: true (matches wire pointer rendering)", propName)
			}
		})
	}
}
