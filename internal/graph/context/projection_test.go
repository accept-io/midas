package contextgraph

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestProjection_JSONShape pins the wire-format field set: every
// expected key is present, and the Phase 1 forbidden fields (Attrs,
// Truncated) are absent. A round-trip ensures the tags decode the
// values back identically.
func TestProjection_JSONShape(t *testing.T) {
	p := Projection{
		Root:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		View:  ViewService,
		Depth: 3,
		Nodes: []Node{
			{Kind: NodeKindBusinessService, ID: "bs-1", Label: "BS One"},
			{Kind: NodeKindCapability, ID: "cap-1", Label: "Cap"},
		},
		Edges: []Edge{
			{
				Kind: EdgeKindHasCapability,
				Src:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
				Dst:  NodeRef{Kind: NodeKindCapability, ID: "cap-1"},
			},
		},
	}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)

	for _, want := range []string{
		`"root":`, `"view":`, `"depth":`, `"nodes":`, `"edges":`,
		`"kind":`, `"id":`, `"label":`,
		`"src":`, `"dst":`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing JSON key %q in %s", want, body)
		}
	}
	for _, illegal := range []string{`"attrs"`, `"truncated"`} {
		if strings.Contains(body, illegal) {
			t.Errorf("forbidden field %s present in %s", illegal, body)
		}
	}

	var got Projection
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Root.Kind != p.Root.Kind || got.Root.ID != p.Root.ID {
		t.Errorf("root: want %+v, got %+v", p.Root, got.Root)
	}
	if got.View != p.View || got.Depth != p.Depth {
		t.Errorf("view/depth: want (%q,%d), got (%q,%d)", p.View, p.Depth, got.View, got.Depth)
	}
	if len(got.Nodes) != len(p.Nodes) || len(got.Edges) != len(p.Edges) {
		t.Errorf("counts: want nodes=%d edges=%d, got nodes=%d edges=%d",
			len(p.Nodes), len(p.Edges), len(got.Nodes), len(got.Edges))
	}
}

// TestEdge_LabelOmitemptyOmitsAbsent pins that an edge with no label
// renders without a "label" key. Phase 1 has no edge labels, so the
// wire shape stays compact.
func TestEdge_LabelOmitemptyOmitsAbsent(t *testing.T) {
	e := Edge{
		Kind: EdgeKindHasCapability,
		Src:  NodeRef{Kind: NodeKindBusinessService, ID: "bs-1"},
		Dst:  NodeRef{Kind: NodeKindCapability, ID: "cap-1"},
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"label"`) {
		t.Errorf("edge label key must be absent when empty; got %s", raw)
	}
}

// TestBusinessServiceData_FailModePolicyID_OmitemptyJSONShape pins the
// D27j-ui-2a wire convention: an empty FailModePolicyID collapses
// (omitempty), a populated one renders the field. Other optional
// strings on the struct already use omitempty; this matches them.
func TestBusinessServiceData_FailModePolicyID_OmitemptyJSONShape(t *testing.T) {
	t.Run("empty omits the key", func(t *testing.T) {
		raw, err := json.Marshal(BusinessServiceData{ID: "bs-1", Name: "BS", Status: "active"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(raw), `"fail_mode_policy_id"`) {
			t.Errorf("fail_mode_policy_id key must be omitted when empty; got %s", raw)
		}
	})
	t.Run("populated renders the key and value", func(t *testing.T) {
		raw, err := json.Marshal(BusinessServiceData{
			ID: "bs-1", Name: "BS", Status: "active",
			FailModePolicyID: "policy-1",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body := string(raw)
		if !strings.Contains(body, `"fail_mode_policy_id":"policy-1"`) {
			t.Errorf("fail_mode_policy_id must serialise as \"policy-1\"; got %s", body)
		}
	})
}

// TestDecisionSurfaceData_FailModePolicyID_OmitemptyJSONShape mirrors
// the BusinessServiceData test for the surface struct.
func TestDecisionSurfaceData_FailModePolicyID_OmitemptyJSONShape(t *testing.T) {
	base := DecisionSurfaceData{
		ID: "surf-1", Version: 1, Name: "S", Status: "active", ProcessID: "proc-1",
		AIBindingIDs:          []string{},
		InheritedAIBindingIDs: []string{},
	}
	t.Run("empty omits the key", func(t *testing.T) {
		raw, err := json.Marshal(base)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(raw), `"fail_mode_policy_id"`) {
			t.Errorf("fail_mode_policy_id key must be omitted when empty; got %s", raw)
		}
	})
	t.Run("populated renders the key and value", func(t *testing.T) {
		d := base
		d.FailModePolicyID = "surf-policy"
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body := string(raw)
		if !strings.Contains(body, `"fail_mode_policy_id":"surf-policy"`) {
			t.Errorf("fail_mode_policy_id must serialise as \"surf-policy\"; got %s", body)
		}
	})
}
