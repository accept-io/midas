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
