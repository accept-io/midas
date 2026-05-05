// Package authoritygraph projects the existing governance map read
// model into a generic node/edge graph for the Authority Graph
// Workbench.
//
// Phase 1 supports a single perspective — view=service — which projects
// the same core information as
// GET /v1/businessservices/{id}/governance-map but in a generic
// nodes/edges shape suitable for traversal and depth-bounded queries.
//
// Phase 2A (this revision) closes the wire-content parity gap with the
// governance-map DTO without introducing a generic Attrs map. Each Node
// kind carries an optional typed data pointer (e.g. Node.BusinessService,
// Node.Coverage). Exactly one of the typed pointers is populated per
// node, matched to Kind. The omitempty tags collapse the unused
// pointers so a Node serialises to a compact shape with only its kind's
// data block.
//
// The package is strictly additive. It re-uses the existing
// governancemap.ReadService (no new repository reads, no schema
// change). Per-profile, per-grant, per-agent detail is out of scope —
// those node kinds are reserved for later phases.
package authoritygraph

// View identifies the perspective the projection is computed in.
//
// Phase 1/2A supports only ViewService. Other views (agent, AI system,
// decision, risk) are reserved for later phases.
const (
	ViewService = "service"
)

// Phase 1 node kinds — the complete allowed set. Forbidden kinds
// (authority_profile, authority_grant, agent, ai_system_version) are
// not exported here; their absence is the structural guard.
const (
	NodeKindBusinessService        = "business_service"
	NodeKindRelatedBusinessService = "related_business_service"
	NodeKindCapability             = "capability"
	NodeKindProcess                = "process"
	NodeKindDecisionSurface        = "decision_surface"
	NodeKindAISystem               = "ai_system"
	NodeKindAISystemBinding        = "ai_system_binding"
	NodeKindAuthoritySummary       = "authority_summary"
	NodeKindCoverage               = "coverage"
)

// Phase 1 edge kinds — the complete allowed set. Forbidden kinds
// (governed_by, has_grant, granted_to, has_active_version) are not
// exported here; their absence is the structural guard.
//
// Edge directionality is fixed (src → dst):
//
//   relates_to        business_service → related_business_service
//   has_capability    business_service → capability
//   has_process       business_service → process
//   has_surface       process → decision_surface
//   bound_to          ai_system_binding → most-specific scope target
//                     (decision_surface | process | capability | business_service)
//   system_of         ai_system_binding → ai_system
//   summarises        authority_summary → business_service (root)
//   reports_coverage  coverage → business_service (root)
const (
	EdgeKindRelatesTo       = "relates_to"
	EdgeKindHasCapability   = "has_capability"
	EdgeKindHasProcess      = "has_process"
	EdgeKindHasSurface      = "has_surface"
	EdgeKindBoundTo         = "bound_to"
	EdgeKindSystemOf        = "system_of"
	EdgeKindSummarises      = "summarises"
	EdgeKindReportsCoverage = "reports_coverage"
)

// Relationship direction values for RelatedBusinessServiceData. Phase 2A
// projects only outgoing relationships; the constant is kept narrow to
// keep the wire shape closed.
const (
	RelationshipDirectionOutgoing = "outgoing"
)

// NodeRef is the (kind, id) pair that uniquely identifies a node in a
// projection. Used by Edge.Src and Edge.Dst, and by Projection.Root.
type NodeRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Node is one entity in the projected graph.
//
// Phase 1 fields (Kind, ID, Label) are always populated. Phase 2A adds
// typed per-kind data pointers — exactly one is populated per node and
// matches the Node's Kind. Unused pointers are omitted from the wire
// payload via json:"...,omitempty", so a Node serialises to a compact
// shape carrying only its kind's data block. There is no generic
// Attrs map.
type Node struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`

	// Typed per-kind data. The field name matches the kind so the wire
	// payload is self-describing.
	BusinessService        *BusinessServiceData        `json:"business_service,omitempty"`
	RelatedBusinessService *RelatedBusinessServiceData `json:"related_business_service,omitempty"`
	Capability             *CapabilityData             `json:"capability,omitempty"`
	Process                *ProcessData                `json:"process,omitempty"`
	DecisionSurface        *DecisionSurfaceData        `json:"decision_surface,omitempty"`
	AISystem               *AISystemData               `json:"ai_system,omitempty"`
	AISystemBinding        *AISystemBindingData        `json:"ai_system_binding,omitempty"`
	AuthoritySummary       *AuthoritySummaryData       `json:"authority_summary,omitempty"`
	Coverage               *CoverageData               `json:"coverage,omitempty"`
}

// Edge is one directed link in the projected graph. The semantic
// directionality is encoded in (Src → Dst) per the table above. Label
// is optional and only populated for edge kinds that carry a
// human-readable description (none in Phase 1/2A; reserved for the
// inline edge explanation epic).
type Edge struct {
	Kind  string  `json:"kind"`
	Src   NodeRef `json:"src"`
	Dst   NodeRef `json:"dst"`
	Label string  `json:"label,omitempty"`
}

// Projection is the authority-graph response payload. Nodes are sorted
// by (Kind, ID) ascending; edges are sorted by
// (Kind, Src.Kind, Src.ID, Dst.Kind, Dst.ID) ascending. Both are
// non-nil; an empty depth=0 projection still carries the root in Nodes
// and an empty Edges slice.
type Projection struct {
	Root  NodeRef `json:"root"`
	View  string  `json:"view"`
	Depth int     `json:"depth"`
	Nodes []Node  `json:"nodes"`
	Edges []Edge  `json:"edges"`
}

// ---------------------------------------------------------------------------
// Per-kind typed data (Phase 2A)
// ---------------------------------------------------------------------------
//
// Each struct mirrors the equivalent governance-map DTO field set,
// limited to what the existing governancemap.Map carries. Where the
// governance-map DTO exposes a field the underlying domain type does
// not own (e.g. Process has no ExternalRef on the domain type today),
// the corresponding typed-data field is omitted; the omission is
// documented in the package report.

// ExternalRefData mirrors the governance-map external_ref wire shape
// (httpapi.externalRefResponse). Defined here to keep authoritygraph
// from importing internal/httpapi (which would create a cycle). The
// JSON tags match the existing wire format byte-for-byte.
type ExternalRefData struct {
	SourceSystem  string  `json:"source_system"`
	SourceID      string  `json:"source_id"`
	SourceURL     string  `json:"source_url,omitempty"`
	SourceVersion string  `json:"source_version,omitempty"`
	LastSyncedAt  *string `json:"last_synced_at,omitempty"`
}

// BusinessServiceData carries the typed information for a node of kind
// "business_service". Mirrors the governance-map DTO
// (governanceMapBusinessService) for the fields available on the
// businessservice.BusinessService domain type.
type BusinessServiceData struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Description     string           `json:"description,omitempty"`
	Status          string           `json:"status"`
	Owner           string           `json:"owner,omitempty"`
	ServiceType     string           `json:"service_type,omitempty"`
	RegulatoryScope string           `json:"regulatory_scope,omitempty"`
	ExternalRef     *ExternalRefData `json:"external_ref,omitempty"`
}

// RelatedBusinessServiceData carries the typed information for a node
// of kind "related_business_service". Phase 2A projects only outgoing
// relationships, so Direction is always "outgoing"; the field is kept
// explicit for forward compatibility with bidirectional projections.
type RelatedBusinessServiceData struct {
	ID               string `json:"id"`
	Name             string `json:"name,omitempty"`
	RelationshipType string `json:"relationship_type"`
	Direction        string `json:"direction"`
}

// CapabilityData carries the typed information for a node of kind
// "capability".
type CapabilityData struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Description        string           `json:"description,omitempty"`
	Status             string           `json:"status"`
	Owner              string           `json:"owner,omitempty"`
	ParentCapabilityID string           `json:"parent_capability_id,omitempty"`
	ExternalRef        *ExternalRefData `json:"external_ref,omitempty"`
}

// ProcessData carries the typed information for a node of kind
// "process".
//
// NOTE: process.Process has no ExternalRef field on the domain type
// today, so ProcessData has no external_ref field. If the domain
// gains ExternalRef in a later phase, the typed-data struct will be
// extended additively.
type ProcessData struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Status            string `json:"status"`
	Owner             string `json:"owner,omitempty"`
	BusinessServiceID string `json:"business_service_id"`
}

// DecisionSurfaceData carries the typed information for a node of kind
// "decision_surface". The two binding-id arrays are non-nil (possibly
// empty) and disjoint per the governance-map contract.
//
// NOTE: surface.DecisionSurface has no ExternalRef and no Owner on the
// domain type today; both are omitted from this struct. If the domain
// gains them, the typed-data struct will be extended additively.
type DecisionSurfaceData struct {
	ID                    string   `json:"id"`
	Version               int      `json:"version"`
	Name                  string   `json:"name"`
	Description           string   `json:"description,omitempty"`
	Status                string   `json:"status"`
	ProcessID             string   `json:"process_id"`
	AIBindingIDs          []string `json:"ai_binding_ids"`
	InheritedAIBindingIDs []string `json:"inherited_ai_binding_ids"`
	ProfileCount          int      `json:"profile_count"`
	GrantCount            int      `json:"grant_count"`
	AgentCount            int      `json:"agent_count"`
}

// AISystemData carries the typed information for a node of kind
// "ai_system". ActiveVersion / ActiveVersionLabel / ActiveVersionStatus
// are populated when the AI system has an active version on the
// governance map, and left at the zero value otherwise.
type AISystemData struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	Description         string           `json:"description,omitempty"`
	Status              string           `json:"status"`
	Vendor              string           `json:"vendor,omitempty"`
	SystemType          string           `json:"system_type,omitempty"`
	ActiveVersion       *int             `json:"active_version,omitempty"`
	ActiveVersionLabel  string           `json:"active_version_label,omitempty"`
	ActiveVersionStatus string           `json:"active_version_status,omitempty"`
	ExternalRef         *ExternalRefData `json:"external_ref,omitempty"`
}

// AISystemBindingData carries the typed information for a node of kind
// "ai_system_binding". The four scope-id pointers mirror the
// governance-map binding DTO; ScopeKind / ScopeID / ScopeLabel record
// the most-specific scope (matching the bound_to edge target and the
// node label format). Role / Description mirror the governance-map
// binding DTO fields and are populated when present on the underlying
// aisystem.AISystemBinding domain type.
type AISystemBindingData struct {
	ID                string  `json:"id"`
	AISystemID        string  `json:"ai_system_id"`
	AISystemName      string  `json:"ai_system_name,omitempty"`
	BusinessServiceID *string `json:"business_service_id"`
	CapabilityID      *string `json:"capability_id"`
	ProcessID         *string `json:"process_id"`
	SurfaceID         *string `json:"surface_id"`
	Role              string  `json:"role,omitempty"`
	Description       string  `json:"description,omitempty"`
	ScopeKind         string  `json:"scope_kind"`
	ScopeID           string  `json:"scope_id"`
	ScopeLabel        string  `json:"scope_label"`
}

// AuthoritySummaryData carries the four count fields the governance-map
// AuthoritySummary DTO exposes. The field names match the existing
// governance-map JSON tags so the values are directly comparable in
// the cross-endpoint contract test.
type AuthoritySummaryData struct {
	SurfaceCount       int `json:"surface_count"`
	ActiveProfileCount int `json:"active_profile_count"`
	ActiveGrantCount   int `json:"active_grant_count"`
	ActiveAgentCount   int `json:"active_agent_count"`
}

// CoverageData carries the four count fields the governance-map
// Coverage DTO exposes (Phase 9 explicit naming).
type CoverageData struct {
	SurfaceCount                int `json:"surface_count"`
	SurfacesWithDirectAIBinding int `json:"surfaces_with_direct_ai_binding"`
	SurfacesWithScopedAIBinding int `json:"surfaces_with_scoped_ai_binding"`
	SurfacesWithNoAIBinding     int `json:"surfaces_with_no_ai_binding"`
}
