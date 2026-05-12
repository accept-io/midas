package httpapi

// openapi_drift_lifecycle_test.go — presence pins for the Drift-1e
// lifecycle path entries and request-schema definitions in
// api/openapi/v1.yaml. Lockstep with Server.routes() is enforced by
// the existing TestOpenAPIContract_PathSymmetry; this file adds
// a focused content pin so a regression that quietly drops a path
// from the YAML surfaces against the right test name.

import (
	"strings"
	"testing"
)

func TestOpenAPI_DriftLifecyclePaths_AllPresent(t *testing.T) {
	src := driftOpenAPISpec(t)
	for _, p := range []string{
		"/v1/controlplane/drift_definitions/{id}/submit:",
		"/v1/controlplane/drift_definitions/{id}/approve:",
		"/v1/controlplane/drift_definitions/{id}/reject:",
		"/v1/controlplane/drift_definitions/{id}/deprecate:",
		"/v1/controlplane/drift_definitions/{id}/retire:",
	} {
		if !strings.Contains(src, p) {
			t.Errorf("OpenAPI spec missing drift lifecycle path: %s", p)
		}
	}
}

func TestOpenAPI_DriftLifecycleRequestSchemas_AllPresent(t *testing.T) {
	src := driftOpenAPISpec(t)
	for _, schema := range []string{
		"SubmitDriftDefinitionRequest:",
		"ApproveDriftDefinitionRequest:",
		"RejectDriftDefinitionRequest:",
		"DeprecateDriftDefinitionRequest:",
		"RetireDriftDefinitionRequest:",
	} {
		if !strings.Contains(src, schema) {
			t.Errorf("OpenAPI spec missing drift lifecycle request schema: %s", schema)
		}
	}
}

func TestOpenAPI_DriftLifecycle_ResponsesUseDriftDefinitionDTO(t *testing.T) {
	// Each lifecycle path's 200 response references DriftDefinition
	// (the Drift-1d full DTO), not a smaller action-specific shape.
	src := driftOpenAPISpec(t)
	for _, op := range []string{
		"submitDriftDefinition",
		"approveDriftDefinition",
		"rejectDriftDefinition",
		"deprecateDriftDefinition",
		"retireDriftDefinition",
	} {
		idx := strings.Index(src, "operationId: "+op+"\n")
		if idx < 0 {
			t.Errorf("operation %q missing", op)
			continue
		}
		// Look for the next "$ref:" in the operation body — should
		// be the request body schema (a DriftXxxRequest); then
		// the response 200 ref should be DriftDefinition. Cheaper
		// approach: assert the 200-response ref appears before the
		// next operationId.
		tail := src[idx:]
		nextOp := strings.Index(tail[len("operationId: "+op):], "operationId: ")
		if nextOp < 0 {
			nextOp = len(tail)
		} else {
			nextOp += len("operationId: " + op)
		}
		opBlock := tail[:nextOp]
		if !strings.Contains(opBlock, `$ref: "#/components/schemas/DriftDefinition"`) {
			t.Errorf("operation %q must reference DriftDefinition in its 200 response", op)
		}
	}
}
