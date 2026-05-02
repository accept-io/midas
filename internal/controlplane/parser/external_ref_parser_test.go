package parser

// Parser coverage for the optional ExternalRef field that five
// document specs gained in Epic 1, PR 3. The parser uses YAML's
// generic struct decoder — these tests confirm the new field
// round-trips on every consuming kind, and that omitting it produces
// a nil spec (not an empty struct).

import (
	"testing"

	"github.com/accept-io/midas/internal/controlplane/types"
)

const externalRefBSYAML = `apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-extref
  name: BS
spec:
  service_type: internal
  status: active
  external_ref:
    source_system: github
    source_id: accept-io/midas
    source_url: https://github.com/accept-io/midas
    source_version: v1.2.0
    last_synced_at: 2026-04-30T09:00:00Z`

const externalRefBSRYAML = `apiVersion: midas.accept.io/v1
kind: BusinessServiceRelationship
metadata:
  id: rel-extref
spec:
  source_business_service_id: bs-a
  target_business_service_id: bs-b
  relationship_type: depends_on
  external_ref:
    source_system: leanix
    source_id: factsheet-1234`

const externalRefAISystemYAML = `apiVersion: midas.accept.io/v1
kind: AISystem
metadata:
  id: ai-extref
  name: AI
spec:
  status: active
  external_ref:
    source_system: servicenow
    source_id: AI-INV-001
    source_url: https://internal.servicenow/now/nav/ui/classic/params/target/AI-INV-001
    source_version: rev-7
    last_synced_at: 2026-04-30T09:00:00Z`

const externalRefAIVersionYAML = `apiVersion: midas.accept.io/v1
kind: AISystemVersion
metadata:
  id: aiv-extref
spec:
  ai_system_id: ai-extref
  version: 1
  status: active
  effective_from: 2026-04-15T00:00:00Z
  external_ref:
    source_system: github
    source_id: accept-io/lending-models/v1.2.0
    source_version: v1.2.0`

const externalRefAIBindingYAML = `apiVersion: midas.accept.io/v1
kind: AISystemBinding
metadata:
  id: bind-extref
spec:
  ai_system_id: ai-extref
  business_service_id: bs-x
  external_ref:
    source_system: custom
    source_id: internal-catalog/binding/42`

const externalRefOmittedYAML = `apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-no-ext
  name: BS
spec:
  service_type: internal
  status: active`

func TestParser_ExternalRef_RoundTripsAcrossAllFiveKinds(t *testing.T) {
	cases := []struct {
		name   string
		yaml   string
		assert func(t *testing.T, doc ParsedDocument)
	}{
		{
			name: "BusinessService",
			yaml: externalRefBSYAML,
			assert: func(t *testing.T, doc ParsedDocument) {
				bs := doc.Doc.(types.BusinessServiceDocument)
				if bs.Spec.ExternalRef == nil {
					t.Fatal("ExternalRef nil")
				}
				if bs.Spec.ExternalRef.SourceSystem != "github" || bs.Spec.ExternalRef.SourceID != "accept-io/midas" {
					t.Errorf("system/id mismatch: %+v", bs.Spec.ExternalRef)
				}
				if bs.Spec.ExternalRef.LastSyncedAt != "2026-04-30T09:00:00Z" {
					t.Errorf("last_synced_at: got %q", bs.Spec.ExternalRef.LastSyncedAt)
				}
			},
		},
		{
			name: "BusinessServiceRelationship",
			yaml: externalRefBSRYAML,
			assert: func(t *testing.T, doc ParsedDocument) {
				rel := doc.Doc.(types.BusinessServiceRelationshipDocument)
				if rel.Spec.ExternalRef == nil || rel.Spec.ExternalRef.SourceSystem != "leanix" {
					t.Errorf("ExternalRef not parsed: %+v", rel.Spec.ExternalRef)
				}
			},
		},
		{
			name: "AISystem",
			yaml: externalRefAISystemYAML,
			assert: func(t *testing.T, doc ParsedDocument) {
				sys := doc.Doc.(types.AISystemDocument)
				if sys.Spec.ExternalRef == nil || sys.Spec.ExternalRef.SourceSystem != "servicenow" {
					t.Errorf("ExternalRef not parsed: %+v", sys.Spec.ExternalRef)
				}
				if sys.Spec.ExternalRef.SourceVersion != "rev-7" {
					t.Errorf("SourceVersion: %q", sys.Spec.ExternalRef.SourceVersion)
				}
			},
		},
		{
			name: "AISystemVersion",
			yaml: externalRefAIVersionYAML,
			assert: func(t *testing.T, doc ParsedDocument) {
				ver := doc.Doc.(types.AISystemVersionDocument)
				if ver.Spec.ExternalRef == nil || ver.Spec.ExternalRef.SourceID != "accept-io/lending-models/v1.2.0" {
					t.Errorf("ExternalRef not parsed: %+v", ver.Spec.ExternalRef)
				}
				// last_synced_at is optional and absent here.
				if ver.Spec.ExternalRef.LastSyncedAt != "" {
					t.Errorf("LastSyncedAt should be empty; got %q", ver.Spec.ExternalRef.LastSyncedAt)
				}
			},
		},
		{
			name: "AISystemBinding",
			yaml: externalRefAIBindingYAML,
			assert: func(t *testing.T, doc ParsedDocument) {
				b := doc.Doc.(types.AISystemBindingDocument)
				if b.Spec.ExternalRef == nil || b.Spec.ExternalRef.SourceSystem != "custom" {
					t.Errorf("ExternalRef not parsed: %+v", b.Spec.ExternalRef)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ParseYAML([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("ParseYAML: %v", err)
			}
			tc.assert(t, doc)
		})
	}
}

func TestParser_ExternalRef_OmittedProducesNilSpec(t *testing.T) {
	doc, err := ParseYAML([]byte(externalRefOmittedYAML))
	if err != nil {
		t.Fatalf("ParseYAML: %v", err)
	}
	bs := doc.Doc.(types.BusinessServiceDocument)
	if bs.Spec.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef when YAML omits the field; got %+v", bs.Spec.ExternalRef)
	}
}
