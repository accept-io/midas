package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/externalref"
)

func TestCapabilityRepo_CreateAndGetByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        "tst-cap-001",
		Name:      "Identity Verification",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		Replaces:  "",
		Owner:     "team-platform",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, c.ID)
	})

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}

	checks := []struct{ field, want, got string }{
		{"ID", c.ID, got.ID},
		{"Name", c.Name, got.Name},
		{"Status", c.Status, got.Status},
		{"Origin", c.Origin, got.Origin},
		{"Replaces", c.Replaces, got.Replaces},
		{"Owner", c.Owner, got.Owner},
	}
	for _, ck := range checks {
		if ck.want != ck.got {
			t.Errorf("%s: want %q, got %q", ck.field, ck.want, ck.got)
		}
	}
	if got.Managed != c.Managed {
		t.Errorf("Managed: want %v, got %v", c.Managed, got.Managed)
	}
	if !got.CreatedAt.Equal(c.CreatedAt) {
		t.Errorf("CreatedAt: want %v, got %v", c.CreatedAt, got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(c.UpdatedAt) {
		t.Errorf("UpdatedAt: want %v, got %v", c.UpdatedAt, got.UpdatedAt)
	}
}

func TestCapabilityRepo_GetByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	got, err := repo.GetByID(ctx, "tst-cap-nonexistent")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCapabilityRepo_Update_DoesNotMutateLifecycleFields(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        "tst-cap-upd-001",
		Name:      "Original Name",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, c.ID)
	})

	updated := now.Add(time.Second)
	c.Name = "Updated Name"
	c.Status = "deprecated"
	c.Description = "now has a description"
	c.Owner = "team-new"
	c.UpdatedAt = updated

	if err := repo.Update(ctx, c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name: want %q, got %q", "Updated Name", got.Name)
	}
	if got.Status != "deprecated" {
		t.Errorf("Status: want deprecated, got %s", got.Status)
	}
	// origin and managed are immutable via Update
	if got.Origin != "manual" {
		t.Errorf("Origin: want manual, got %s", got.Origin)
	}
	if !got.Managed {
		t.Error("Managed: want true, got false")
	}
}

func TestCapabilityRepo_List(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ids := []string{"tst-cap-list-001", "tst-cap-list-002"}
	for i, id := range ids {
		c := &capability.Capability{
			ID:        id,
			Name:      fmt.Sprintf("Capability %d", i+1),
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := repo.Create(ctx, c); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
		}
	})

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := 0
	for _, c := range all {
		for _, id := range ids {
			if c.ID == id {
				found++
			}
		}
	}
	if found != len(ids) {
		t.Errorf("List: want %d test capabilities in result, got %d", len(ids), found)
	}
}

// ---------------------------------------------------------------------------
// created_by round-trip (Phase 0B-1)
//
// Capability.CreatedBy is populated by the control-plane mapper from
// the apply actor (e.g. "operator:alice"). Prior to Phase 0B-1 the
// Postgres schema had no created_by column on capabilities, so the
// field was silently dropped on every persist — memory mode preserved
// it (struct-by-value), Postgres mode lost it. The fix added the
// column + the round-trip in Create / GetByID / List. These tests pin
// the round-trip and the empty-actor edge case so a regression in any
// of the three touched code paths surfaces here.
// ---------------------------------------------------------------------------

// TestCapabilityRepo_CreateGetByID_PreservesCreatedBy confirms that a
// Capability persisted with a non-empty CreatedBy reads back via
// GetByID with the same value.
func TestCapabilityRepo_CreateGetByID_PreservesCreatedBy(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	const id = "tst-cap-cb-getbyid"
	const actor = "operator:cap-cb-test"
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        id,
		Name:      "Capability with creator",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actor,
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}
	if got.CreatedBy != actor {
		t.Errorf("CreatedBy: want %q, got %q", actor, got.CreatedBy)
	}
}

// TestCapabilityRepo_List_PreservesCreatedBy confirms that
// CreatedBy survives the List() read path (separate SELECT/Scan call
// site from GetByID).
func TestCapabilityRepo_List_PreservesCreatedBy(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	const id = "tst-cap-cb-list"
	const actor = "operator:cap-cb-list-test"
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        id,
		Name:      "Capability with creator (list)",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actor,
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var foundActor string
	var foundRow bool
	for _, c := range all {
		if c.ID == id {
			foundRow = true
			foundActor = c.CreatedBy
			break
		}
	}
	if !foundRow {
		t.Fatalf("seeded capability %q not found in List() result", id)
	}
	if foundActor != actor {
		t.Errorf("CreatedBy via List: want %q, got %q", actor, foundActor)
	}
}

// TestCapabilityRepo_Create_AllowsEmptyCreatedBy confirms that
// persisting a Capability with an empty CreatedBy is accepted (the
// column is nullable; the repo writes NULL via sql.NullString) and
// that GetByID round-trips it as the empty string. This is the
// fixture / hand-rolled-INSERT path; the control-plane apply path
// always populates CreatedBy with an actor.
func TestCapabilityRepo_Create_AllowsEmptyCreatedBy(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	const id = "tst-cap-cb-empty"
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := &capability.Capability{
		ID:        id,
		Name:      "Capability without creator",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		// CreatedBy intentionally empty.
	}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("Create with empty CreatedBy: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}
	if got.CreatedBy != "" {
		t.Errorf("CreatedBy: want empty string, got %q", got.CreatedBy)
	}

	// Defence-in-depth: verify the column actually wrote NULL (rather
	// than the empty string) so the storage shape is honest about
	// "no actor" vs "actor was the empty string".
	var raw *string
	if err := db.QueryRowContext(ctx,
		`SELECT created_by FROM capabilities WHERE capability_id = $1`, id).Scan(&raw); err != nil {
		t.Fatalf("direct SQL probe: %v", err)
	}
	if raw != nil {
		t.Errorf("created_by column: want NULL when CreatedBy is empty, got %q", *raw)
	}
}

// ---------------------------------------------------------------------------
// external_ref round-trip (Phase 0B-2)
//
// Capability now carries the structured ExternalRef pattern that the
// Epic 1 PR 3 entities already use (BS, BSR, AISystem, AISystemVersion,
// AISystemBinding). The postgres repo writes/reads the five flat
// ext_* columns via the shared extRefSelectColumns / extRefScan /
// extRefInsertValues / mapExtRefError helpers; these tests reuse the
// shared pgExtRefFixture / assertExtRefRoundTrip / pgInconsistentExtRef
// helpers from external_ref_repo_test.go (same package).
// ---------------------------------------------------------------------------

// makeCapabilityWithExt is a small constructor that mirrors the
// makePGBSWithExt helper: produces a Capability struct with the given
// ExternalRef slot and otherwise minimum-valid lifecycle fields.
func makeCapabilityWithExt(id string, ref *externalref.ExternalRef) *capability.Capability {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &capability.Capability{
		ID:          id,
		Name:        id,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
		ExternalRef: ref,
	}
}

// TestCapabilityRepo_CreateGetByID_PreservesExternalRef confirms a
// Capability persisted with a populated ExternalRef reads back via
// GetByID with every field intact.
func TestCapabilityRepo_CreateGetByID_PreservesExternalRef(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const id = "tst-cap-ext-rt"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	if err := repo.Create(ctx, makeCapabilityWithExt(id, pgExtRefFixture())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}
	assertExtRefRoundTrip(t, got.ExternalRef)
}

// TestCapabilityRepo_List_PreservesExternalRef confirms ExternalRef
// also round-trips through List's separate SELECT/Scan path.
func TestCapabilityRepo_List_PreservesExternalRef(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const id = "tst-cap-ext-list"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	if err := repo.Create(ctx, makeCapabilityWithExt(id, pgExtRefFixture())); err != nil {
		t.Fatalf("Create: %v", err)
	}
	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, c := range all {
		if c.ID != id {
			continue
		}
		found = true
		assertExtRefRoundTrip(t, c.ExternalRef)
		break
	}
	if !found {
		t.Fatalf("seeded capability %q not found in List() result", id)
	}
}

// TestCapabilityRepo_Create_AllowsNilExternalRef confirms a Capability
// persisted with a nil ExternalRef writes the five ext_* columns as
// NULL and reads back with ExternalRef == nil. This is the canonical
// "no external reference" state and the most common case.
func TestCapabilityRepo_Create_AllowsNilExternalRef(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const id = "tst-cap-ext-nil"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	if err := repo.Create(ctx, makeCapabilityWithExt(id, nil)); err != nil {
		t.Fatalf("Create with nil ExternalRef: %v", err)
	}
	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected capability, got nil")
	}
	if got.ExternalRef != nil {
		t.Errorf("expected nil ExternalRef from NULL columns; got %+v", got.ExternalRef)
	}
}

// TestCapabilityRepo_ExternalRef_RejectsInconsistent_ViaCheckConstraint
// confirms that the Postgres CHECK constraint
// chk_capabilities_ext_consistency rejects a Capability whose
// ExternalRef has source_system set but source_id empty (or vice
// versa), and that the constraint violation surfaces as the typed
// externalref.ErrInconsistent sentinel via mapExtRefError. Mirrors
// the BS/BSR/AISystem/AISystemVersion/AISystemBinding equivalents.
func TestCapabilityRepo_ExternalRef_RejectsInconsistent_ViaCheckConstraint(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const id = "tst-cap-ext-bad"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	err = repo.Create(ctx, makeCapabilityWithExt(id, pgInconsistentExtRef()))
	if !errors.Is(err, externalref.ErrInconsistent) {
		t.Errorf("Create with inconsistent ExternalRef: want ErrInconsistent, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListByParentCapabilityID (Phase 2)
//
// Direct-children lookup against postgres. Index-backed by
// idx_capabilities_parent_capability_id (existing in schema.sql) —
// no schema change required. Mirrors the memory-repo contract:
// returns direct children only (no recursive descendants), ordered
// by capability_id ascending, non-nil empty slice on no-match,
// empty parentID short-circuits.
// ---------------------------------------------------------------------------

// seedCapPG inserts a capability with the given ID and parent. Mirrors
// the memory repo's seedCap helper. Caller is responsible for cleanup.
func seedCapPG(t *testing.T, repo *CapabilityRepo, id, parentID string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := repo.Create(context.Background(), &capability.Capability{
		ID:                 id,
		Name:               id,
		Status:             "active",
		Origin:             "manual",
		Managed:            true,
		ParentCapabilityID: parentID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Create %s: %v", id, err)
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_Empty(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = 'tst-cap-lbp-leaf'`)
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	// Lookup against a non-existent parent — non-nil empty slice.
	out, err := repo.ListByParentCapabilityID(ctx, "tst-cap-lbp-no-such-parent")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(out) != 0 {
		t.Errorf("expected 0 children for non-existent parent, got %d", len(out))
	}

	// Populated parent that has no children.
	seedCapPG(t, repo, "tst-cap-lbp-leaf", "")
	out2, err := repo.ListByParentCapabilityID(ctx, "tst-cap-lbp-leaf")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(leaf): %v", err)
	}
	if out2 == nil || len(out2) != 0 {
		t.Errorf("want non-nil empty slice for leaf parent, got %v", out2)
	}

	// Empty parentID short-circuits.
	out3, err := repo.ListByParentCapabilityID(ctx, "")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(\"\"): %v", err)
	}
	if out3 == nil || len(out3) != 0 {
		t.Errorf("empty parentID must short-circuit to non-nil empty slice; got %v", out3)
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_FiltersByParent(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	ids := []string{
		"tst-cap-lbp-fp-parent-a",
		"tst-cap-lbp-fp-parent-b",
		"tst-cap-lbp-fp-other-root",
		"tst-cap-lbp-fp-a-child-1",
		"tst-cap-lbp-fp-a-child-2",
		"tst-cap-lbp-fp-b-child-1",
	}
	t.Cleanup(func() {
		// Children before parents (FK order).
		for _, id := range []string{
			"tst-cap-lbp-fp-a-child-1", "tst-cap-lbp-fp-a-child-2", "tst-cap-lbp-fp-b-child-1",
			"tst-cap-lbp-fp-parent-a", "tst-cap-lbp-fp-parent-b", "tst-cap-lbp-fp-other-root",
		} {
			_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
		}
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	seedCapPG(t, repo, ids[0], "")     // parent-a (root)
	seedCapPG(t, repo, ids[1], "")     // parent-b (root)
	seedCapPG(t, repo, ids[2], "")     // other-root (root)
	seedCapPG(t, repo, ids[3], ids[0]) // a-child-1
	seedCapPG(t, repo, ids[4], ids[0]) // a-child-2
	seedCapPG(t, repo, ids[5], ids[1]) // b-child-1

	got, err := repo.ListByParentCapabilityID(ctx, ids[0])
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(parent-a): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 children of parent-a, got %d", len(got))
	}
	for _, c := range got {
		if c.ParentCapabilityID != ids[0] {
			t.Errorf("filter leaked: %s has parent_capability_id=%q", c.ID, c.ParentCapabilityID)
		}
	}

	gotB, err := repo.ListByParentCapabilityID(ctx, ids[1])
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(parent-b): %v", err)
	}
	if len(gotB) != 1 || gotB[0].ID != ids[5] {
		t.Errorf("want [%s] for parent-b, got %+v", ids[5], gotB)
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_DirectChildrenOnly(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	t.Cleanup(func() {
		// Children before parents.
		for _, id := range []string{"tst-cap-lbp-dco-leaf", "tst-cap-lbp-dco-mid", "tst-cap-lbp-dco-top"} {
			_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
		}
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	seedCapPG(t, repo, "tst-cap-lbp-dco-top", "")
	seedCapPG(t, repo, "tst-cap-lbp-dco-mid", "tst-cap-lbp-dco-top")
	seedCapPG(t, repo, "tst-cap-lbp-dco-leaf", "tst-cap-lbp-dco-mid")

	got, err := repo.ListByParentCapabilityID(ctx, "tst-cap-lbp-dco-top")
	if err != nil {
		t.Fatalf("ListByParentCapabilityID(top): %v", err)
	}
	if len(got) != 1 || got[0].ID != "tst-cap-lbp-dco-mid" {
		t.Fatalf("want only direct child mid, got %+v", got)
	}
	for _, c := range got {
		if c.ID == "tst-cap-lbp-dco-leaf" {
			t.Error("recursive descendant leaf must NOT be returned by ListByParentCapabilityID(top)")
		}
	}
}

func TestCapabilityRepo_ListByParentCapabilityID_OrdersByID(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	ids := []string{
		"tst-cap-lbp-ord-parent",
		"tst-cap-lbp-ord-z-child",
		"tst-cap-lbp-ord-m-child",
		"tst-cap-lbp-ord-a-child",
	}
	t.Cleanup(func() {
		// Children first (FK).
		for _, id := range []string{ids[1], ids[2], ids[3], ids[0]} {
			_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
		}
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}
	seedCapPG(t, repo, ids[0], "")
	// Insert deliberately out of lexical order.
	seedCapPG(t, repo, ids[1], ids[0])
	seedCapPG(t, repo, ids[2], ids[0])
	seedCapPG(t, repo, ids[3], ids[0])

	got, err := repo.ListByParentCapabilityID(ctx, ids[0])
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	want := []string{ids[3], ids[2], ids[1]} // a-child, m-child, z-child
	if len(got) != len(want) {
		t.Fatalf("want %d children, got %d", len(want), len(got))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("got[%d] = %q, want %q (full=%v)", i, got[i].ID, id, got)
		}
	}
}

// TestCapabilityRepo_ListByParentCapabilityID_PreservesFullCapabilityFields
// proves the new SELECT projects every wire-relevant field — including
// the post-Phase-0B-1 created_by and the post-Phase-0B-2 ext_* columns
// — and that the structured ExternalRef materialises correctly through
// extRefScan.ToExternalRef. A regression that copy-pasted only the
// pre-0B-1 column set would silently drop CreatedBy + ExternalRef from
// the new method's results.
func TestCapabilityRepo_ListByParentCapabilityID_PreservesFullCapabilityFields(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const (
		parentID = "tst-cap-lbp-full-parent"
		childID  = "tst-cap-lbp-full-child"
	)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, childID)
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, parentID)
	})

	repo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	const replacedID = "tst-cap-lbp-full-replaced"
	t.Cleanup(func() {
		// fk_capabilities_replaces is enforced so we must delete the
		// child BEFORE the row it replaces. The cleanup defined for
		// parent/child above runs in registration order; this final
		// cleanup runs first (LIFO), so the deletes here precede them.
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, replacedID)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	// Parent must exist before the child's parent_capability_id FK is
	// validated; the replaced row must exist before the child's
	// replaces FK is validated. Seed both before the child INSERT.
	if err := repo.Create(ctx, &capability.Capability{
		ID: parentID, Name: parentID, Status: "active", Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	if err := repo.Create(ctx, &capability.Capability{
		ID: replacedID, Name: "replaced", Status: "deprecated",
		Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create replaced: %v", err)
	}

	// Child carries the full field set. ExternalRef matches
	// pgExtRefFixture so the shared assertExtRefRoundTrip helper can
	// be reused here.
	if err := repo.Create(ctx, &capability.Capability{
		ID:                 childID,
		Name:               "child name",
		Description:        "child description",
		Status:             "active",
		Origin:             "manual",
		Managed:            true,
		Replaces:           replacedID,
		Owner:              "team-platform",
		ParentCapabilityID: parentID,
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          "operator:lbp-full",
		ExternalRef:        pgExtRefFixture(),
	}); err != nil {
		t.Fatalf("Create child: %v", err)
	}

	got, err := repo.ListByParentCapabilityID(ctx, parentID)
	if err != nil {
		t.Fatalf("ListByParentCapabilityID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 child, got %d", len(got))
	}
	c := got[0]

	// Field-by-field assertions for the full wire-relevant set.
	if c.ID != childID {
		t.Errorf("ID: want %q, got %q", childID, c.ID)
	}
	if c.Name != "child name" {
		t.Errorf("Name: want %q, got %q", "child name", c.Name)
	}
	if c.Description != "child description" {
		t.Errorf("Description: want %q, got %q", "child description", c.Description)
	}
	if c.Status != "active" {
		t.Errorf("Status: want active, got %q", c.Status)
	}
	if c.Owner != "team-platform" {
		t.Errorf("Owner: want team-platform, got %q", c.Owner)
	}
	if c.Origin != "manual" {
		t.Errorf("Origin: want manual, got %q", c.Origin)
	}
	if !c.Managed {
		t.Error("Managed: want true, got false")
	}
	if c.Replaces != replacedID {
		t.Errorf("Replaces: want %q, got %q", replacedID, c.Replaces)
	}
	if c.ParentCapabilityID != parentID {
		t.Errorf("ParentCapabilityID: want %q, got %q", parentID, c.ParentCapabilityID)
	}
	if c.CreatedBy != "operator:lbp-full" {
		t.Errorf("CreatedBy: want operator:lbp-full, got %q", c.CreatedBy)
	}
	assertExtRefRoundTrip(t, c.ExternalRef)
}
