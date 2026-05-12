package httpapi

// openapi_drift_test.go — content pins for the Drift-1d enums in
// api/openapi/v1.yaml. Guards against the V2 drift types and the
// single 'unknown' status creeping back into the spec.

import (
	"os"
	"strings"
	"testing"
)

func driftOpenAPISpec(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	return string(b)
}

// driftSchemaSlice slices the v1.yaml file from the named schema's
// header line up to the next top-level schema header. Sufficient for
// substring assertions scoped to one component schema body. The
// terminator is "\n\n    " (blank line + 4-space indent), which only
// matches the gap between adjacent schema definitions; intra-schema
// continuation lines use deeper indents and never include a blank
// line.
func driftSchemaSlice(t *testing.T, schemaName string) string {
	t.Helper()
	src := driftOpenAPISpec(t)
	header := "    " + schemaName + ":\n"
	i := strings.Index(src, header)
	if i < 0 {
		t.Fatalf("schema %q not found in spec", schemaName)
	}
	tail := src[i+len(header):]
	end := strings.Index(tail, "\n\n    ") // gap between adjacent schemas
	if end < 0 {
		return tail
	}
	return tail[:end]
}

func TestOpenAPI_DriftType_OnlyV1Values(t *testing.T) {
	body := driftSchemaSlice(t, "DriftType")
	for _, want := range []string{
		"invocation", "outcome", "confidence", "latency", "evidence",
		"authority", "policy", "coverage", "process_path",
	} {
		if !strings.Contains(body, "- "+want) {
			t.Errorf("DriftType enum missing %q", want)
		}
	}
	for _, banned := range []string{"population", "data", "prediction", "performance", "concept"} {
		if strings.Contains(body, "- "+banned) {
			t.Errorf("DriftType enum must not contain V2-deferred %q", banned)
		}
	}
}

func TestOpenAPI_DriftStatusBand_FiveBandsExactly(t *testing.T) {
	body := driftSchemaSlice(t, "DriftStatusBand")
	for _, want := range []string{
		"healthy", "warning", "breached",
		"unknown_insufficient_data", "unknown_detector_error",
	} {
		if !strings.Contains(body, "- "+want) {
			t.Errorf("DriftStatusBand enum missing %q", want)
		}
	}
	// A bare "unknown" entry must not appear. We grep for the
	// YAML-list form "        - unknown\n" exactly to avoid matching
	// the longer unknown_* values.
	if strings.Contains(body, "- unknown\n") {
		t.Error("DriftStatusBand must not contain a bare 'unknown' value")
	}
}

func TestOpenAPI_DriftBaselineStrategy_V1ValuesOnly(t *testing.T) {
	body := driftSchemaSlice(t, "DriftBaselineStrategy")
	for _, want := range []string{
		"rolling", "fixed_governed", "previous_equivalent",
		"since_last_governed_change", "manually_pinned",
	} {
		if !strings.Contains(body, "- "+want) {
			t.Errorf("DriftBaselineStrategy enum missing %q", want)
		}
	}
	for _, banned := range []string{"champion_challenger", "seasonality_aware"} {
		if strings.Contains(body, "- "+banned) {
			t.Errorf("DriftBaselineStrategy enum must not contain V2-deferred %q", banned)
		}
	}
}

func TestOpenAPI_DriftPointComputationMode_FourValues(t *testing.T) {
	body := driftSchemaSlice(t, "DriftPointComputationMode")
	for _, want := range []string{"realtime", "backfilled", "corrected", "imported"} {
		if !strings.Contains(body, "- "+want) {
			t.Errorf("DriftPointComputationMode enum missing %q", want)
		}
	}
}

func TestOpenAPI_DriftTargetEntityKind_NineValues(t *testing.T) {
	body := driftSchemaSlice(t, "DriftTargetEntityKind")
	for _, want := range []string{
		"business_service", "capability", "process",
		"decision_surface", "ai_system", "ai_system_binding",
		"agent", "authority_profile", "authority_grant",
	} {
		if !strings.Contains(body, "- "+want) {
			t.Errorf("DriftTargetEntityKind enum missing %q", want)
		}
	}
}

func TestOpenAPI_DriftPaths_AllPresent(t *testing.T) {
	src := driftOpenAPISpec(t)
	wantPaths := []string{
		"/v1/drift/definitions/{id}:",
		"/v1/drift/definitions/{id}/versions:",
		"/v1/drift/definitions/{id}/versions/{version}:",
		"/v1/drift/definitions/{id}/series:",
		"/v1/drift/definitions/{id}/observations:",
		"/v1/drift/series/{id}:",
		"/v1/drift/series/{id}/points:",
		"/v1/drift/series/{id}/observations:",
		"/v1/drift/series/{id}/annotations:",
		"/v1/drift/series-points/{id}:",
		"/v1/drift/observations/{id}:",
		"/v1/drift/observations/{id}/annotations:",
		"/v1/drift/annotations/{id}:",
		"/v1/drift/entities/{kind}/{entity_id}/observations:",
	}
	for _, p := range wantPaths {
		if !strings.Contains(src, p) {
			t.Errorf("OpenAPI spec missing drift path: %s", p)
		}
	}
}
