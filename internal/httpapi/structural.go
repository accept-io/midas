package httpapi

import (
	"context"

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

// StructuralService satisfies the structuralService interface by delegating
// to the underlying repository implementations.
type StructuralService struct {
	capabilities     CapabilityReader
	processes        ProcessReader
	surfaces         ProcessSurfaceReader
	businessServices BusinessServiceReader
	bsRelationships  BusinessServiceRelationshipReader
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
