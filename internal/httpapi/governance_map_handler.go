package httpapi

// HTTP handler for GET /v1/businessservices/{id}/governance-map
// (Epic 1, PR 4). The handler is intentionally thin: parse the path,
// call the read service, marshal the Map type to a wire-shape struct.
// All aggregation logic lives in internal/governancemap.

import (
	"context"
	"errors"
	"net/http"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// governanceMapReadService is the narrow interface the handler depends
// on. *governancemap.ReadService satisfies it. Defined here (not in
// the governancemap package) so the handler can swap in test doubles
// without dragging the full package surface.
type governanceMapReadService interface {
	GetGovernanceMap(ctx context.Context, businessServiceID string) (*governancemap.Map, error)
	HasAllReaders() bool
}

// WithGovernanceMap attaches a governance map read service to this
// Server. Returns the receiver for chaining. Pass nil to disable the
// endpoint (which then returns 501 with "governance map read service
// not configured").
func (s *Server) WithGovernanceMap(svc governanceMapReadService) *Server {
	s.governanceMap = svc
	return s
}

// handleGetBusinessServiceGovernanceMap serves
// GET /v1/businessservices/{id}/governance-map.
//
// Status codes:
//   - 200 OK on success
//   - 404 Not Found when the business service does not exist
//   - 500 Internal Server Error on read service failure
//   - 501 Not Implemented when the read service is not configured
//     (mirrors PR 1's `/relationships` and PR 2's AI system endpoints)
//
// Auth: enforced upstream by the same middleware as the parent
// `/v1/businessservices/{id}` handler (requireAuth + requireRole).
func (s *Server) handleGetBusinessServiceGovernanceMap(w http.ResponseWriter, r *http.Request, businessServiceID string) {
	if s.governanceMap == nil || !s.governanceMap.HasAllReaders() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "governance map read service not configured",
		})
		return
	}
	gmap, err := s.governanceMap.GetGovernanceMap(r.Context(), businessServiceID)
	if err != nil {
		if errors.Is(err, governancemap.ErrServiceNotConfigured) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{
				"error": "governance map read service not configured",
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if gmap == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "business service not found"})
		return
	}
	writeJSON(w, http.StatusOK, toGovernanceMapResponse(gmap))
}

// ---------------------------------------------------------------------------
// Wire-format response types
// ---------------------------------------------------------------------------
//
// Each response struct mirrors a single Map node. The shape is described
// in api/openapi/v1.yaml (component schemas under GovernanceMap*).
//
// Wire-shape rules (per PR 4 brief):
//
//   - Arrays are always arrays ([]), never null.
//   - Optional scalar fields use pointers / `omitempty` so absent
//     values render predictably.
//   - external_ref uses the PR 3 `externalRefResponse` helper — appears
//     as `null` when absent.
//   - recent_decisions is OMITTED entirely from the wire response
//     (no key, no null) — Step 0.5 deferral; PR 8 will revisit.

type governanceMapResponse struct {
	BusinessService  governanceMapBusinessService  `json:"business_service"`
	Relationships    governanceMapRelationships    `json:"relationships"`
	Capabilities     []governanceMapCapability     `json:"capabilities"`
	Processes        []governanceMapProcess        `json:"processes"`
	Surfaces         []governanceMapSurface        `json:"surfaces"`
	AISystems        []governanceMapAISystem       `json:"ai_systems"`
	AuthoritySummary governanceMapAuthoritySummary `json:"authority_summary"`
	Coverage         governanceMapCoverage         `json:"coverage"`
	// recent_decisions intentionally omitted (Step 0.5 deferral —
	// PR 8 will populate when envelope substrate lands).
}

type governanceMapBusinessService struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Description     string               `json:"description,omitempty"`
	ServiceType     string               `json:"service_type"`
	RegulatoryScope string               `json:"regulatory_scope,omitempty"`
	Status          string               `json:"status"`
	OwnerID         string               `json:"owner_id,omitempty"`
	ExternalRef     *externalRefResponse `json:"external_ref"`
}

type governanceMapRelationships struct {
	Outgoing []governanceMapRelationship `json:"outgoing"`
	Incoming []governanceMapRelationship `json:"incoming"`
}

type governanceMapRelationship struct {
	ID                      string `json:"id"`
	SourceBusinessServiceID string `json:"source_business_service_id"`
	TargetBusinessServiceID string `json:"target_business_service_id"`
	// OtherName is the opposing service's name (target name for
	// outgoing, source name for incoming). Empty when the opposing
	// service cannot be resolved.
	OtherName        string `json:"other_name,omitempty"`
	RelationshipType string `json:"relationship_type"`
	Description      string `json:"description,omitempty"`
}

type governanceMapCapability struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

type governanceMapProcess struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	BusinessServiceID string `json:"business_service_id"`
	Status            string `json:"status"`
}

type governanceMapSurface struct {
	ID           string   `json:"id"`
	Version      int      `json:"version"`
	Name         string   `json:"name"`
	ProcessID    string   `json:"process_id"`
	Status       string   `json:"status"`
	AIBindings   []string `json:"ai_bindings"`
	ProfileCount int      `json:"profile_count"`
	GrantCount   int      `json:"grant_count"`
	AgentCount   int      `json:"agent_count"`
}

type governanceMapAISystem struct {
	ID            string                         `json:"id"`
	Name          string                         `json:"name"`
	Vendor        string                         `json:"vendor,omitempty"`
	SystemType    string                         `json:"system_type,omitempty"`
	Status        string                         `json:"status"`
	ExternalRef   *externalRefResponse           `json:"external_ref"`
	ActiveVersion *governanceMapAISystemVersion  `json:"active_version"`
	Bindings      []governanceMapAISystemBinding `json:"bindings"`
}

type governanceMapAISystemVersion struct {
	AISystemID   string `json:"ai_system_id"`
	Version      int    `json:"version"`
	ReleaseLabel string `json:"release_label,omitempty"`
	Status       string `json:"status"`
}

type governanceMapAISystemBinding struct {
	ID                string  `json:"id"`
	AISystemID        string  `json:"ai_system_id"`
	AISystemVersion   *int    `json:"ai_system_version"`
	BusinessServiceID *string `json:"business_service_id"`
	CapabilityID      *string `json:"capability_id"`
	ProcessID         *string `json:"process_id"`
	SurfaceID         *string `json:"surface_id"`
	Role              string  `json:"role,omitempty"`
	Description       string  `json:"description,omitempty"`
}

type governanceMapAuthoritySummary struct {
	SurfaceCount       int `json:"surface_count"`
	ActiveProfileCount int `json:"active_profile_count"`
	ActiveGrantCount   int `json:"active_grant_count"`
	ActiveAgentCount   int `json:"active_agent_count"`
}

type governanceMapCoverage struct {
	SurfaceCount             int `json:"surface_count"`
	SurfacesWithAIBinding    int `json:"surfaces_with_ai_binding"`
	SurfacesWithoutAIBinding int `json:"surfaces_without_ai_binding"`
}

// ---------------------------------------------------------------------------
// Map → response mapping
// ---------------------------------------------------------------------------

func toGovernanceMapResponse(m *governancemap.Map) governanceMapResponse {
	resp := governanceMapResponse{
		BusinessService: toGovernanceMapBSResponse(m.BusinessService.BusinessService),
		Relationships: governanceMapRelationships{
			Outgoing: make([]governanceMapRelationship, 0, len(m.Relationships.Outgoing)),
			Incoming: make([]governanceMapRelationship, 0, len(m.Relationships.Incoming)),
		},
		Capabilities: make([]governanceMapCapability, 0, len(m.Capabilities)),
		Processes:    make([]governanceMapProcess, 0, len(m.Processes)),
		Surfaces:     make([]governanceMapSurface, 0, len(m.Surfaces)),
		AISystems:    make([]governanceMapAISystem, 0, len(m.AISystems)),
		AuthoritySummary: governanceMapAuthoritySummary{
			SurfaceCount:       m.AuthoritySummary.SurfaceCount,
			ActiveProfileCount: m.AuthoritySummary.ActiveProfileCount,
			ActiveGrantCount:   m.AuthoritySummary.ActiveGrantCount,
			ActiveAgentCount:   m.AuthoritySummary.ActiveAgentCount,
		},
		Coverage: governanceMapCoverage{
			SurfaceCount:             m.Coverage.SurfaceCount,
			SurfacesWithAIBinding:    m.Coverage.SurfacesWithAIBinding,
			SurfacesWithoutAIBinding: m.Coverage.SurfacesWithoutAIBinding,
		},
	}
	for _, rn := range m.Relationships.Outgoing {
		resp.Relationships.Outgoing = append(resp.Relationships.Outgoing, toGovernanceMapRelationship(rn))
	}
	for _, rn := range m.Relationships.Incoming {
		resp.Relationships.Incoming = append(resp.Relationships.Incoming, toGovernanceMapRelationship(rn))
	}
	for _, cn := range m.Capabilities {
		resp.Capabilities = append(resp.Capabilities, toGovernanceMapCapability(cn.Capability))
	}
	for _, pn := range m.Processes {
		resp.Processes = append(resp.Processes, toGovernanceMapProcess(pn.Process))
	}
	for _, sn := range m.Surfaces {
		resp.Surfaces = append(resp.Surfaces, toGovernanceMapSurface(sn))
	}
	for _, ai := range m.AISystems {
		resp.AISystems = append(resp.AISystems, toGovernanceMapAISystem(ai))
	}
	return resp
}

func toGovernanceMapBSResponse(bs *businessservice.BusinessService) governanceMapBusinessService {
	return governanceMapBusinessService{
		ID:              bs.ID,
		Name:            bs.Name,
		Description:     bs.Description,
		ServiceType:     string(bs.ServiceType),
		RegulatoryScope: bs.RegulatoryScope,
		Status:          bs.Status,
		OwnerID:         bs.OwnerID,
		ExternalRef:     toExternalRefResponse(bs.ExternalRef),
	}
}

func toGovernanceMapRelationship(rn *governancemap.RelationshipNode) governanceMapRelationship {
	rel := rn.Relationship
	return governanceMapRelationship{
		ID:                      rel.ID,
		SourceBusinessServiceID: rel.SourceBusinessService,
		TargetBusinessServiceID: rel.TargetBusinessService,
		OtherName:               rn.OtherName,
		RelationshipType:        rel.RelationshipType,
		Description:             rel.Description,
	}
}

func toGovernanceMapCapability(c *capability.Capability) governanceMapCapability {
	return governanceMapCapability{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Status:      c.Status,
	}
}

func toGovernanceMapProcess(p *process.Process) governanceMapProcess {
	return governanceMapProcess{
		ID:                p.ID,
		Name:              p.Name,
		BusinessServiceID: p.BusinessServiceID,
		Status:            p.Status,
	}
}

func toGovernanceMapSurface(sn *governancemap.SurfaceNode) governanceMapSurface {
	bindings := sn.AIBindingIDs
	if bindings == nil {
		bindings = []string{}
	}
	s := sn.Surface
	return governanceMapSurface{
		ID:           s.ID,
		Version:      s.Version,
		Name:         s.Name,
		ProcessID:    s.ProcessID,
		Status:       string(s.Status),
		AIBindings:   bindings,
		ProfileCount: sn.ProfileCount,
		GrantCount:   sn.GrantCount,
		AgentCount:   sn.AgentCount,
	}
}

func toGovernanceMapAISystem(ai *governancemap.AISystemNode) governanceMapAISystem {
	out := governanceMapAISystem{
		ID:          ai.System.ID,
		Name:        ai.System.Name,
		Vendor:      ai.System.Vendor,
		SystemType:  ai.System.SystemType,
		Status:      ai.System.Status,
		ExternalRef: toExternalRefResponse(ai.System.ExternalRef),
		Bindings:    make([]governanceMapAISystemBinding, 0, len(ai.Bindings)),
	}
	if ai.ActiveVersion != nil {
		out.ActiveVersion = &governanceMapAISystemVersion{
			AISystemID:   ai.ActiveVersion.AISystemID,
			Version:      ai.ActiveVersion.Version,
			ReleaseLabel: ai.ActiveVersion.ReleaseLabel,
			Status:       ai.ActiveVersion.Status,
		}
	}
	for _, b := range ai.Bindings {
		out.Bindings = append(out.Bindings, toGovernanceMapBinding(b))
	}
	return out
}

func toGovernanceMapBinding(b *aisystem.AISystemBinding) governanceMapAISystemBinding {
	var version *int
	if b.AISystemVersion != nil {
		v := *b.AISystemVersion
		version = &v
	}
	return governanceMapAISystemBinding{
		ID:                b.ID,
		AISystemID:        b.AISystemID,
		AISystemVersion:   version,
		BusinessServiceID: nullableStringPtr(b.BusinessServiceID),
		CapabilityID:      nullableStringPtr(b.CapabilityID),
		ProcessID:         nullableStringPtr(b.ProcessID),
		SurfaceID:         nullableStringPtr(b.SurfaceID),
		Role:              b.Role,
		Description:       b.Description,
	}
}

// Compile-time touches to keep the unused-imports linter quiet when
// the response mapping evolves; remove if the imports become directly
// referenced.
var (
	_ = authority.ProfileStatusActive
	_ = surface.SurfaceStatusActive
)
