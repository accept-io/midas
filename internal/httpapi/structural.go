package httpapi

import (
	"context"
	"errors"
	"sort"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// CapabilityReader is the capability repository subset needed for structural reads.
//
// ListByParentCapabilityID was added in Phase 2 to back the children
// read endpoint (Phase 3A). The method returns direct children only —
// the handler must walk the hierarchy itself if recursive descendants
// are ever needed (currently they are not).
type CapabilityReader interface {
	GetByID(ctx context.Context, id string) (*capability.Capability, error)
	List(ctx context.Context) ([]*capability.Capability, error)
	ListByParentCapabilityID(ctx context.Context, parentID string) ([]*capability.Capability, error)
}

// ProcessReader is the process repository subset needed for structural reads.
type ProcessReader interface {
	GetByID(ctx context.Context, id string) (*process.Process, error)
	List(ctx context.Context) ([]*process.Process, error)
}

// ProcessSurfaceReader is the surface repository subset needed for process traversal.
type ProcessSurfaceReader interface {
	ListByProcessID(ctx context.Context, processID string) ([]*surface.DecisionSurface, error)
}

// BusinessServiceReader is the business service repository subset needed for structural reads.
type BusinessServiceReader interface {
	GetByID(ctx context.Context, id string) (*businessservice.BusinessService, error)
	List(ctx context.Context) ([]*businessservice.BusinessService, error)
}

// BusinessServiceRelationshipReader is the BSR repository subset needed for
// the read-side governance-map endpoint introduced in Epic 1, PR 1.
type BusinessServiceRelationshipReader interface {
	ListBySourceBusinessService(ctx context.Context, sourceID string) ([]*businessservice.BusinessServiceRelationship, error)
	ListByTargetBusinessService(ctx context.Context, targetID string) ([]*businessservice.BusinessServiceRelationship, error)
}

// BusinessServiceCapabilityReader is the BSC repository subset needed
// to back the reverse-direction Capability → BusinessService lookup
// (Phase 3B): GET /v1/capabilities/{id}/businessservices. Only the
// capability-side List method is exposed here; the BS-side List is
// already covered by governancemap's reader, which is wired separately.
type BusinessServiceCapabilityReader interface {
	ListByCapabilityID(ctx context.Context, capabilityID string) ([]*businessservicecapability.BusinessServiceCapability, error)
}

// AISystemReader is the AISystem repository subset needed for the
// /v1/aisystems read endpoints (Epic 1, PR 2).
type AISystemReader interface {
	GetByID(ctx context.Context, id string) (*aisystem.AISystem, error)
	List(ctx context.Context) ([]*aisystem.AISystem, error)
}

// AISystemVersionReader is the version repository subset needed for the
// /v1/aisystems/{id}/versions read endpoints.
type AISystemVersionReader interface {
	GetByIDAndVersion(ctx context.Context, aiSystemID string, version int) (*aisystem.AISystemVersion, error)
	ListBySystem(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemVersion, error)
}

// AISystemBindingReader is the binding repository subset needed for the
// /v1/aisystems/{id}/bindings read endpoint and the Capability-scoped
// AI bindings endpoint added in Phase 3C
// (GET /v1/capabilities/{id}/ai-bindings). The same domain reader
// (aisystem.BindingRepository) backs both methods, so wiring is shared
// — no separate option/predicate needed.
type AISystemBindingReader interface {
	ListByAISystem(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, error)
	ListByCapability(ctx context.Context, capabilityID string) ([]*aisystem.AISystemBinding, error)
}

// StructuralService satisfies the structuralService interface by delegating
// to the underlying repository implementations.
type StructuralService struct {
	capabilities     CapabilityReader
	processes        ProcessReader
	surfaces         ProcessSurfaceReader
	businessServices BusinessServiceReader
	bsRelationships  BusinessServiceRelationshipReader
	bsCapabilities   BusinessServiceCapabilityReader
	aiSystems        AISystemReader
	aiVersions       AISystemVersionReader
	aiBindings       AISystemBindingReader
}

// NewStructuralService constructs a StructuralService.
// surfaces may be nil; traversal endpoints will return an empty slice if nil.
func NewStructuralService(caps CapabilityReader, procs ProcessReader, surfs ProcessSurfaceReader) *StructuralService {
	return &StructuralService{
		capabilities: caps,
		processes:    procs,
		surfaces:     surfs,
	}
}

// WithBusinessServices attaches a BusinessServiceReader to this StructuralService,
// enabling the /v1/businessservices endpoints. Returns the receiver for chaining.
func (s *StructuralService) WithBusinessServices(bs BusinessServiceReader) *StructuralService {
	s.businessServices = bs
	return s
}

// WithBusinessServiceRelationships attaches a BSR reader, enabling the
// /v1/businessservices/{id}/relationships endpoint (Epic 1, PR 1).
// Returns the receiver for chaining.
func (s *StructuralService) WithBusinessServiceRelationships(r BusinessServiceRelationshipReader) *StructuralService {
	s.bsRelationships = r
	return s
}

// WithBusinessServiceCapabilities attaches a BSC reader, enabling the
// /v1/capabilities/{id}/businessservices endpoint (Phase 3B). The
// reader supplies BSC junction rows for a given capability; the
// service then dereferences each row's BusinessServiceID via the
// already-wired BS reader. Returns the receiver for chaining.
func (s *StructuralService) WithBusinessServiceCapabilities(r BusinessServiceCapabilityReader) *StructuralService {
	s.bsCapabilities = r
	return s
}

// WithAISystems attaches the three AI System Registration readers
// (Epic 1, PR 2), enabling the /v1/aisystems/* endpoints. Any of the
// three readers may be nil — the handler returns 501 when the relevant
// reader is missing. Returns the receiver for chaining.
func (s *StructuralService) WithAISystems(systems AISystemReader, versions AISystemVersionReader, bindings AISystemBindingReader) *StructuralService {
	s.aiSystems = systems
	s.aiVersions = versions
	s.aiBindings = bindings
	return s
}

// GetCapability returns a capability by ID. Returns nil, nil when not found.
func (s *StructuralService) GetCapability(ctx context.Context, id string) (*capability.Capability, error) {
	return s.capabilities.GetByID(ctx, id)
}

// ListCapabilities returns all capabilities.
func (s *StructuralService) ListCapabilities(ctx context.Context) ([]*capability.Capability, error) {
	return s.capabilities.List(ctx)
}

// ListChildCapabilities returns the direct children of parentID.
//
// Returns:
//   - (nil, false, nil) when the parent does not exist (handler maps to 404)
//   - (children, true, nil) on success — children may be empty (handler maps to 200 with [])
//   - (_, _, err) on repository error
//
// Recursive descendants are NOT returned; the underlying repository
// method is intentionally direct-children-only. A future caller that
// needs the full subtree must walk the hierarchy itself.
func (s *StructuralService) ListChildCapabilities(ctx context.Context, parentID string) ([]*capability.Capability, bool, error) {
	parent, err := s.capabilities.GetByID(ctx, parentID)
	if err != nil {
		return nil, false, err
	}
	if parent == nil {
		return nil, false, nil
	}
	children, err := s.capabilities.ListByParentCapabilityID(ctx, parentID)
	if err != nil {
		return nil, true, err
	}
	if children == nil {
		children = []*capability.Capability{}
	}
	return children, true, nil
}

// GetProcess returns a process by ID. Returns nil, nil when not found.
func (s *StructuralService) GetProcess(ctx context.Context, id string) (*process.Process, error) {
	return s.processes.GetByID(ctx, id)
}

// ListProcesses returns all processes.
func (s *StructuralService) ListProcesses(ctx context.Context) ([]*process.Process, error) {
	return s.processes.List(ctx)
}

// GetBusinessService returns a business service by ID. Returns nil, nil when not found
// or when no BusinessServiceReader has been configured.
func (s *StructuralService) GetBusinessService(ctx context.Context, id string) (*businessservice.BusinessService, error) {
	if s.businessServices == nil {
		return nil, nil
	}
	return s.businessServices.GetByID(ctx, id)
}

// ListBusinessServices returns all business services. Returns an empty slice when
// no BusinessServiceReader has been configured.
func (s *StructuralService) ListBusinessServices(ctx context.Context) ([]*businessservice.BusinessService, error) {
	if s.businessServices == nil {
		return []*businessservice.BusinessService{}, nil
	}
	return s.businessServices.List(ctx)
}

// ListSurfacesByProcess returns surfaces belonging to the given process.
// Returns (nil, false, nil) when the process does not exist.
// Returns (surfs, true, nil) including empty slice when found.
func (s *StructuralService) ListSurfacesByProcess(ctx context.Context, processID string) ([]*surface.DecisionSurface, bool, error) {
	proc, err := s.processes.GetByID(ctx, processID)
	if err != nil {
		return nil, false, err
	}
	if proc == nil {
		return nil, false, nil
	}
	if s.surfaces == nil {
		return []*surface.DecisionSurface{}, true, nil
	}
	surfs, err := s.surfaces.ListByProcessID(ctx, processID)
	if err != nil {
		return nil, true, err
	}
	if surfs == nil {
		surfs = []*surface.DecisionSurface{}
	}
	return surfs, true, nil
}

// ListRelationshipsForBusinessService returns the outgoing and incoming BSR
// rows for the given business service, partitioned by direction.
//
// Returns:
//   - found = false when the queried business_service_id does not exist
//     (so the handler can map to 404)
//   - empty slices (never nil) for outgoing / incoming when no rows match
//   - error wrapping the first repo error encountered
//
// When the BSR reader has not been configured, returns
// ([]{}, []{}, true, nil) — the absent reader is treated as "no
// relationships exist for any service" rather than a separate error path.
// The /v1/businessservices/{id}/relationships handler decides whether to
// return 501 based on whether the reader is configured at all (via
// HasBusinessServiceRelationships).
func (s *StructuralService) ListRelationshipsForBusinessService(ctx context.Context, businessServiceID string) (outgoing, incoming []*businessservice.BusinessServiceRelationship, found bool, err error) {
	bs, err := s.GetBusinessService(ctx, businessServiceID)
	if err != nil {
		return nil, nil, false, err
	}
	if bs == nil {
		return nil, nil, false, nil
	}
	if s.bsRelationships == nil {
		return []*businessservice.BusinessServiceRelationship{}, []*businessservice.BusinessServiceRelationship{}, true, nil
	}
	outgoing, err = s.bsRelationships.ListBySourceBusinessService(ctx, businessServiceID)
	if err != nil {
		return nil, nil, true, err
	}
	if outgoing == nil {
		outgoing = []*businessservice.BusinessServiceRelationship{}
	}
	incoming, err = s.bsRelationships.ListByTargetBusinessService(ctx, businessServiceID)
	if err != nil {
		return nil, nil, true, err
	}
	if incoming == nil {
		incoming = []*businessservice.BusinessServiceRelationship{}
	}
	return outgoing, incoming, true, nil
}

// HasBusinessServiceRelationships reports whether the BSR reader has been
// wired. The /v1/businessservices/{id}/relationships handler uses this to
// distinguish "endpoint not configured" (501) from "no relationships exist"
// (200 with empty arrays).
func (s *StructuralService) HasBusinessServiceRelationships() bool {
	return s.bsRelationships != nil
}

// HasBusinessServiceCapabilities reports whether both the BSC reader
// AND the BS reader have been wired. The
// /v1/capabilities/{id}/businessservices handler needs both to
// resolve BSC junction rows into full BusinessService responses;
// either one missing produces a 501.
func (s *StructuralService) HasBusinessServiceCapabilities() bool {
	return s.bsCapabilities != nil && s.businessServices != nil
}

// ListBusinessServicesByCapability returns the Business Services
// linked to the given capability via the business_service_capabilities
// junction (Phase 3B).
//
// Returns:
//   - (nil, false, nil) when the capability does not exist (handler maps to 404)
//   - (services, true, nil) on success — services may be empty when the
//     capability has no BSC links (handler maps to 200 with [])
//   - (_, _, err) on repository error
//
// Dangling-link convention (matches governancemap.read_service): a BSC
// row whose business_service_id resolves to nil (deleted BS) is
// silently skipped rather than surfaced as a 500. The control plane's
// BSC junction has FK=ON DELETE RESTRICT in postgres so this can only
// occur in a degraded path; skipping protects the read API's shape.
//
// Results are ordered by business_service_id ascending (delivered by
// the underlying BSCRepo iteration plus a final sort here for
// determinism — memory iteration is non-deterministic). The caller's
// chosen sort key matches what GET /v1/businessservices already uses.
func (s *StructuralService) ListBusinessServicesByCapability(ctx context.Context, capabilityID string) ([]*businessservice.BusinessService, bool, error) {
	if s.capabilities == nil {
		return nil, false, nil
	}
	cap, err := s.capabilities.GetByID(ctx, capabilityID)
	if err != nil {
		return nil, false, err
	}
	if cap == nil {
		return nil, false, nil
	}
	if s.bsCapabilities == nil || s.businessServices == nil {
		// Both readers must be wired to dereference junction rows; the
		// handler enforces 501 via HasBusinessServiceCapabilities
		// before reaching this method, but be defensive.
		return []*businessservice.BusinessService{}, true, nil
	}
	links, err := s.bsCapabilities.ListByCapabilityID(ctx, capabilityID)
	if err != nil {
		return nil, true, err
	}
	out := make([]*businessservice.BusinessService, 0, len(links))
	for _, link := range links {
		bs, err := s.businessServices.GetByID(ctx, link.BusinessServiceID)
		if err != nil {
			return nil, true, err
		}
		if bs == nil {
			// Dangling BSC row — see method doc. Skip silently.
			continue
		}
		out = append(out, bs)
	}
	// Stable, ID-ascending order so the wire shape is deterministic
	// across memory and postgres backends.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, true, nil
}

// ---------------------------------------------------------------------------
// AI System Registration (Epic 1, PR 2)
// ---------------------------------------------------------------------------

// HasAISystems reports whether the AISystem reader has been wired. The
// /v1/aisystems list and detail handlers use this to distinguish "endpoint
// not configured" (501) from "no systems exist" (200 with empty array).
func (s *StructuralService) HasAISystems() bool {
	return s.aiSystems != nil
}

// HasAISystemVersions reports whether the version reader has been wired.
// Used by the /v1/aisystems/{id}/versions handler to return 501 when the
// reader is absent even if the parent system reader is configured.
func (s *StructuralService) HasAISystemVersions() bool {
	return s.aiVersions != nil
}

// HasAISystemBindings reports whether the binding reader has been wired.
func (s *StructuralService) HasAISystemBindings() bool {
	return s.aiBindings != nil
}

// GetAISystem returns the AI system with the given ID. Returns
// (nil, nil) when not found OR when the reader is not configured —
// the handler distinguishes the two via HasAISystems.
//
// The domain repository signals not-found via aisystem.ErrAISystemNotFound;
// this wrapper translates that to (nil, nil) so the handler can branch on
// the value rather than the error type.
func (s *StructuralService) GetAISystem(ctx context.Context, id string) (*aisystem.AISystem, error) {
	if s.aiSystems == nil {
		return nil, nil
	}
	sys, err := s.aiSystems.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, aisystem.ErrAISystemNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return sys, nil
}

// ListAISystems returns all AI systems. Returns an empty slice (never
// nil) when the reader is not configured, matching the
// ListBusinessServices posture.
func (s *StructuralService) ListAISystems(ctx context.Context) ([]*aisystem.AISystem, error) {
	if s.aiSystems == nil {
		return []*aisystem.AISystem{}, nil
	}
	out, err := s.aiSystems.List(ctx)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []*aisystem.AISystem{}
	}
	return out, nil
}

// GetAISystemVersion returns the (ai_system_id, version) tuple.
//
// Returns:
//   - (nil, false, nil) when the parent AI system does not exist
//     (handler maps to 404 on the system).
//   - (nil, true, nil) when the parent exists but the requested version
//     does not (handler maps to 404 on the version).
//   - (ver, true, nil) on success.
//
// When the version reader is not configured, returns (nil, true, nil)
// for any version once the parent is confirmed to exist — the handler
// uses HasAISystemVersions to return 501 when appropriate before
// reaching this method.
func (s *StructuralService) GetAISystemVersion(ctx context.Context, aiSystemID string, version int) (*aisystem.AISystemVersion, bool, error) {
	sys, err := s.GetAISystem(ctx, aiSystemID)
	if err != nil {
		return nil, false, err
	}
	if sys == nil {
		return nil, false, nil
	}
	if s.aiVersions == nil {
		return nil, true, nil
	}
	ver, err := s.aiVersions.GetByIDAndVersion(ctx, aiSystemID, version)
	if err != nil {
		if errors.Is(err, aisystem.ErrAISystemVersionNotFound) {
			return nil, true, nil
		}
		return nil, true, err
	}
	return ver, true, nil
}

// ListAISystemVersions returns all versions for the given AI system,
// ordered by version DESC (latest first).
//
// Returns:
//   - (nil, false, nil) when the parent AI system does not exist
//     (handler maps to 404).
//   - (versions, true, nil) including empty slice when found.
//
// When the version reader is not configured, returns
// ([]{}, true, nil) once the parent is confirmed — the handler uses
// HasAISystemVersions for the 501 branch.
func (s *StructuralService) ListAISystemVersions(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemVersion, bool, error) {
	sys, err := s.GetAISystem(ctx, aiSystemID)
	if err != nil {
		return nil, false, err
	}
	if sys == nil {
		return nil, false, nil
	}
	if s.aiVersions == nil {
		return []*aisystem.AISystemVersion{}, true, nil
	}
	versions, err := s.aiVersions.ListBySystem(ctx, aiSystemID)
	if err != nil {
		return nil, true, err
	}
	if versions == nil {
		versions = []*aisystem.AISystemVersion{}
	}
	return versions, true, nil
}

// ListAISystemBindings returns all bindings for the given AI system.
// Returns (nil, false, nil) when the parent AI system does not exist;
// (bindings, true, nil) including empty slice when found.
func (s *StructuralService) ListAISystemBindings(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, bool, error) {
	sys, err := s.GetAISystem(ctx, aiSystemID)
	if err != nil {
		return nil, false, err
	}
	if sys == nil {
		return nil, false, nil
	}
	if s.aiBindings == nil {
		return []*aisystem.AISystemBinding{}, true, nil
	}
	bindings, err := s.aiBindings.ListByAISystem(ctx, aiSystemID)
	if err != nil {
		return nil, true, err
	}
	if bindings == nil {
		bindings = []*aisystem.AISystemBinding{}
	}
	return bindings, true, nil
}

// ListAIBindingsByCapability returns the AI System bindings whose
// capability_id matches the given capability (Phase 3C — backs
// GET /v1/capabilities/{id}/ai-bindings).
//
// Returns:
//   - (nil, false, nil) when the capability does not exist (handler maps to 404)
//   - (bindings, true, nil) on success — bindings may be empty
//     (handler maps to 200 with [])
//   - (_, _, err) on repository error
//
// Direct Capability scope only: results are exactly the bindings that
// the BindingRepository.ListByCapability returns. Bindings that
// reference the capability indirectly (via a Process whose BS the
// capability is linked to, etc.) are NOT returned — that broader
// inference is out of scope for this read endpoint.
//
// Bindings carry their own multi-context fields (BS, Cap, Process,
// Surface IDs) so there is no dereferencing to do; the dangling-link
// concern that applies to BSC junctions does not apply here.
//
// Ordering: the underlying repository decides. Memory's
// ListByCapability iterates the map (non-deterministic in Go); the
// service sorts by binding ID ascending here so the wire shape is
// deterministic across memory and postgres backends, matching the
// posture established for ListBusinessServicesByCapability (Phase 3B).
func (s *StructuralService) ListAIBindingsByCapability(ctx context.Context, capabilityID string) ([]*aisystem.AISystemBinding, bool, error) {
	if s.capabilities == nil {
		return nil, false, nil
	}
	cap, err := s.capabilities.GetByID(ctx, capabilityID)
	if err != nil {
		return nil, false, err
	}
	if cap == nil {
		return nil, false, nil
	}
	if s.aiBindings == nil {
		// Reader not configured → handler converts to 501 via
		// HasAISystemBindings before reaching this method, but be
		// defensive: return an empty slice rather than nil so the
		// envelope shape is honest if the handler is bypassed.
		return []*aisystem.AISystemBinding{}, true, nil
	}
	bindings, err := s.aiBindings.ListByCapability(ctx, capabilityID)
	if err != nil {
		return nil, true, err
	}
	if bindings == nil {
		bindings = []*aisystem.AISystemBinding{}
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	return bindings, true, nil
}

// ---------------------------------------------------------------------------
// Explicit-mode validation service
// ---------------------------------------------------------------------------

// ExplicitSurfaceReader is the surface repository subset required for
// explicit-mode structural validation. Satisfied by existing repo implementations.
type ExplicitSurfaceReader interface {
	FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error)
}

// ExplicitValidationService provides process and surface existence lookups
// for explicit-mode evaluate requests. It is intentionally narrow — only
// what PR5 explicit-mode validation needs.
type ExplicitValidationService struct {
	processes ProcessReader
	surfaces  ExplicitSurfaceReader
}

// NewExplicitValidationService constructs an ExplicitValidationService.
func NewExplicitValidationService(procs ProcessReader, surfs ExplicitSurfaceReader) *ExplicitValidationService {
	return &ExplicitValidationService{processes: procs, surfaces: surfs}
}

// GetProcess returns a process by ID. Returns nil, nil when not found.
func (s *ExplicitValidationService) GetProcess(ctx context.Context, id string) (*process.Process, error) {
	return s.processes.GetByID(ctx, id)
}

// FindLatestSurface returns the latest version of a surface by ID.
// Returns nil, nil when not found.
func (s *ExplicitValidationService) FindLatestSurface(ctx context.Context, id string) (*surface.DecisionSurface, error) {
	return s.surfaces.FindLatestByID(ctx, id)
}
