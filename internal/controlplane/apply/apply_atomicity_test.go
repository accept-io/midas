package apply

// apply_atomicity_test.go — encodes the intended atomic-rollback contract
// for control-plane apply execution.
//
// Contract (the decision this test defines):
//   When Apply executes a bundle of multiple documents, either EVERY
//   operation that reaches the executor persists successfully, or NONE do.
//   A runtime repository error at operation N in a multi-document bundle
//   must leave persistence in the same state as before the bundle was
//   applied:
//
//     (a) documents persisted before N must NOT remain,
//     (b) documents queued after N must NOT execute.
//
// Rationale:
//   Partial persistence is the control-plane equivalent of a half-written
//   schema change. It leaves downstream referential state in an
//   operator-unfriendly intermediate shape (for example: a Capability
//   visible to readers while the Process meant to own it is missing, and
//   the Surface meant to consume the Process never materialises). Apply
//   is a declarative operation over structurally-linked resources;
//   atomicity is the only semantic that preserves the invariants the
//   resource graph encodes.
//
// Scope:
//   This test only concerns the EXECUTION phase. Failures that would have
//   been caught earlier — parse errors, structural validation, referential
//   integrity at plan time — are out of scope. The injected error here is
//   a runtime Create failure returned by the repository layer AFTER the
//   plan has been fully validated. In production this maps to transient
//   DB errors, constraint violations, or network blips during the executor
//   loop.
//
// Expected status against HEAD:
//   This test is EXPECTED TO FAIL against the current apply executor.
//   The executor at internal/controlplane/apply/service.go (executePlan)
//   is not wrapped in a transaction; on a mid-loop Create error it
//   records the error via result.AddError and continues to the next
//   entry. That means today:
//     - the pre-failure Capability remains in its repo,
//     - the post-failure Surface is still executed and persisted.
//   Both violations are asserted below. The test will pass once Issue 1
//   lands atomic-apply semantics.
//
// Test-only seam:
//   None in production code. The existing RepositorySet / repository
//   interface surface is sufficient: the test substitutes focused fakes
//   that implement the SurfaceRepository, ProcessRepository, and
//   CapabilityRepository interfaces. No new field, flag, or hook is
//   introduced on any production struct.

import (
	"context"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Test-only repository fakes.
//
// Each fake tracks three distinct things, and it matters which one a given
// assertion reads:
//
//   - createCalls: a counter incremented at the TOP of Create, before any
//     conditional skip, append, or error return. It records the number of
//     Create ATTEMPTS the executor made against this repo. Use this to
//     assert whether the executor reached a given operation at all.
//
//   - provisional: entities Create wrote during the current transaction.
//     These have been accepted by the repo but not yet committed.
//
//   - created: the slice of entities the fake has COMMITTED. This records
//     the OBSERVABLE STATE of the repo after a run. Use this to assert
//     whether a prior write "remained" after a failure.
//
// On commit() provisional moves to created; on rollback() provisional is
// discarded. The apply service drives commit/rollback through the
// fakeTxRunner (see below) that this test file wires into RepositorySet.Tx.
//
// The distinction between provisional and created is the core of the
// atomicity test: a transactional apply runs every Create through
// provisional, then commits together; on mid-bundle failure the TxRunner
// rolls back and created stays empty.
// ---------------------------------------------------------------------------

// stagingRepo is implemented by every recording fake so a single fakeTxRunner
// can commit or rollback them uniformly.
type stagingRepo interface {
	commit()
	rollback()
}

type recordingBusinessServiceRepo struct {
	createCalls int
	provisional []*businessservice.BusinessService
	created     []*businessservice.BusinessService
}

func (r *recordingBusinessServiceRepo) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *recordingBusinessServiceRepo) GetByID(_ context.Context, _ string) (*businessservice.BusinessService, error) {
	return nil, nil
}
func (r *recordingBusinessServiceRepo) Create(_ context.Context, s *businessservice.BusinessService) error {
	r.createCalls++
	r.provisional = append(r.provisional, s)
	return nil
}
func (r *recordingBusinessServiceRepo) commit() {
	r.created = append(r.created, r.provisional...)
	r.provisional = nil
}
func (r *recordingBusinessServiceRepo) rollback() {
	r.provisional = nil
}

// recordingProcessRepoWithFailure's Create returns an error when the
// candidate's ID matches failID, simulating a repository-layer failure
// encountered partway through executor iteration.
type recordingProcessRepoWithFailure struct {
	createCalls int
	provisional []*process.Process
	created     []*process.Process
	failID      string // if non-empty, Create returns failErr for this ID
	failErr     error
}

func (r *recordingProcessRepoWithFailure) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *recordingProcessRepoWithFailure) GetByID(_ context.Context, _ string) (*process.Process, error) {
	return nil, nil
}
func (r *recordingProcessRepoWithFailure) Create(_ context.Context, p *process.Process) error {
	r.createCalls++
	if r.failID != "" && p.ID == r.failID {
		return r.failErr
	}
	r.provisional = append(r.provisional, p)
	return nil
}
func (r *recordingProcessRepoWithFailure) commit() {
	r.created = append(r.created, r.provisional...)
	r.provisional = nil
}
func (r *recordingProcessRepoWithFailure) rollback() {
	r.provisional = nil
}

type recordingSurfaceRepo struct {
	createCalls int
	provisional []*surface.DecisionSurface
	created     []*surface.DecisionSurface
}

func (r *recordingSurfaceRepo) FindLatestByID(_ context.Context, _ string) (*surface.DecisionSurface, error) {
	return nil, nil
}
func (r *recordingSurfaceRepo) Create(_ context.Context, s *surface.DecisionSurface) error {
	r.createCalls++
	r.provisional = append(r.provisional, s)
	return nil
}
func (r *recordingSurfaceRepo) commit() {
	r.created = append(r.created, r.provisional...)
	r.provisional = nil
}
func (r *recordingSurfaceRepo) rollback() {
	r.provisional = nil
}

// fakeTxRunner is the transaction-participation seam the atomicity test
// wires into RepositorySet.Tx. It runs fn against the same RepositorySet
// the service would otherwise use directly, then commits (moves
// provisional → created) or rolls back (discards provisional) on every
// recording fake it knows about.
//
// The runner is deliberately minimal: it does not create a "scoped" set
// of fresh repositories. The recording fakes already model the two-phase
// view (provisional + created) internally, so passing the outer set
// straight through is sufficient for the contract under test.
type fakeTxRunner struct {
	staging []stagingRepo
	set     *RepositorySet
}

func newFakeTxRunner(set *RepositorySet, staging ...stagingRepo) *fakeTxRunner {
	return &fakeTxRunner{set: set, staging: staging}
}

func (r *fakeTxRunner) WithTx(_ context.Context, _ string, fn func(*RepositorySet) error) error {
	if err := fn(r.set); err != nil {
		for _, s := range r.staging {
			s.rollback()
		}
		return err
	}
	for _, s := range r.staging {
		s.commit()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Bundle fixture — three documents, two kinds crossed, well-formed.
//
//   doc 1: BusinessService "bs-atomic-test"  (tier 0 — executes first)
//   doc 2: Process         "proc-atomic-test" (tier 2 — executes second)
//   doc 3: Surface         "surf-atomic-test" (tier 3 — executes third)
//
// The Process references the BusinessService and the Surface references the
// Process, so referential integrity passes at plan time via same-bundle
// resolution (no repository lookups required for the references). Every
// document is structurally valid; none of these cases would have been
// rejected by parse or by validate.
// ---------------------------------------------------------------------------

func atomicityBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-atomic-test",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-atomic-test", Name: "Atomicity Test Service"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-atomic-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-atomic-test", Name: "Atomicity Test Process"},
				Spec:       types.ProcessSpec{BusinessServiceID: "bs-atomic-test", Status: "active"},
			},
		},
		{
			Kind: types.KindSurface,
			ID:   "surf-atomic-test",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata:   types.DocumentMetadata{ID: "surf-atomic-test", Name: "Atomicity Test Surface"},
				Spec: types.SurfaceSpec{
					Category:  "financial",
					RiskTier:  "high",
					Status:    "active",
					ProcessID: "proc-atomic-test",
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Contract test: mid-bundle Create failure must roll back.
//
// With atomic apply landed (see RepositorySet.Tx / TxRunner), today's
// executor aborts on the first persistence error and the fakeTxRunner
// rolls back the provisional writes — so the Capability stays off the
// committed list and the Surface is never attempted.
// ---------------------------------------------------------------------------

func TestApplyExecution_Atomic_NoResidueOnMidBundleFailure(t *testing.T) {
	bsRepo := &recordingBusinessServiceRepo{}
	procRepo := &recordingProcessRepoWithFailure{
		failID:  "proc-atomic-test",
		failErr: errors.New("simulated repository failure during Process.Create"),
	}
	surfRepo := &recordingSurfaceRepo{}

	set := RepositorySet{
		BusinessServices: bsRepo,
		Processes:        procRepo,
		Surfaces:         surfRepo,
	}
	set.Tx = newFakeTxRunner(&set, bsRepo, procRepo, surfRepo)
	svc := NewServiceWithRepos(set)

	result := svc.Apply(context.Background(), atomicityBundle(), "tester")

	// Sanity: the error was surfaced by the executor. This is not the
	// contract assertion — it just confirms the failure-injection seam
	// fired where we intended.
	if result.ApplyErrorCount() == 0 {
		t.Fatalf("expected an error to be surfaced on Process.Create failure; got result=%+v", result)
	}

	// Contract (a): earlier writes must not remain persisted.
	//
	// Phrased against observable persisted state: after the failure, the
	// BusinessService must not be visible to any reader of the repository.
	// Whether the executor reached BusinessService.Create is beside the point —
	// what matters is that no trace of it remains.
	if len(bsRepo.created) != 0 {
		t.Errorf("atomic-apply contract violated: BusinessService persisted before the failure remained in the repo "+
			"(want 0 records, got %d). The executor must wrap state changes in a transaction so prior Create "+
			"calls roll back when a later Create fails.", len(bsRepo.created))
	}

	// Contract (b): later operations must not execute.
	//
	// Phrased against Create call ATTEMPTS rather than persisted state.
	// A plausible atomic-apply implementation could call Create and then
	// roll back on commit failure, leaving surfRepo.created empty even
	// though the Surface operation was attempted. That shape would pass
	// an "is it persisted" check, but it would violate the contract the
	// test is actually encoding: the executor must stop at the first
	// failure and never attempt downstream operations.
	if surfRepo.createCalls != 0 {
		t.Errorf("atomic-apply contract violated: Surface queued after the failure was still attempted "+
			"(want 0 Create calls, got %d). Issue 1 must abort the executor loop on first error, not "+
			"continue through the remaining plan entries.", surfRepo.createCalls)
	}

	// Defensive: the injected failure itself was exercised exactly once,
	// and the fake's append path did not run (Create returned failErr
	// before the append). Documents the invariant so a regression in the
	// fake itself does not silently mask a regression in the SUT.
	if procRepo.createCalls != 1 {
		t.Errorf("failure-injection sanity: want exactly 1 Process Create attempt, got %d", procRepo.createCalls)
	}
	if len(procRepo.created) != 0 {
		t.Errorf("unexpected: injected-failure record appeared in Process repo (%d records); check the fake",
			len(procRepo.created))
	}
}

// ---------------------------------------------------------------------------
// Positive control: same harness, no injected failure, all three documents
// must persist. This keeps the atomicity test honest — if the failing
// assertions above ever start passing for the wrong reason (e.g. the
// executor silently skipping writes), the positive control will fail and
// point at the harness rather than the SUT.
// ---------------------------------------------------------------------------

func TestApplyExecution_Atomic_PositiveControl_AllPersistSansFailure(t *testing.T) {
	bsRepo := &recordingBusinessServiceRepo{}
	procRepo := &recordingProcessRepoWithFailure{} // no failID → never fails
	surfRepo := &recordingSurfaceRepo{}

	set := RepositorySet{
		BusinessServices: bsRepo,
		Processes:        procRepo,
		Surfaces:         surfRepo,
	}
	set.Tx = newFakeTxRunner(&set, bsRepo, procRepo, surfRepo)
	svc := NewServiceWithRepos(set)

	result := svc.Apply(context.Background(), atomicityBundle(), "tester")

	if result.ApplyErrorCount() != 0 {
		t.Fatalf("positive control: expected zero apply errors, got %d: %+v",
			result.ApplyErrorCount(), result.Results)
	}
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("positive control: expected zero validation errors, got %d: %+v",
			result.ValidationErrorCount(), result.ValidationErrors)
	}
	// Call-attempt assertions: the executor must have reached every
	// repository exactly once. Paired with the persisted-length checks
	// below, these confirm that the fakes' two views of the world
	// (attempts vs. persisted state) agree on the happy path — so that
	// any disagreement surfaced by the failure test above is a real
	// signal about the SUT, not an artefact of the harness.
	if bsRepo.createCalls != 1 {
		t.Errorf("positive control: want 1 BusinessService Create attempt, got %d", bsRepo.createCalls)
	}
	if procRepo.createCalls != 1 {
		t.Errorf("positive control: want 1 Process Create attempt, got %d", procRepo.createCalls)
	}
	if surfRepo.createCalls != 1 {
		t.Errorf("positive control: want 1 Surface Create attempt, got %d", surfRepo.createCalls)
	}

	// Persisted-state assertions: every attempt resulted in a stored
	// record. No skips, no silent drops.
	if len(bsRepo.created) != 1 {
		t.Errorf("positive control: want 1 BusinessService persisted, got %d", len(bsRepo.created))
	}
	if len(procRepo.created) != 1 {
		t.Errorf("positive control: want 1 Process persisted, got %d", len(procRepo.created))
	}
	if len(surfRepo.created) != 1 {
		t.Errorf("positive control: want 1 Surface persisted, got %d", len(surfRepo.created))
	}
	if result.CreatedCount() != 3 {
		t.Errorf("positive control: want 3 resources created in result, got %d", result.CreatedCount())
	}
}
