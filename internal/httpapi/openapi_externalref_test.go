package httpapi

// openapi_externalref_test.go — asserts that the ExternalRef component
// schema is referenced from every entity response schema that the
// Cluster B mapper populates.
//
// Load-bearing posture: this test catches the regression where a new
// entity gains a domain ExternalRef field but the OpenAPI spec
// forgets to add the `external_ref` property on the corresponding
// response schema. Also catches the inverse — a typo on the schema
// reference (e.g. `ExtRef`) — by requiring the canonical
// `#/components/schemas/ExternalRef` path.

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// loadSpecComponentSchemas returns the components.schemas map from the
// vendored OpenAPI spec.
func loadSpecComponentSchemas(t *testing.T) map[string]any {
	t.Helper()
	specPath := filepath.Join(repoRoot(t), "api", "openapi", "v1.yaml")
	b, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}
	return doc.Components.Schemas
}

func TestOpenAPIContract_ExternalRefSchemaDefined(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	if _, ok := schemas["ExternalRef"]; !ok {
		t.Fatal("components.schemas.ExternalRef not defined in OpenAPI spec")
	}
}

func TestOpenAPIContract_ExternalRefSchemaReferenced(t *testing.T) {
	// Each of these five response schemas gained an `external_ref`
	// property in PR 3. The property must $ref ExternalRef.
	wantReferencingSchemas := []string{
		"BusinessService",
		"BusinessServiceRelationship",
		"AISystem",
		"AISystemVersion",
		"AISystemBinding",
	}

	schemas := loadSpecComponentSchemas(t)

	const wantRef = "#/components/schemas/ExternalRef"

	for _, schemaName := range wantReferencingSchemas {
		t.Run(schemaName, func(t *testing.T) {
			schema, ok := schemas[schemaName]
			if !ok {
				t.Fatalf("schema %q missing from spec", schemaName)
			}
			schemaMap, ok := schema.(map[string]any)
			if !ok {
				t.Fatalf("schema %q is not a map: %T", schemaName, schema)
			}
			props, ok := schemaMap["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema %q has no properties map", schemaName)
			}
			extProp, ok := props["external_ref"]
			if !ok {
				t.Fatalf("schema %q missing `external_ref` property", schemaName)
			}
			// The property uses `allOf: [{$ref: ...}]` per OpenAPI 3.0
			// nullable-with-ref pattern. Walk the structure to find the
			// referenced path verbatim.
			if !containsRef(extProp, wantRef) {
				t.Errorf("schema %q.external_ref does not reference %q; got %+v",
					schemaName, wantRef, extProp)
			}
		})
	}
}

// containsRef recursively walks a parsed YAML node and returns true if
// any `$ref` value equals target. Handles the `allOf: [{$ref: ...}]`
// nullable-with-ref pattern as well as a plain top-level $ref.
func containsRef(node any, target string) bool {
	switch v := node.(type) {
	case map[string]any:
		for k, val := range v {
			if k == "$ref" {
				if s, ok := val.(string); ok && s == target {
					return true
				}
			}
			if containsRef(val, target) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsRef(item, target) {
				return true
			}
		}
	case string:
		// leaf — already handled above when key was "$ref"
	}
	return false
}

// TestOpenAPIContract_ExternalRefRequiredFields confirms the schema's
// required list still includes source_system + source_id. The
// consistency invariant is enforced at the application layer (validator)
// and database layer (CHECK); the OpenAPI required list documents it
// for clients.
func TestOpenAPIContract_ExternalRefRequiredFields(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	schema, ok := schemas["ExternalRef"].(map[string]any)
	if !ok {
		t.Fatal("ExternalRef schema not a map")
	}
	requiredRaw, ok := schema["required"].([]any)
	if !ok {
		t.Fatal("ExternalRef schema has no required list")
	}
	got := make([]string, 0, len(requiredRaw))
	for _, r := range requiredRaw {
		if s, ok := r.(string); ok {
			got = append(got, s)
		}
	}
	wantSet := map[string]bool{"source_system": false, "source_id": false}
	for _, name := range got {
		if _, ok := wantSet[name]; ok {
			wantSet[name] = true
		}
	}
	for name, found := range wantSet {
		if !found {
			t.Errorf("ExternalRef.required missing %q (got %v)", name, got)
		}
	}
}

// TestOpenAPIContract_ExternalRefHasNullableProperty confirms the
// response-schema usage pattern: each `external_ref` property is
// nullable so absent values render as JSON null (matching the
// toExternalRefResponse contract that nil-or-IsZero returns nil).
func TestOpenAPIContract_ExternalRefHasNullableProperty(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	for _, schemaName := range []string{
		"BusinessService", "BusinessServiceRelationship",
		"AISystem", "AISystemVersion", "AISystemBinding",
	} {
		t.Run(schemaName, func(t *testing.T) {
			schema, ok := schemas[schemaName].(map[string]any)
			if !ok {
				t.Fatalf("schema %q missing", schemaName)
			}
			props, _ := schema["properties"].(map[string]any)
			extProp, _ := props["external_ref"].(map[string]any)
			if extProp == nil {
				t.Fatalf("external_ref property missing on %q", schemaName)
			}
			// nullable: true must be present; otherwise the spec implies
			// absent values are forbidden, which contradicts the wire
			// shape (rendered as null when absent).
			if nullable, _ := extProp["nullable"].(bool); !nullable {
				t.Errorf("external_ref on %q must be nullable", schemaName)
			}
		})
	}
}
