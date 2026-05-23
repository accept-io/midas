package httpapi

// openapi_enum_drift_test.go — regression pin guarding the OpenAPI spec
// against drift from the Go-runtime enum constants it documents.
//
// Existence rationale: D37x-followup-1 surfaced that
// api/openapi/v1.yaml's EvaluateResponse.outcome enum declared "execute"
// for ~11 days while the Go runtime emitted "accept" and the schema's
// CHECK constraint permitted only "accept". This test pins each
// (Go-runtime, OpenAPI) enum pair so the next drift fails loudly at
// `go test` time, not in a downstream consumer.
//
// Coverage scope is documented in the D37x-followup-3 final report.
// Adding a new pair is a one-row append to openapiEnumPinCases. The
// case table supports two OpenAPI addressing modes:
//
//   - nested property:  ("Schema", ["prop"])   → components.schemas.Schema.properties.prop.enum
//   - top-level enum:   ("Schema", nil/empty)  → components.schemas.Schema.enum

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
	"gopkg.in/yaml.v3"
)

// openapiEnumDriftCase pairs a Go-runtime enum's wire values against the
// OpenAPI schema enum that documents the same wire contract. The pin is
// bidirectional: the test fails if either side adds or removes a value
// relative to the other.
//
// propertyPath addressing:
//   - non-empty: walk components.schemas[schemaName].properties[path...].enum
//   - empty:     read components.schemas[schemaName].enum directly
//
// To pin a new enum, append an entry below.
type openapiEnumDriftCase struct {
	name         string   // human-readable case name; surfaces on failure
	schemaName   string   // OpenAPI component schema name
	propertyPath []string // nil/empty for top-level enum schemas
	runtime      []string // canonical Go-runtime wire values; order-independent
}

// openapiEnumPinCases is the canonical set of (Go-runtime, OpenAPI)
// enum pairings this package guards. Grouped by domain for readability.
var openapiEnumPinCases = []openapiEnumDriftCase{
	// ----- eval -----
	{
		name:         "eval.Outcome",
		schemaName:   "EvaluateResponse",
		propertyPath: []string{"outcome"},
		runtime: []string{
			string(eval.OutcomeAccept),
			string(eval.OutcomeEscalate),
			string(eval.OutcomeReject),
			string(eval.OutcomeRequestClarification),
		},
	},

	// ----- failmode (rule axes) -----
	{
		name:         "failmode.CorrectnessClass",
		schemaName:   "FailModePolicyRule",
		propertyPath: []string{"correctness_class"},
		runtime: []string{
			string(failmode.CorrectnessClassGovernanceIntegrity),
			string(failmode.CorrectnessClassPersistence),
			string(failmode.CorrectnessClassInput),
			string(failmode.CorrectnessClassResource),
			string(failmode.CorrectnessClassConsistency),
		},
	},
	{
		name:         "failmode.PermittedMode",
		schemaName:   "FailModePolicyRule",
		propertyPath: []string{"permitted_mode"},
		runtime: []string{
			string(failmode.PermittedModeClosed),
			string(failmode.PermittedModeSoft),
			string(failmode.PermittedModeOpen),
			string(failmode.PermittedModeNotApplicable),
		},
	},
	{
		name:         "failmode.EnforcementState",
		schemaName:   "FailModePolicyRule",
		propertyPath: []string{"enforcement_state"},
		runtime: []string{
			string(failmode.EnforcementStateEvidenceOnly),
			string(failmode.EnforcementStateDryRun),
			string(failmode.EnforcementStateEnforced),
		},
	},
	{
		name:         "failmode.Outcome",
		schemaName:   "FailModePolicyRule",
		propertyPath: []string{"outcome"},
		runtime: []string{
			string(failmode.OutcomeDeny),
			string(failmode.OutcomeEscalate),
			string(failmode.OutcomePermitWithEvidence),
			string(failmode.OutcomeManualReview),
		},
	},

	// ----- lifecycle (all five Go types share the canonical
	//       LifecycleStatus component introduced in D37x-followup-3) -----
	{
		name:       "surface.SurfaceStatus -> LifecycleStatus",
		schemaName: "LifecycleStatus",
		runtime: []string{
			string(surface.SurfaceStatusDraft),
			string(surface.SurfaceStatusReview),
			string(surface.SurfaceStatusActive),
			string(surface.SurfaceStatusDeprecated),
			string(surface.SurfaceStatusRetired),
		},
	},
	{
		name:       "authority.ProfileStatus -> LifecycleStatus",
		schemaName: "LifecycleStatus",
		runtime: []string{
			string(authority.ProfileStatusDraft),
			string(authority.ProfileStatusReview),
			string(authority.ProfileStatusActive),
			string(authority.ProfileStatusDeprecated),
			string(authority.ProfileStatusRetired),
		},
	},
	{
		name:       "failmode.FailModePolicyStatus -> LifecycleStatus",
		schemaName: "LifecycleStatus",
		runtime: []string{
			string(failmode.FailModePolicyStatusDraft),
			string(failmode.FailModePolicyStatusReview),
			string(failmode.FailModePolicyStatusActive),
			string(failmode.FailModePolicyStatusDeprecated),
			string(failmode.FailModePolicyStatusRetired),
		},
	},
	{
		name:       "escalation.Status -> LifecycleStatus",
		schemaName: "LifecycleStatus",
		runtime: []string{
			string(escalation.StatusDraft),
			string(escalation.StatusReview),
			string(escalation.StatusActive),
			string(escalation.StatusDeprecated),
			string(escalation.StatusRetired),
		},
	},
	{
		name:       "drift.DriftDefinitionStatus -> LifecycleStatus",
		schemaName: "LifecycleStatus",
		runtime: []string{
			string(drift.DriftDefinitionStatusDraft),
			string(drift.DriftDefinitionStatusReview),
			string(drift.DriftDefinitionStatusActive),
			string(drift.DriftDefinitionStatusDeprecated),
			string(drift.DriftDefinitionStatusRetired),
		},
	},

	// ----- envelope -----
	{
		name:         "envelope.EnvelopeState",
		schemaName:   "Envelope",
		propertyPath: []string{"state"},
		runtime: []string{
			string(envelope.EnvelopeStateReceived),
			string(envelope.EnvelopeStateEvaluating),
			string(envelope.EnvelopeStateOutcomeRecorded),
			string(envelope.EnvelopeStateEscalated),
			string(envelope.EnvelopeStateAwaitingReview),
			string(envelope.EnvelopeStateClosed),
		},
	},

	// ----- agent -----
	{
		name:         "agent.AgentType",
		schemaName:   "Agent",
		propertyPath: []string{"type"},
		runtime: []string{
			string(agent.AgentTypeAI),
			string(agent.AgentTypeService),
			string(agent.AgentTypeOperator),
		},
	},
	{
		name:         "agent.OperationalState",
		schemaName:   "Agent",
		propertyPath: []string{"operational_state"},
		runtime: []string{
			string(agent.OperationalStateActive),
			string(agent.OperationalStateSuspended),
			string(agent.OperationalStateRevoked),
		},
	},

	// ----- authority (grant / escalation / fail-mode) -----
	//
	// authority.GrantStatus is pinned against BOTH OpenAPI sites that
	// declare it — the declarative shape (Grant.status) and the
	// operational response shape (GrantLifecycleResponse.status). The
	// followup-3 audit found the two sites had drifted (Grant.status
	// carried a spec-only "expired" value); followup-4 removed the
	// drift and added the sibling pin below to prevent regression.
	{
		name:         "authority.GrantStatus (vs Grant)",
		schemaName:   "Grant",
		propertyPath: []string{"status"},
		runtime: []string{
			string(authority.GrantStatusActive),
			string(authority.GrantStatusSuspended),
			string(authority.GrantStatusRevoked),
		},
	},
	{
		name:         "authority.GrantStatus (vs GrantLifecycleResponse)",
		schemaName:   "GrantLifecycleResponse",
		propertyPath: []string{"status"},
		runtime: []string{
			string(authority.GrantStatusActive),
			string(authority.GrantStatusSuspended),
			string(authority.GrantStatusRevoked),
		},
	},
	{
		name:         "authority.EscalationMode",
		schemaName:   "Profile",
		propertyPath: []string{"escalation_mode"},
		runtime: []string{
			string(authority.EscalationModeAuto),
			string(authority.EscalationModeManual),
		},
	},
	{
		name:         "authority.FailMode",
		schemaName:   "Profile",
		propertyPath: []string{"fail_mode"},
		runtime: []string{
			string(authority.FailModeOpen),
			string(authority.FailModeClosed),
		},
	},

	// ----- value -----
	{
		name:         "value.ConsequenceType",
		schemaName:   "Consequence",
		propertyPath: []string{"type"},
		runtime: []string{
			string(value.ConsequenceTypeMonetary),
			string(value.ConsequenceTypeRiskRating),
		},
	},
	{
		name:         "value.RiskRating",
		schemaName:   "Consequence",
		propertyPath: []string{"risk_rating"},
		runtime: []string{
			string(value.RiskRatingLow),
			string(value.RiskRatingMedium),
			string(value.RiskRatingHigh),
			string(value.RiskRatingCritical),
		},
	},

	// ----- adminaudit -----
	{
		name:         "adminaudit.Action",
		schemaName:   "AdminAuditEntry",
		propertyPath: []string{"action"},
		runtime: []string{
			string(adminaudit.ActionApplyInvoked),
			string(adminaudit.ActionPasswordChanged),
			string(adminaudit.ActionBootstrapAdminCreated),
		},
	},
	{
		name:         "adminaudit.Outcome",
		schemaName:   "AdminAuditEntry",
		propertyPath: []string{"outcome"},
		runtime: []string{
			string(adminaudit.OutcomeSuccess),
			string(adminaudit.OutcomeFailure),
		},
	},
	{
		name:         "adminaudit.ActorType",
		schemaName:   "AdminAuditEntry",
		propertyPath: []string{"actor_type"},
		runtime: []string{
			string(adminaudit.ActorTypeUser),
			string(adminaudit.ActorTypeSystem),
		},
	},

	// ----- escalation -----
	{
		name:       "escalation.Kind",
		schemaName: "EscalationTargetKind",
		runtime: []string{
			string(escalation.KindRole),
			string(escalation.KindAgent),
			string(escalation.KindSurface),
			string(escalation.KindExternal),
		},
	},
}

// TestOpenAPIEnumsMatchRuntimeConstants asserts that every pinned
// (Go-runtime enum, OpenAPI schema enum) pair has identical value sets.
// See openapiEnumPinCases for the covered enums.
func TestOpenAPIEnumsMatchRuntimeConstants(t *testing.T) {
	doc := loadOpenAPIDoc(t)
	for _, c := range openapiEnumPinCases {
		t.Run(c.name, func(t *testing.T) {
			var specValues []string
			if len(c.propertyPath) == 0 {
				specValues = schemaTopLevelEnum(t, doc, c.schemaName)
			} else {
				specValues = schemaEnum(t, doc, c.schemaName, c.propertyPath)
			}
			assertEnumSetEqual(t, c.name, c.runtime, specValues)
		})
	}
}

// loadOpenAPIDoc parses api/openapi/v1.yaml into a generic map for
// structural traversal. Uses gopkg.in/yaml.v3 (already a direct
// dependency; see go.mod and openapi_contract_test.go).
func loadOpenAPIDoc(t *testing.T) map[string]any {
	t.Helper()
	specPath := filepath.Join(repoRoot(t), "api", "openapi", "v1.yaml")
	b, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}
	return doc
}

// schemaEnum walks components.schemas[schemaName].properties[path...]
// and returns the leaf .enum array as []string. Fails the test if the
// path does not resolve or the leaf is not a string list.
func schemaEnum(t *testing.T, doc map[string]any, schemaName string, path []string) []string {
	t.Helper()
	if len(path) == 0 {
		t.Fatalf("schemaEnum: empty path for schema %q; use schemaTopLevelEnum for top-level enums",
			schemaName)
	}
	properties := schemaProperties(t, doc, schemaName)
	cur := properties
	for i, segment := range path {
		next, ok := cur[segment].(map[string]any)
		if !ok {
			t.Fatalf("openapi schema %q: no property at path %v (failed at %q index %d)",
				schemaName, path, segment, i)
		}
		if i == len(path)-1 {
			return enumStrings(t, schemaName, path, next)
		}
		nestedProps, ok := next["properties"].(map[string]any)
		if !ok {
			t.Fatalf("openapi schema %q: no nested properties at segment %q",
				schemaName, segment)
		}
		cur = nestedProps
	}
	t.Fatalf("openapi schema %q: walked empty path", schemaName)
	return nil
}

// schemaTopLevelEnum returns components.schemas[schemaName].enum as
// []string. Use this for schemas that ARE an enum (e.g. LifecycleStatus,
// EscalationTargetKind) rather than schemas that contain a property
// with an enum.
func schemaTopLevelEnum(t *testing.T, doc map[string]any, schemaName string) []string {
	t.Helper()
	schemas := schemaComponents(t, doc)
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("openapi schema %q not found under components.schemas", schemaName)
	}
	enum, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("openapi schema %q has no top-level enum array (got: %T); is it perhaps a properties-bearing schema? use schemaEnum instead",
			schemaName, schema["enum"])
	}
	out := make([]string, 0, len(enum))
	for j, v := range enum {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("openapi schema %q enum[%d] is %T, want string", schemaName, j, v)
		}
		out = append(out, s)
	}
	return out
}

// schemaComponents returns the components.schemas map. Fails the test
// if the doc is missing the expected structure.
func schemaComponents(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("openapi spec missing top-level components map")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatalf("openapi spec missing components.schemas map")
	}
	return schemas
}

// schemaProperties returns components.schemas[schemaName].properties.
// Fails the test if the schema is missing or has no properties.
func schemaProperties(t *testing.T, doc map[string]any, schemaName string) map[string]any {
	t.Helper()
	schemas := schemaComponents(t, doc)
	schema, ok := schemas[schemaName].(map[string]any)
	if !ok {
		t.Fatalf("openapi schema %q not found under components.schemas", schemaName)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("openapi schema %q has no properties map", schemaName)
	}
	return properties
}

// enumStrings extracts an enum array from a leaf property node and
// returns it as []string. Fails the test if the node lacks an enum or
// the enum contains a non-string value.
func enumStrings(t *testing.T, schemaName string, path []string, node map[string]any) []string {
	t.Helper()
	enum, ok := node["enum"].([]any)
	if !ok {
		t.Fatalf("openapi schema %q property %v has no enum array", schemaName, path)
	}
	out := make([]string, 0, len(enum))
	for j, v := range enum {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("openapi schema %q property %v enum[%d] is %T, want string",
				schemaName, path, j, v)
		}
		out = append(out, s)
	}
	return out
}

// assertEnumSetEqual reports any element in want missing from got, and any
// element in got missing from want. Ordering is intentionally not part of
// the contract — enum value order in YAML carries no semantic meaning.
func assertEnumSetEqual(t *testing.T, name string, want, got []string) {
	t.Helper()
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	sortedGot := append([]string(nil), got...)
	sort.Strings(sortedGot)

	wantSet := make(map[string]struct{}, len(sortedWant))
	for _, v := range sortedWant {
		wantSet[v] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(sortedGot))
	for _, v := range sortedGot {
		gotSet[v] = struct{}{}
	}

	var missingFromSpec, extraInSpec []string
	for _, v := range sortedWant {
		if _, ok := gotSet[v]; !ok {
			missingFromSpec = append(missingFromSpec, v)
		}
	}
	for _, v := range sortedGot {
		if _, ok := wantSet[v]; !ok {
			extraInSpec = append(extraInSpec, v)
		}
	}
	if len(missingFromSpec) == 0 && len(extraInSpec) == 0 {
		return
	}
	t.Errorf("%s: OpenAPI enum drift vs Go runtime", name)
	t.Errorf("  runtime: %v", sortedWant)
	t.Errorf("  openapi: %v", sortedGot)
	if len(missingFromSpec) > 0 {
		t.Errorf("  in Go runtime but missing from OpenAPI: %v", missingFromSpec)
	}
	if len(extraInSpec) > 0 {
		t.Errorf("  in OpenAPI but missing from Go runtime: %v", extraInSpec)
	}
}
