package httpapi

import (
	"context"
	"errors"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// CapabilityReader is the capability repository subset needed for structural reads.
type CapabilityReader interface {
	GetByID(ctx context.Context, id string) (*capability.Capability, error)
	List(ctx context.Context) ([]*capability.Capability, error)
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
// /v1/aisystems/{id}/bindings read endpoint.
type AISystemBindingReader interface {
	ListByAISystem(ctx context.Context, aiSystemID string) ([]*aisystem.AISystemBinding, error)
}

// StructuralService satisfies the structuralService interface by delegating
// to the underlying repository implementations.
type StructuralService struct {
	capabilities     CapabilityReader
	processes        ProcessReader
	surfaces         ProcessSurfaceReader
	businessServices BusinessServiceReader
	bsRelationships  BusinessServiceRelationshipReader
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
