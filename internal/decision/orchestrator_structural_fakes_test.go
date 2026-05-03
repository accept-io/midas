package decision_test

// Minimal fake repositories for the v1 service-led structural chain.
// These exist so that orchestrator tests using fakeStore / spyStore can
// satisfy the structural-resolution step added in ADR-0001 implementation.
//
// Each fake implements only the methods that the orchestrator's
// resolveStructure helper actually calls. Other interface methods exist
// as no-op stubs purely to satisfy the interface contract.

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
)

// ---------------------------------------------------------------------------
// fakeProcessRepo
// ---------------------------------------------------------------------------

type fakeProcessRepo struct {
	procs map[string]*process.Process
}

func (r *fakeProcessRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.procs[id]
	return ok, nil
}

func (r *fakeProcessRepo) GetByID(_ context.Context, id string) (*process.Process, error) {
	if r.procs == nil {
		return nil, nil
	}
	return r.procs[id], nil
}

func (r *fakeProcessRepo) List(_ context.Context) ([]*process.Process, error) {
	out := make([]*process.Process, 0, len(r.procs))
	for _, p := range r.procs {
		out = append(out, p)
	}
	return out, nil
}

// ListByBusinessService satisfies the process.ProcessRepository contract
// added in Epic 1 PR 4 to support the governance map read service. The
// fake filters its in-memory procs map by BusinessServiceID; orchestrator
// tests do not exercise this method directly, but the interface must
// be satisfied for fakeStore / spyStore to compile.
func (r *fakeProcessRepo) ListByBusinessService(_ context.Context, businessServiceID string) ([]*process.Process, error) {
	out := make([]*process.Process, 0)
	for _, p := range r.procs {
		if p.BusinessServiceID == businessServiceID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakeProcessRepo) Create(_ context.Context, p *process.Process) error {
	if r.procs == nil {
		r.procs = map[string]*process.Process{}
	}
	r.procs[p.ID] = p
	return nil
}

func (r *fakeProcessRepo) Update(_ context.Context, p *process.Process) error {
	if r.procs == nil {
		r.procs = map[string]*process.Process{}
	}
	r.procs[p.ID] = p
	return nil
}

// ---------------------------------------------------------------------------
// fakeBusinessServiceRepo
// ---------------------------------------------------------------------------

type fakeBusinessServiceRepo struct {
	services map[string]*businessservice.BusinessService
}

func (r *fakeBusinessServiceRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.services[id]
	return ok, nil
}

func (r *fakeBusinessServiceRepo) GetByID(_ context.Context, id string) (*businessservice.BusinessService, error) {
	if r.services == nil {
		return nil, nil
	}
	return r.services[id], nil
}

func (r *fakeBusinessServiceRepo) List(_ context.Context) ([]*businessservice.BusinessService, error) {
	out := make([]*businessservice.BusinessService, 0, len(r.services))
	for _, s := range r.services {
		out = append(out, s)
	}
	return out, nil
}

func (r *fakeBusinessServiceRepo) Create(_ context.Context, s *businessservice.BusinessService) error {
	if r.services == nil {
		r.services = map[string]*businessservice.BusinessService{}
	}
	r.services[s.ID] = s
	return nil
}

func (r *fakeBusinessServiceRepo) Update(_ context.Context, s *businessservice.BusinessService) error {
	if r.services == nil {
		r.services = map[string]*businessservice.BusinessService{}
	}
	r.services[s.ID] = s
	return nil
}

// ---------------------------------------------------------------------------
// fakeBSCRepo
// ---------------------------------------------------------------------------

type fakeBSCRepo struct {
	// links keyed by businessServiceID; each entry is the full list of
	// (BusinessServiceID, CapabilityID) rows for that BS.
	links map[string][]*businessservicecapability.BusinessServiceCapability
}

func (r *fakeBSCRepo) Create(_ context.Context, bsc *businessservicecapability.BusinessServiceCapability) error {
	if r.links == nil {
		r.links = map[string][]*businessservicecapability.BusinessServiceCapability{}
	}
	r.links[bsc.BusinessServiceID] = append(r.links[bsc.BusinessServiceID], bsc)
	return nil
}

func (r *fakeBSCRepo) Exists(_ context.Context, businessServiceID, capabilityID string) (bool, error) {
	for _, l := range r.links[businessServiceID] {
		if l.CapabilityID == capabilityID {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeBSCRepo) ListByBusinessServiceID(_ context.Context, businessServiceID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	if r.links == nil {
		return nil, nil
	}
	return append([]*businessservicecapability.BusinessServiceCapability(nil), r.links[businessServiceID]...), nil
}

func (r *fakeBSCRepo) ListByCapabilityID(_ context.Context, capabilityID string) ([]*businessservicecapability.BusinessServiceCapability, error) {
	var out []*businessservicecapability.BusinessServiceCapability
	for _, list := range r.links {
		for _, l := range list {
			if l.CapabilityID == capabilityID {
				out = append(out, l)
			}
		}
	}
	return out, nil
}

func (r *fakeBSCRepo) Delete(_ context.Context, businessServiceID, capabilityID string) error {
	list := r.links[businessServiceID]
	for i, l := range list {
		if l.CapabilityID == capabilityID {
			r.links[businessServiceID] = append(list[:i], list[i+1:]...)
			return nil
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// fakeCapabilityRepo
// ---------------------------------------------------------------------------

type fakeCapabilityRepo struct {
	caps map[string]*capability.Capability
}

func (r *fakeCapabilityRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.caps[id]
	return ok, nil
}

func (r *fakeCapabilityRepo) GetByID(_ context.Context, id string) (*capability.Capability, error) {
	if r.caps == nil {
		return nil, nil
	}
	return r.caps[id], nil
}

func (r *fakeCapabilityRepo) List(_ context.Context) ([]*capability.Capability, error) {
	out := make([]*capability.Capability, 0, len(r.caps))
	for _, c := range r.caps {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeCapabilityRepo) Create(_ context.Context, c *capability.Capability) error {
	if r.caps == nil {
		r.caps = map[string]*capability.Capability{}
	}
	r.caps[c.ID] = c
	return nil
}

func (r *fakeCapabilityRepo) Update(_ context.Context, c *capability.Capability) error {
	if r.caps == nil {
		r.caps = map[string]*capability.Capability{}
	}
	r.caps[c.ID] = c
	return nil
}

// seedStructuralChain seeds the four fake repos with a minimal valid
// service-led chain so that an orchestrator test calling Evaluate gets
// past the structural-resolution step. It matches the surface fixture
// stamped by seedSpyStore / seedActiveSurface where ProcessID = "proc-test".
//
// **Harness contract — read this before adding orchestrator tests.**
// There is no single canonical helper that constructs *store.Repositories
// for orchestrator tests; the four structural repos (Processes,
// BusinessServices, BusinessServiceCapabilities, Capabilities) must be
// wired into each test's repository set AND populated via
// seedStructuralChain (or equivalent) before the orchestrator's Evaluate
// flow can complete. Today the wiring lives in three places:
//   - testRepos / newRepos / newOrchestrator       (orchestrator_test.go)
//   - fakeStore / newFakeStore                     (orchestrator_lifecycle_test.go)
//   - spyStore  / newSpyStore                      (orchestrator_accumulator_regression_test.go)
//
// Adding a new harness means you must wire the four repos into your
// harness AND call this helper, OR seed the structural chain by hand.
// Forgetting either step manifests as a nil-pointer panic inside
// resolveStructure rather than a clean test failure. A follow-up PR
// should centralise this into a single decisiontest helper; until then
// the contract is enforced by convention across these three files.
//
// `_ = time.Now` keeps the time import live for callers that pass real
// clock values. The default fixture below leaves CreatedAt zero — the
// orchestrator does not consult these timestamps during resolution.
func seedStructuralChain(
	procs *fakeProcessRepo,
	services *fakeBusinessServiceRepo,
	bsc *fakeBSCRepo,
	caps *fakeCapabilityRepo,
	processID, businessServiceID string,
	capabilityIDs []string,
) {
	_ = time.Now
	if procs.procs == nil {
		procs.procs = map[string]*process.Process{}
	}
	procs.procs[processID] = &process.Process{
		ID:                processID,
		Name:              processID,
		BusinessServiceID: businessServiceID,
		Status:            "active",
		Origin:            "manual",
		Managed:           true,
	}
	if services.services == nil {
		services.services = map[string]*businessservice.BusinessService{}
	}
	services.services[businessServiceID] = &businessservice.BusinessService{
		ID:      businessServiceID,
		Name:    businessServiceID,
		Status:  "active",
		Origin:  "manual",
		Managed: true,
	}
	if bsc.links == nil {
		bsc.links = map[string][]*businessservicecapability.BusinessServiceCapability{}
	}
	if caps.caps == nil {
		caps.caps = map[string]*capability.Capability{}
	}
	for _, capID := range capabilityIDs {
		bsc.links[businessServiceID] = append(bsc.links[businessServiceID], &businessservicecapability.BusinessServiceCapability{
			BusinessServiceID: businessServiceID,
			CapabilityID:      capID,
		})
		caps.caps[capID] = &capability.Capability{
			ID:      capID,
			Name:    capID,
			Status:  "active",
			Origin:  "manual",
			Managed: true,
		}
	}
}
