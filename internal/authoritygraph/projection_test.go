package authoritygraph

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
