package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
)

func newTestBinding(id, sysID string, now time.Time) *aisystem.AISystemBinding {
	return &aisystem.AISystemBinding{
		ID:                id,
		AISystemID:        sysID,
		BusinessServiceID: "bs-x",
		CreatedAt:         now,
	}
}

func TestAIBindingRepo_Create_RoundTrip(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if err := r.Create(ctx, newTestBinding("b1", "ai-1", now)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := r.GetByID(ctx, "b1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != "b1" || got.AISystemID != "ai-1" || got.BusinessServiceID != "bs-x" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestAIBindingRepo_Create_DuplicateID(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestBinding("dup", "ai-1", now)); err != nil {
		t.Fatal(err)
	}
	err := r.Create(ctx, newTestBinding("dup", "ai-1", now))
	if !errors.Is(err, aisystem.ErrAISystemBindingAlreadyExists) {
		t.Errorf("want ErrAISystemBindingAlreadyExists, got %v", err)
	}
}

func TestAIBindingRepo_Create_RejectsNoContextReference(t *testing.T) {
	r := NewAISystemBindingRepo()
	now := time.Now().UTC()
	b := &aisystem.AISystemBinding{
		ID:         "no-ctx",
		AISystemID: "ai-1",
		CreatedAt:  now,
	}
	err := r.Create(context.Background(), b)
	if !errors.Is(err, aisystem.ErrBindingMissingContext) {
		t.Errorf("want ErrBindingMissingContext, got %v", err)
	}
}

func TestAIBindingRepo_Create_AcceptsAnyContextSubset(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	cases := []struct {
		id    string
		field func(*aisystem.AISystemBinding)
	}{
		{"only-bs", func(b *aisystem.AISystemBinding) { b.BusinessServiceID = "bs-1" }},
		{"only-cap", func(b *aisystem.AISystemBinding) { b.CapabilityID = "cap-1" }},
		{"only-proc", func(b *aisystem.AISystemBinding) { b.ProcessID = "proc-1" }},
		{"only-surf", func(b *aisystem.AISystemBinding) { b.SurfaceID = "surf-1" }},
	}
	for _, tc := range cases {
		b := &aisystem.AISystemBinding{
			ID:         tc.id,
			AISystemID: "ai-1",
			CreatedAt:  now,
		}
		tc.field(b)
		if err := r.Create(ctx, b); err != nil {
			t.Errorf("Create %s: unexpected error %v", tc.id, err)
		}
	}
}

func TestAIBindingRepo_Create_RejectsMissingAISystemFK(t *testing.T) {
	sysRepo := NewAISystemRepo()
	r := NewAISystemBindingRepo()
	r.aiSystems = sysRepo

	now := time.Now().UTC()
	err := r.Create(context.Background(), newTestBinding("b1", "ghost", now))
	if !errors.Is(err, aisystem.ErrAISystemNotFound) {
		t.Errorf("want ErrAISystemNotFound, got %v", err)
	}
}

func TestAIBindingRepo_GetByID_NotFound(t *testing.T) {
	r := NewAISystemBindingRepo()
	_, err := r.GetByID(context.Background(), "absent")
	if !errors.Is(err, aisystem.ErrAISystemBindingNotFound) {
		t.Errorf("want ErrAISystemBindingNotFound, got %v", err)
	}
}

func TestAIBindingRepo_ListByAISystem(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Millisecond)

	for i, id := range []string{"a-old", "a-mid", "a-new"} {
		b := newTestBinding(id, "ai-1", base.Add(time.Duration(i)*time.Hour))
		if err := r.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Create(ctx, newTestBinding("other", "ai-2", base)); err != nil {
		t.Fatal(err)
	}
	got, err := r.ListByAISystem(ctx, "ai-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "a-new" || got[2].ID != "a-old" {
		t.Errorf("ListByAISystem ordering off; got %v", bindingIDs(got))
	}
}

func TestAIBindingRepo_ListByContextFields(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	bindings := []*aisystem.AISystemBinding{
		{ID: "by-bs", AISystemID: "ai-1", BusinessServiceID: "bs-1", CreatedAt: now},
		{ID: "by-cap", AISystemID: "ai-1", CapabilityID: "cap-1", CreatedAt: now.Add(time.Second)},
		{ID: "by-proc", AISystemID: "ai-1", ProcessID: "proc-1", CreatedAt: now.Add(2 * time.Second)},
		{ID: "by-surf", AISystemID: "ai-1", SurfaceID: "surf-1", CreatedAt: now.Add(3 * time.Second)},
	}
	for _, b := range bindings {
		if err := r.Create(ctx, b); err != nil {
			t.Fatal(err)
		}
	}

	checks := []struct {
		fn   func() ([]*aisystem.AISystemBinding, error)
		want string
	}{
		{func() ([]*aisystem.AISystemBinding, error) { return r.ListByBusinessService(ctx, "bs-1") }, "by-bs"},
		{func() ([]*aisystem.AISystemBinding, error) { return r.ListByCapability(ctx, "cap-1") }, "by-cap"},
		{func() ([]*aisystem.AISystemBinding, error) { return r.ListByProcess(ctx, "proc-1") }, "by-proc"},
		{func() ([]*aisystem.AISystemBinding, error) { return r.ListBySurface(ctx, "surf-1") }, "by-surf"},
	}
	for _, c := range checks {
		got, err := c.fn()
		if err != nil {
			t.Errorf("list call: %v", err)
			continue
		}
		if len(got) != 1 || got[0].ID != c.want {
			t.Errorf("expected single result %q, got %v", c.want, bindingIDs(got))
		}
	}
}

func TestAIBindingRepo_Delete(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := r.Create(ctx, newTestBinding("d1", "ai-1", now)); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, "d1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := r.Delete(ctx, "d1"); !errors.Is(err, aisystem.ErrAISystemBindingNotFound) {
		t.Errorf("Delete (idempotent): want ErrAISystemBindingNotFound, got %v", err)
	}
}

func TestAIBindingRepo_DefensiveCopy(t *testing.T) {
	r := NewAISystemBindingRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	ver := 7
	b := &aisystem.AISystemBinding{
		ID:                "defensive",
		AISystemID:        "ai-1",
		AISystemVersion:   &ver,
		BusinessServiceID: "bs-1",
		CreatedAt:         now,
	}
	if err := r.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	*b.AISystemVersion = 999

	got, _ := r.GetByID(ctx, "defensive")
	if got.AISystemVersion == nil || *got.AISystemVersion != 7 {
		t.Errorf("memory repo leaked pointer mutation: want version=7, got %v", got.AISystemVersion)
	}
}

func TestAIBindingRepo_FKErrorMessage(t *testing.T) {
	sysRepo := NewAISystemRepo()
	if err := sysRepo.Create(context.Background(), newTestAISystem("ai-1", time.Now())); err != nil {
		t.Fatal(err)
	}
	r := NewAISystemBindingRepo()
	r.aiSystems = sysRepo

	now := time.Now().UTC()
	b := &aisystem.AISystemBinding{
		ID:                "bad-bs",
		AISystemID:        "ai-1",
		BusinessServiceID: "bs-ghost",
		CreatedAt:         now,
	}
	// bsvcs validator not wired, so this should succeed; smoke test the
	// happy path against the wired aiSystems validator.
	if err := r.Create(context.Background(), b); err != nil {
		t.Fatalf("Create with wired aiSystems but no bsvcs validator: %v", err)
	}

	// Now wire a bsvcs validator with no rows; missing FK should surface.
	r2 := NewAISystemBindingRepo()
	r2.aiSystems = sysRepo
	r2.bsvcs = NewBusinessServiceRepo()
	b2 := &aisystem.AISystemBinding{
		ID:                "bad-bs-2",
		AISystemID:        "ai-1",
		BusinessServiceID: "bs-ghost",
		CreatedAt:         now,
	}
	err := r2.Create(context.Background(), b2)
	if err == nil {
		t.Fatal("want FK error, got nil")
	}
	if !strings.Contains(err.Error(), "business_service") || !strings.Contains(err.Error(), "bs-ghost") {
		t.Errorf("FK error message lacks context; got %q", err.Error())
	}
}

func bindingIDs(bs []*aisystem.AISystemBinding) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.ID
	}
	return out
}
