package httpapi

// openapi_escalation_target_test.go — D31k-impl-1 contract pins for the
// EscalationTarget read API. Pins paths declared, component schemas
// present, and exact enum values for Kind + Status.

import (
	"strings"
	"testing"
)

func TestOpenAPIContract_EscalationTargetPathsDeclared(t *testing.T) {
	paths := loadSpecPaths(t)
	want := []string{
		"/v1/escalation_targets",
		"/v1/escalation_targets/{id}",
		"/v1/escalation_targets/{id}/versions",
		"/v1/escalation_targets/{id}/versions/{version}",
	}
	have := map[string]bool{}
	for _, p := range paths {
		have[p] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("OpenAPI spec missing path %q", w)
		}
	}
}

func TestOpenAPIContract_EscalationTargetSchemasDeclared(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	for _, name := range []string{
		"EscalationTarget",
		"EscalationTargetKind",
		"EscalationTargetStatus",
		"EscalationTargetListResponse",
	} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("components.schemas.%s missing from OpenAPI spec", name)
		}
	}
}

func TestOpenAPIContract_EscalationTargetKind_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	k, ok := schemas["EscalationTargetKind"].(map[string]any)
	if !ok {
		t.Fatal("EscalationTargetKind schema not a map")
	}
	enum, ok := k["enum"].([]any)
	if !ok {
		t.Fatal("EscalationTargetKind.enum missing")
	}
	want := map[string]bool{"role": true, "agent": true, "surface": true, "external": true}
	got := map[string]bool{}
	for _, v := range enum {
		if s, ok := v.(string); ok {
			got[s] = true
		}
	}
	if len(enum) != len(want) {
		t.Errorf("EscalationTargetKind enum size: want %d, got %d (%v)", len(want), len(enum), enum)
	}
	for w := range want {
		if !got[w] {
			t.Errorf("EscalationTargetKind enum missing %q", w)
		}
	}
}

func TestOpenAPIContract_EscalationTargetStatus_Enum(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	s, ok := schemas["EscalationTargetStatus"].(map[string]any)
	if !ok {
		t.Fatal("EscalationTargetStatus schema not a map")
	}
	// After D37x-followup-3, EscalationTargetStatus delegates to the
	// canonical LifecycleStatus component via `allOf: [$ref: ...]`.
	// Resolve the enum through the alias chain so this pin keeps its
	// original intent — "EscalationTargetStatus yields the 5-value
	// lifecycle posture" — independent of which schema actually
	// carries the enum array.
	enum := resolveLifecycleEnum(t, "EscalationTargetStatus", s, schemas)
	want := map[string]bool{
		"draft": true, "review": true, "active": true,
		"deprecated": true, "retired": true,
	}
	got := map[string]bool{}
	for _, v := range enum {
		if str, ok := v.(string); ok {
			got[str] = true
		}
	}
	if len(enum) != len(want) {
		t.Errorf("EscalationTargetStatus enum size: want %d, got %d (%v)", len(want), len(enum), enum)
	}
	for w := range want {
		if !got[w] {
			t.Errorf("EscalationTargetStatus enum missing %q", w)
		}
	}
}

// resolveLifecycleEnum returns the enum array for a schema that either
// (a) declares enum inline, or (b) delegates to another schema via
// `allOf: [{$ref: "#/components/schemas/<Name>"}]`. The single-hop
// indirection introduced by D37x-followup-3's LifecycleStatus
// canonicalisation is the only shape this resolver needs to handle.
func resolveLifecycleEnum(t *testing.T, schemaName string, s, schemas map[string]any) []any {
	t.Helper()
	if enum, ok := s["enum"].([]any); ok {
		return enum
	}
	allOf, ok := s["allOf"].([]any)
	if !ok {
		t.Fatalf("%s: schema carries neither enum nor allOf", schemaName)
	}
	const refPrefix = "#/components/schemas/"
	for _, item := range allOf {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ref, ok := entry["$ref"].(string)
		if !ok || !strings.HasPrefix(ref, refPrefix) {
			continue
		}
		target := strings.TrimPrefix(ref, refPrefix)
		ts, ok := schemas[target].(map[string]any)
		if !ok {
			t.Fatalf("%s: allOf $ref target %q not found under components.schemas", schemaName, target)
		}
		if enum, ok := ts["enum"].([]any); ok {
			return enum
		}
	}
	t.Fatalf("%s: allOf entries did not resolve to a schema with an enum", schemaName)
	return nil
}

func TestOpenAPIContract_Profile_HasEscalationTargetID(t *testing.T) {
	schemas := loadSpecComponentSchemas(t)
	// The wire-shape schema for AuthorityProfile is exposed under
	// the canonical name "Profile" in v1.yaml (see /v1/profiles/*).
	// D31k-impl-1 adds escalation_target_id as an additive optional
	// property; the field is omitted/absent when no target is
	// configured.
	prof, ok := schemas["Profile"].(map[string]any)
	if !ok {
		t.Fatal("Profile schema not present")
	}
	props, ok := prof["properties"].(map[string]any)
	if !ok {
		t.Fatal("Profile properties missing")
	}
	if _, ok := props["escalation_target_id"].(map[string]any); !ok {
		t.Errorf("Profile.properties.escalation_target_id missing")
	}
}
