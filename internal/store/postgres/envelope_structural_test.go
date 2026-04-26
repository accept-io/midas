package postgres

// envelope_structural_test.go — round-trip, ordering, JSONB shape, and
// audit-survival coverage for ADR-0001 envelope structural snapshot fields
// against the Postgres envelope repo.
//
// Tests skip when DATABASE_URL is unset.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/envelope"
)

func newStructuralEnv(t *testing.T, id string, structure envelope.ResolvedStructure) *envelope.Envelope {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	env, err := envelope.New(id, "src-test", "req-"+id, json.RawMessage(`{}`), now)
	if err != nil {
		t.Fatalf("envelope.New: %v", err)
	}
	env.Resolved.Structure = structure
	return env
}

// TestEnvelopeRepo_StructuralRoundTrip_Populated asserts that all 11 new
// columns plus the JSONB capability snapshot survive Create+GetByID with
// byte-equality on the capability set.
func TestEnvelopeRepo_StructuralRoundTrip_Populated(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	want := envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-pg-1", Origin: "manual", Managed: true, Replaces: "proc-pg-0", Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-pg-1", Origin: "manual", Managed: true, Replaces: "", Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{
			{ID: "cap-x-001", Name: "Cap One", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
			{ID: "cap-x-002", Name: "Cap Two", Origin: "manual", Managed: true, Replaces: "cap-x-old", Status: "deprecated"},
			{ID: "cap-x-003", Name: "Cap Three", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
		},
	}
	env := newStructuralEnv(t, "env-pg-rt-1", want)

	ctx := context.Background()
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "env-pg-rt-1")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}

	if got.Resolved.Structure.Process != want.Process {
		t.Errorf("process: want %+v, got %+v", want.Process, got.Resolved.Structure.Process)
	}
	if got.Resolved.Structure.BusinessService != want.BusinessService {
		t.Errorf("business service: want %+v, got %+v", want.BusinessService, got.Resolved.Structure.BusinessService)
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != len(want.EnablingCapabilities) {
		t.Fatalf("capability count: want %d, got %d", len(want.EnablingCapabilities), len(got.Resolved.Structure.EnablingCapabilities))
	}
	for i, c := range want.EnablingCapabilities {
		if got.Resolved.Structure.EnablingCapabilities[i] != c {
			t.Errorf("capability[%d]: want %+v, got %+v", i, c, got.Resolved.Structure.EnablingCapabilities[i])
		}
	}
}

// TestEnvelopeRepo_StructuralRoundTrip_EmptyCapabilities asserts the
// empty-set behaviour required by PR-3 and ADR-0001: a BusinessService
// with zero enabling capabilities round-trips with a non-nil empty
// capability slice (JSON `[]`, never `null`).
func TestEnvelopeRepo_StructuralRoundTrip_EmptyCapabilities(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	env := newStructuralEnv(t, "env-pg-rt-empty", envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-pg-empty", Origin: "manual", Managed: true, Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-pg-empty", Origin: "manual", Managed: true, Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{},
	})

	ctx := context.Background()
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "env-pg-rt-empty")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}
	if got.Resolved.Structure.EnablingCapabilities == nil {
		t.Error("want non-nil empty slice; got nil")
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != 0 {
		t.Errorf("want length 0; got %d", len(got.Resolved.Structure.EnablingCapabilities))
	}

	// Read the raw column and confirm it is the JSON literal `[]`, not `null`.
	var raw []byte
	if err := db.QueryRow(`SELECT resolved_enabling_capabilities_json::text FROM operational_envelopes WHERE id = $1`, "env-pg-rt-empty").Scan(&raw); err != nil {
		t.Fatalf("read raw column: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("on-disk column: want %q, got %q", "[]", string(raw))
	}
}

// TestEnvelopeRepo_StructuralCapabilityOrdering asserts the repository
// sorts capabilities by ID ascending on write, regardless of caller
// ordering. Determinism is required for byte-level envelope hashing in
// future GCA work.
func TestEnvelopeRepo_StructuralCapabilityOrdering(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	// Caller passes capabilities in deliberately non-sorted order.
	unsorted := []envelope.CapabilitySnapshot{
		{ID: "cap-z", Name: "Z", Origin: "manual", Managed: true, Status: "active"},
		{ID: "cap-a", Name: "A", Origin: "manual", Managed: true, Status: "active"},
		{ID: "cap-m", Name: "M", Origin: "manual", Managed: true, Status: "active"},
	}
	env := newStructuralEnv(t, "env-pg-order", envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-pg-order", Origin: "manual", Managed: true, Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-pg-order", Origin: "manual", Managed: true, Status: "active",
		},
		EnablingCapabilities: unsorted,
	})

	ctx := context.Background()
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "env-pg-order")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}

	wantIDs := []string{"cap-a", "cap-m", "cap-z"}
	for i, want := range wantIDs {
		if got.Resolved.Structure.EnablingCapabilities[i].ID != want {
			t.Errorf("position %d: want %q, got %q", i, want, got.Resolved.Structure.EnablingCapabilities[i].ID)
		}
	}
}

// TestEnvelopeRepo_StructuralDeterminism asserts two envelopes built with
// the same structural snapshot produce byte-identical capability JSONB on
// disk, regardless of the caller-side input order.
func TestEnvelopeRepo_StructuralDeterminism(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	build := func(id string, caps []envelope.CapabilitySnapshot) *envelope.Envelope {
		return newStructuralEnv(t, id, envelope.ResolvedStructure{
			Process:              envelope.ProcessSnapshot{ID: "proc-pg-det", Origin: "manual", Managed: true, Status: "active"},
			BusinessService:      envelope.BusinessServiceSnapshot{ID: "bs-pg-det", Origin: "manual", Managed: true, Status: "active"},
			EnablingCapabilities: caps,
		})
	}

	caps1 := []envelope.CapabilitySnapshot{
		{ID: "cap-d-1", Name: "One", Origin: "manual", Managed: true, Status: "active"},
		{ID: "cap-d-2", Name: "Two", Origin: "manual", Managed: true, Status: "active"},
	}
	caps2 := []envelope.CapabilitySnapshot{
		{ID: "cap-d-2", Name: "Two", Origin: "manual", Managed: true, Status: "active"},
		{ID: "cap-d-1", Name: "One", Origin: "manual", Managed: true, Status: "active"},
	}

	ctx := context.Background()
	if err := repo.Create(ctx, build("env-det-A", caps1)); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if err := repo.Create(ctx, build("env-det-B", caps2)); err != nil {
		t.Fatalf("Create B: %v", err)
	}

	var rawA, rawB []byte
	if err := db.QueryRow(`SELECT resolved_enabling_capabilities_json::text FROM operational_envelopes WHERE id = $1`, "env-det-A").Scan(&rawA); err != nil {
		t.Fatalf("read A: %v", err)
	}
	if err := db.QueryRow(`SELECT resolved_enabling_capabilities_json::text FROM operational_envelopes WHERE id = $1`, "env-det-B").Scan(&rawB); err != nil {
		t.Fatalf("read B: %v", err)
	}
	if string(rawA) != string(rawB) {
		t.Errorf("on-disk JSONB differs between two envelopes with semantically identical capability sets:\n  A: %s\n  B: %s", string(rawA), string(rawB))
	}
}

// TestEnvelopeRepo_StructuralCapabilityWithEmptyReplaces asserts a
// capability with an empty `replaces` field round-trips correctly. This
// is the predominant case (most capabilities have no predecessor) and
// must not require special-casing by callers.
func TestEnvelopeRepo_StructuralCapabilityWithEmptyReplaces(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	env := newStructuralEnv(t, "env-pg-emptyrepl", envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-pg-emptyrepl", Origin: "manual", Managed: true, Replaces: "", Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-pg-emptyrepl", Origin: "manual", Managed: true, Replaces: "", Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{
			{ID: "cap-er-1", Name: "EmptyRepl", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
		},
	})

	ctx := context.Background()
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, "env-pg-emptyrepl")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}

	if got.Resolved.Structure.Process.Replaces != "" {
		t.Errorf("process.Replaces round-trip: want \"\", got %q", got.Resolved.Structure.Process.Replaces)
	}
	if got.Resolved.Structure.BusinessService.Replaces != "" {
		t.Errorf("business_service.Replaces round-trip: want \"\", got %q", got.Resolved.Structure.BusinessService.Replaces)
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != 1 {
		t.Fatalf("capability count: want 1, got %d", len(got.Resolved.Structure.EnablingCapabilities))
	}
	if got.Resolved.Structure.EnablingCapabilities[0].Replaces != "" {
		t.Errorf("capability.Replaces round-trip: want \"\", got %q", got.Resolved.Structure.EnablingCapabilities[0].Replaces)
	}
}

// TestEnvelopeRepo_StructuralAuditSurvival asserts the envelope's
// capability snapshot survives subsequent deletion of the underlying
// Capability row — proving the deliberate FK omission in ADR-0001 holds
// at runtime. The envelope is point-in-time evidence; the live capabilities
// table can change without rewriting history.
func TestEnvelopeRepo_StructuralAuditSurvival(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ctx := context.Background()

	// Seed a Capability row that will later be deleted out from under the
	// snapshot.  No FK exists between the envelope's JSONB snapshot and
	// this row, so the deletion must succeed and leave the envelope intact.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO capabilities (capability_id, name, status, origin, managed, created_at, updated_at)
		VALUES ('cap-audit-survive', 'Audit Survive', 'active', 'manual', true, $1, $1)
	`, now); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, "cap-audit-survive")
	})

	env := newStructuralEnv(t, "env-pg-audit-survive", envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-pg-audit", Origin: "manual", Managed: true, Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-pg-audit", Origin: "manual", Managed: true, Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{
			{ID: "cap-audit-survive", Name: "Audit Survive", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
		},
	})
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create envelope: %v", err)
	}

	// Delete the capability that the snapshot names. Must succeed because
	// nothing in operational_envelopes FKs into capabilities.
	if _, err := db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, "cap-audit-survive"); err != nil {
		t.Fatalf("delete capability post-write: %v — proves an unintended FK exists", err)
	}

	// Re-read the envelope. The snapshot must still name the deleted
	// capability with full lifecycle metadata.
	got, err := repo.GetByID(ctx, "env-pg-audit-survive")
	if err != nil || got == nil {
		t.Fatalf("GetByID after capability deletion: env=%v err=%v", got, err)
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != 1 {
		t.Fatalf("capability snapshot length: want 1, got %d (envelope evidence did not survive structural change)", len(got.Resolved.Structure.EnablingCapabilities))
	}
	cap0 := got.Resolved.Structure.EnablingCapabilities[0]
	if cap0.ID != "cap-audit-survive" || cap0.Name != "Audit Survive" || cap0.Origin != "manual" || cap0.Status != "active" || !cap0.Managed {
		t.Errorf("capability snapshot was mutated by structural deletion: got %+v", cap0)
	}
}

// TestEnvelopeRepo_StructuralReadPathOverride asserts that when the
// dedicated columns and the embedded copy inside resolved_json carry
// divergent values, the read path returns the dedicated-column values.
// This pins the override semantics documented in the implementation
// report: dedicated columns are written explicitly with the envelope row
// and are the authoritative source on read; resolved_json is allowed to
// drift only as a side effect of partial writes or schema evolution, and
// the scan path overwrites the unmarshalled blob with column values.
func TestEnvelopeRepo_StructuralReadPathOverride(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	// Step 1: write an envelope normally so all columns and resolved_json
	// agree on a known structural snapshot.
	env := newStructuralEnv(t, "env-override", envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-truth", Origin: "manual", Managed: true, Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-truth", Origin: "manual", Managed: true, Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{
			{ID: "cap-truth-1", Name: "Truth One", Origin: "manual", Managed: true, Status: "active"},
		},
	})
	ctx := context.Background()
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Step 2: poison resolved_json with divergent structural values via
	// raw SQL. The dedicated columns and resolved_enabling_capabilities_json
	// remain untouched; only the embedded copy inside resolved_json
	// disagrees with them.
	const divergentResolved = `{
		"authority": {"surface_id":"","surface_version":0,"profile_id":"","profile_version":0,"agent_id":"","grant_id":""},
		"structure": {
			"process": {"id":"proc-LIE","origin":"manual","managed":false,"replaces":"","status":"deprecated"},
			"business_service": {"id":"bs-LIE","origin":"manual","managed":false,"replaces":"","status":"deprecated"},
			"enabling_capabilities": [
				{"id":"cap-LIE","name":"Liar","origin":"manual","managed":false,"replaces":"","status":"deprecated"}
			]
		},
		"metadata": {}
	}`
	if _, err := db.ExecContext(ctx, `UPDATE operational_envelopes SET resolved_json = $1::jsonb WHERE id = $2`, divergentResolved, "env-override"); err != nil {
		t.Fatalf("poison resolved_json: %v", err)
	}

	// Step 3: read the envelope. The dedicated columns must win.
	got, err := repo.GetByID(ctx, "env-override")
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}
	if got.Resolved.Structure.Process.ID != "proc-truth" {
		t.Errorf("process.ID: dedicated column should win; want %q, got %q (resolved_json contained %q)",
			"proc-truth", got.Resolved.Structure.Process.ID, "proc-LIE")
	}
	if got.Resolved.Structure.Process.Status != "active" {
		t.Errorf("process.Status: dedicated column should win; want %q, got %q",
			"active", got.Resolved.Structure.Process.Status)
	}
	if !got.Resolved.Structure.Process.Managed {
		t.Errorf("process.Managed: dedicated column should win; want true, got false")
	}
	if got.Resolved.Structure.BusinessService.ID != "bs-truth" {
		t.Errorf("business_service.ID: dedicated column should win; want %q, got %q (resolved_json contained %q)",
			"bs-truth", got.Resolved.Structure.BusinessService.ID, "bs-LIE")
	}
	if got.Resolved.Structure.BusinessService.Status != "active" {
		t.Errorf("business_service.Status: dedicated column should win; want %q, got %q",
			"active", got.Resolved.Structure.BusinessService.Status)
	}
	// Capabilities: the dedicated JSONB column was not poisoned, so the
	// read should return the original snapshot. This proves the JSONB
	// column is read independently of resolved_json's embedded copy.
	if len(got.Resolved.Structure.EnablingCapabilities) != 1 {
		t.Fatalf("capability count: want 1, got %d", len(got.Resolved.Structure.EnablingCapabilities))
	}
	if got.Resolved.Structure.EnablingCapabilities[0].ID != "cap-truth-1" {
		t.Errorf("capability.ID: dedicated JSONB column should win; want %q, got %q (resolved_json contained %q)",
			"cap-truth-1", got.Resolved.Structure.EnablingCapabilities[0].ID, "cap-LIE")
	}
}

// TestEnvelopeRepo_StructuralCapabilityByteIdentity asserts that the
// capability array bytes inside resolved_json (path: structure.enabling_capabilities)
// and inside the dedicated resolved_enabling_capabilities_json column are
// byte-identical for the same envelope. This proves the persistence-level
// duplication is consistent: both representations come from the same
// in-memory slice and are sorted in place before marshalling, so postgres
// stores them in the same canonical JSONB form.
func TestEnvelopeRepo_StructuralCapabilityByteIdentity(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	cleanupOperationalEnvelopes(t, db)

	repo, err := NewEnvelopeRepo(db)
	if err != nil {
		t.Fatalf("NewEnvelopeRepo: %v", err)
	}

	// Build an envelope with several capabilities in deliberately non-sorted
	// input order, mixing populated and empty replaces, so the byte-identity
	// claim has something interesting to assert against.
	env := newStructuralEnv(t, "env-byte-id", envelope.ResolvedStructure{
		Process: envelope.ProcessSnapshot{
			ID: "proc-byte", Origin: "manual", Managed: true, Status: "active",
		},
		BusinessService: envelope.BusinessServiceSnapshot{
			ID: "bs-byte", Origin: "manual", Managed: true, Status: "active",
		},
		EnablingCapabilities: []envelope.CapabilitySnapshot{
			{ID: "cap-byte-z", Name: "Zed", Origin: "manual", Managed: true, Replaces: "cap-byte-old", Status: "deprecated"},
			{ID: "cap-byte-a", Name: "Alpha", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
			{ID: "cap-byte-m", Name: "Mid", Origin: "manual", Managed: true, Replaces: "", Status: "active"},
		},
	})

	ctx := context.Background()
	if err := repo.Create(ctx, env); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read both representations as text. Postgres serialises JSONB to text
	// in a canonical form (sorted keys, no whitespace) so byte-equality of
	// the text representation implies byte-equality of the stored JSONB.
	var embeddedText, dedicatedText string
	err = db.QueryRowContext(ctx, `
		SELECT
			(resolved_json->'structure'->'enabling_capabilities')::text,
			resolved_enabling_capabilities_json::text
		FROM operational_envelopes
		WHERE id = $1
	`, "env-byte-id").Scan(&embeddedText, &dedicatedText)
	if err != nil {
		t.Fatalf("read both columns: %v", err)
	}

	if embeddedText != dedicatedText {
		t.Errorf("capability JSON bytes diverge between resolved_json and resolved_enabling_capabilities_json:\n"+
			"  embedded  (resolved_json->structure->enabling_capabilities): %s\n"+
			"  dedicated (resolved_enabling_capabilities_json):             %s",
			embeddedText, dedicatedText)
	}
}
