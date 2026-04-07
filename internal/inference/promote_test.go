package inference

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/store/sqltx"
)

// ---------------------------------------------------------------------------
// Stub implementations for unit tests (no DB required)
// ---------------------------------------------------------------------------

type stubCapPromoter struct {
	meta       map[string]struct{ exists bool; origin string; managed bool }
	replaces   map[string]string
	existsMap  map[string]bool
	created    []string
	deprecated []string
	createErr  error
}

func (s *stubCapPromoter) GetInferredMeta(_ context.Context, id string) (bool, string, bool, error) {
	if m, ok := s.meta[id]; ok {
		return m.exists, m.origin, m.managed, nil
	}
	return false, "", false, nil
}

func (s *stubCapPromoter) GetReplaces(_ context.Context, id string) (string, bool, error) {
	if rep, ok := s.replaces[id]; ok {
		return rep, true, nil
	}
	return "", false, nil
}

func (s *stubCapPromoter) Exists(_ context.Context, id string) (bool, error) {
	return s.existsMap[id], nil
}

func (s *stubCapPromoter) CreateManaged(_ context.Context, _ sqltx.DBTX, id, _ string) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, id)
	return nil
}

func (s *stubCapPromoter) DeprecateInferred(_ context.Context, _ sqltx.DBTX, id string) error {
	s.deprecated = append(s.deprecated, id)
	return nil
}

type stubProcPromoter struct {
	meta       map[string]struct{ exists bool; origin string; managed bool; capabilityID string }
	replaces   map[string]string
	existsMap  map[string]bool
	created    []string
	deprecated []string
}

func (s *stubProcPromoter) GetInferredMeta(_ context.Context, id string) (bool, string, bool, string, error) {
	if m, ok := s.meta[id]; ok {
		return m.exists, m.origin, m.managed, m.capabilityID, nil
	}
	return false, "", false, "", nil
}

func (s *stubProcPromoter) GetReplaces(_ context.Context, id string) (string, bool, error) {
	if rep, ok := s.replaces[id]; ok {
		return rep, true, nil
	}
	return "", false, nil
}

func (s *stubProcPromoter) Exists(_ context.Context, id string) (bool, error) {
	return s.existsMap[id], nil
}

func (s *stubProcPromoter) CreateManaged(_ context.Context, _ sqltx.DBTX, id, _, _ string) error {
	s.created = append(s.created, id)
	return nil
}

func (s *stubProcPromoter) DeprecateInferred(_ context.Context, _ sqltx.DBTX, id string) error {
	s.deprecated = append(s.deprecated, id)
	return nil
}

type stubSurfaceMigrator struct {
	count    int
	migrated int64
}

func (s *stubSurfaceMigrator) CountByProcessID(_ context.Context, _ string) (int, error) {
	return s.count, nil
}

func (s *stubSurfaceMigrator) MigrateProcess(_ context.Context, _ sqltx.DBTX, _, _ string) (int64, error) {
	return s.migrated, nil
}

// newStubService constructs a PromoteService with stub repos and a real (but unused)
// *sql.DB. BeginTx is never called in unit tests because the stubs short-circuit earlier.
// Pass nil for db when testing pre-transaction validation paths.
func newStubService(caps capabilityPromoter, procs processPromoter, surfs surfaceMigrator) *PromoteService {
	return &PromoteService{db: nil, caps: caps, procs: procs, surfs: surfs}
}

// ---------------------------------------------------------------------------
// validatePromoteRequest — pure unit tests (no stub needed)
// ---------------------------------------------------------------------------

func TestValidatePromoteRequest_AllFieldsRequired(t *testing.T) {
	cases := []struct {
		name string
		req  PromoteRequest
		want string
	}{
		{"empty from cap", PromoteRequest{FromProcessID: "p", ToCapabilityID: "c", ToProcessID: "q"}, "from_capability_id"},
		{"empty from proc", PromoteRequest{FromCapabilityID: "c", ToCapabilityID: "d", ToProcessID: "q"}, "from_process_id"},
		{"empty to cap", PromoteRequest{FromCapabilityID: "c", FromProcessID: "p", ToProcessID: "q"}, "to_capability_id"},
		{"empty to proc", PromoteRequest{FromCapabilityID: "c", FromProcessID: "p", ToCapabilityID: "d"}, "to_process_id"},
		{"same cap ids", PromoteRequest{FromCapabilityID: "same", FromProcessID: "p", ToCapabilityID: "same", ToProcessID: "q"}, "must differ"},
		{"same proc ids", PromoteRequest{FromCapabilityID: "c", FromProcessID: "same", ToCapabilityID: "d", ToProcessID: "same"}, "must differ"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePromoteRequest(tc.req)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			var pe PromoteErr
			if !errors.As(err, &pe) {
				t.Errorf("want PromoteErr, got %T: %v", err, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Promote — pre-transaction validation (stub-based unit tests)
// ---------------------------------------------------------------------------

// TestPromote_Returns400_WhenFromCapabilityNotFound verifies that a missing
// from capability produces a PromoteErr.
func TestPromote_Returns400_WhenFromCapabilityNotFound(t *testing.T) {
	caps := &stubCapPromoter{meta: map[string]struct{ exists bool; origin string; managed bool }{}}
	procs := &stubProcPromoter{}
	surfs := &stubSurfaceMigrator{}

	svc := newStubService(caps, procs, surfs)
	_, err := svc.Promote(context.Background(), PromoteRequest{
		FromCapabilityID: "auto:lending",
		FromProcessID:    "auto:lending.origination",
		ToCapabilityID:   "lending",
		ToProcessID:      "lending.origination",
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("want PromoteErr, got %T: %v", err, err)
	}
}

// TestPromote_Returns400_WhenFromCapabilityNotInferred verifies that a capability
// with origin != inferred is rejected.
func TestPromote_Returns400_WhenFromCapabilityNotInferred(t *testing.T) {
	caps := &stubCapPromoter{
		meta: map[string]struct{ exists bool; origin string; managed bool }{
			"auto:lending": {exists: true, origin: "manual", managed: true},
		},
	}
	procs := &stubProcPromoter{}
	surfs := &stubSurfaceMigrator{}

	svc := newStubService(caps, procs, surfs)
	_, err := svc.Promote(context.Background(), PromoteRequest{
		FromCapabilityID: "auto:lending",
		FromProcessID:    "auto:lending.origination",
		ToCapabilityID:   "lending",
		ToProcessID:      "lending.origination",
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("want PromoteErr, got %T: %v", err, err)
	}
}

// TestPromote_Returns400_WhenFromProcessNotFound verifies that a missing
// from process produces a PromoteErr.
func TestPromote_Returns400_WhenFromProcessNotFound(t *testing.T) {
	caps := &stubCapPromoter{
		meta: map[string]struct{ exists bool; origin string; managed bool }{
			"auto:lending": {exists: true, origin: "inferred", managed: false},
		},
	}
	procs := &stubProcPromoter{meta: map[string]struct {
		exists       bool
		origin       string
		managed      bool
		capabilityID string
	}{}}
	surfs := &stubSurfaceMigrator{}

	svc := newStubService(caps, procs, surfs)
	_, err := svc.Promote(context.Background(), PromoteRequest{
		FromCapabilityID: "auto:lending",
		FromProcessID:    "auto:lending.origination",
		ToCapabilityID:   "lending",
		ToProcessID:      "lending.origination",
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("want PromoteErr, got %T: %v", err, err)
	}
}

// TestPromote_Returns400_WhenProcessBelongsToWrongCapability verifies that a
// process whose capability_id does not match from.capability_id is rejected.
func TestPromote_Returns400_WhenProcessBelongsToWrongCapability(t *testing.T) {
	caps := &stubCapPromoter{
		meta: map[string]struct{ exists bool; origin string; managed bool }{
			"auto:lending": {exists: true, origin: "inferred", managed: false},
		},
	}
	procs := &stubProcPromoter{
		meta: map[string]struct {
			exists       bool
			origin       string
			managed      bool
			capabilityID string
		}{
			"auto:lending.origination": {exists: true, origin: "inferred", managed: false, capabilityID: "auto:other"},
		},
	}
	surfs := &stubSurfaceMigrator{}

	svc := newStubService(caps, procs, surfs)
	_, err := svc.Promote(context.Background(), PromoteRequest{
		FromCapabilityID: "auto:lending",
		FromProcessID:    "auto:lending.origination",
		ToCapabilityID:   "lending",
		ToProcessID:      "lending.origination",
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("want PromoteErr, got %T: %v", err, err)
	}
}

// TestPromote_Returns400_WhenToCapabilityAlreadyExists verifies that a
// collision on the target capability ID is rejected.
func TestPromote_Returns400_WhenToCapabilityAlreadyExists(t *testing.T) {
	caps := &stubCapPromoter{
		meta: map[string]struct{ exists bool; origin string; managed bool }{
			"auto:lending": {exists: true, origin: "inferred", managed: false},
		},
		existsMap: map[string]bool{"lending": true},
	}
	procs := &stubProcPromoter{
		meta: map[string]struct {
			exists       bool
			origin       string
			managed      bool
			capabilityID string
		}{
			"auto:lending.origination": {exists: true, origin: "inferred", managed: false, capabilityID: "auto:lending"},
		},
	}
	surfs := &stubSurfaceMigrator{}

	svc := newStubService(caps, procs, surfs)
	_, err := svc.Promote(context.Background(), PromoteRequest{
		FromCapabilityID: "auto:lending",
		FromProcessID:    "auto:lending.origination",
		ToCapabilityID:   "lending",
		ToProcessID:      "lending.origination",
	})

	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("want PromoteErr, got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// checkReplacesChain — cycle detection unit tests
// ---------------------------------------------------------------------------

func TestCheckReplacesChain_NoCycle(t *testing.T) {
	// A → B → (no replaces)
	getReplaces := func(_ context.Context, id string) (string, bool, error) {
		switch id {
		case "A":
			return "B", true, nil
		default:
			return "", false, nil
		}
	}
	err := checkReplacesChain(context.Background(), getReplaces, "A", "C")
	if err != nil {
		t.Errorf("want nil, got %v", err)
	}
}

func TestCheckReplacesChain_DetectsCycle(t *testing.T) {
	// A → B, and target is B → would create a cycle
	getReplaces := func(_ context.Context, id string) (string, bool, error) {
		if id == "A" {
			return "B", true, nil
		}
		return "", false, nil
	}
	err := checkReplacesChain(context.Background(), getReplaces, "A", "B")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("want PromoteErr, got %T: %v", err, err)
	}
}

func TestCheckReplacesChain_PropagatesGetReplacesError(t *testing.T) {
	getReplaces := func(_ context.Context, _ string) (string, bool, error) {
		return "", false, errors.New("db error")
	}
	err := checkReplacesChain(context.Background(), getReplaces, "A", "B")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	// Should NOT be a PromoteErr — it's a system error.
	var pe PromoteErr
	if errors.As(err, &pe) {
		t.Errorf("want wrapped system error, got PromoteErr: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require DATABASE_URL)
// ---------------------------------------------------------------------------

func newTestPromoteService(t *testing.T, db *sql.DB) *PromoteService {
	t.Helper()
	caps, err := postgres.NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}
	procs, err := postgres.NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}
	surfs, err := postgres.NewSurfaceRepo(db)
	if err != nil {
		t.Fatalf("NewSurfaceRepo: %v", err)
	}
	return NewPromoteService(db, caps, procs, surfs)
}

// TestPromote_Integration_FullFlow exercises the complete promotion flow against
// a real database: creates inferred entities, promotes them, and verifies final state.
func TestPromote_Integration_FullFlow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		surfID  = "pr6test.origination"
		capID   = "auto:pr6test"
		procID  = "auto:pr6test.origination"
		toCap   = "pr6test"
		toProc  = "pr6test.origination"
	)

	// Seed inferred structure using the ensure service.
	ensureSvc := newTestService(t, db)
	if _, err := ensureSvc.EnsureInferredStructure(ctx, surfID); err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}

	t.Cleanup(func() {
		// Clean up in FK-safe order: surfaces → processes → capabilities.
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = $1`, surfID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id IN ($1, $2)`, procID, toProc)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id IN ($1, $2)`, capID, toCap)
	})

	promoteSvc := newTestPromoteService(t, db)
	resp, err := promoteSvc.Promote(ctx, PromoteRequest{
		FromCapabilityID: capID,
		FromProcessID:    procID,
		ToCapabilityID:   toCap,
		ToProcessID:      toProc,
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	if resp.SurfacesMigrated != 1 {
		t.Errorf("want SurfacesMigrated=1, got %d", resp.SurfacesMigrated)
	}
	if resp.ToCapabilityID != toCap {
		t.Errorf("want ToCapabilityID=%q, got %q", toCap, resp.ToCapabilityID)
	}
	if resp.ToProcessID != toProc {
		t.Errorf("want ToProcessID=%q, got %q", toProc, resp.ToProcessID)
	}

	// Verify new managed capability was created.
	var capOrigin string
	var capManaged bool
	var capReplaces sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT origin, managed, replaces FROM capabilities WHERE capability_id = $1`, toCap).
		Scan(&capOrigin, &capManaged, &capReplaces); err != nil {
		t.Fatalf("query new capability: %v", err)
	}
	if capOrigin != "manual" {
		t.Errorf("new capability origin: want %q, got %q", "manual", capOrigin)
	}
	if !capManaged {
		t.Error("new capability managed: want true, got false")
	}
	if !capReplaces.Valid || capReplaces.String != capID {
		t.Errorf("new capability replaces: want %q, got %v", capID, capReplaces)
	}

	// Verify new managed process was created.
	var procOrigin string
	var procManaged bool
	var procReplaces sql.NullString
	var procCapID string
	if err := db.QueryRowContext(ctx, `SELECT origin, managed, replaces, capability_id FROM processes WHERE process_id = $1`, toProc).
		Scan(&procOrigin, &procManaged, &procReplaces, &procCapID); err != nil {
		t.Fatalf("query new process: %v", err)
	}
	if procOrigin != "manual" {
		t.Errorf("new process origin: want %q, got %q", "manual", procOrigin)
	}
	if !procManaged {
		t.Error("new process managed: want true, got false")
	}
	if !procReplaces.Valid || procReplaces.String != procID {
		t.Errorf("new process replaces: want %q, got %v", procID, procReplaces)
	}
	if procCapID != toCap {
		t.Errorf("new process capability_id: want %q, got %q", toCap, procCapID)
	}

	// Verify surface was migrated in place.
	var surfProcID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT process_id FROM decision_surfaces WHERE id = $1 AND version = 1`, surfID).
		Scan(&surfProcID); err != nil {
		t.Fatalf("query surface: %v", err)
	}
	if !surfProcID.Valid || surfProcID.String != toProc {
		t.Errorf("surface process_id: want %q, got %v", toProc, surfProcID)
	}

	// Verify old inferred entities were deprecated.
	var oldCapStatus, oldProcStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM capabilities WHERE capability_id = $1`, capID).
		Scan(&oldCapStatus); err != nil {
		t.Fatalf("query old capability: %v", err)
	}
	if oldCapStatus != "deprecated" {
		t.Errorf("old capability status: want %q, got %q", "deprecated", oldCapStatus)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM processes WHERE process_id = $1`, procID).
		Scan(&oldProcStatus); err != nil {
		t.Fatalf("query old process: %v", err)
	}
	if oldProcStatus != "deprecated" {
		t.Errorf("old process status: want %q, got %q", "deprecated", oldProcStatus)
	}
}

// TestPromote_Integration_IdempotencyGuard verifies that promoting the same
// inferred entities twice fails on the second call (to entities already exist).
func TestPromote_Integration_IdempotencyGuard(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		surfID = "pr6idem.origination"
		capID  = "auto:pr6idem"
		procID = "auto:pr6idem.origination"
		toCap  = "pr6idem"
		toProc = "pr6idem.origination"
	)

	ensureSvc := newTestService(t, db)
	if _, err := ensureSvc.EnsureInferredStructure(ctx, surfID); err != nil {
		t.Fatalf("EnsureInferredStructure: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM decision_surfaces WHERE id = $1`, surfID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id IN ($1, $2)`, procID, toProc)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id IN ($1, $2)`, capID, toCap)
	})

	promoteSvc := newTestPromoteService(t, db)
	req := PromoteRequest{FromCapabilityID: capID, FromProcessID: procID, ToCapabilityID: toCap, ToProcessID: toProc}

	if _, err := promoteSvc.Promote(ctx, req); err != nil {
		t.Fatalf("first Promote: %v", err)
	}

	_, err := promoteSvc.Promote(ctx, req)
	if err == nil {
		t.Fatal("second Promote: want error (target entities exist), got nil")
	}
	var pe PromoteErr
	if !errors.As(err, &pe) {
		t.Errorf("second Promote: want PromoteErr, got %T: %v", err, err)
	}
}
