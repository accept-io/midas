package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
)

// businessservice_failmode_policy_test.go — round-trip regression for
// the optional BusinessService.FailModePolicyID default
// (D27j-impl-2). cloneBusinessService struct-copies the field, so the
// test confirms reads observe the field with both populated and
// unpopulated values.

func bsForFailModeTest(id string) *businessservice.BusinessService {
	now := time.Now().UTC()
	return &businessservice.BusinessService{
		ID:          id,
		Name:        id,
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestBusinessServiceRepo_FailModePolicyID_RoundTrip(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()

	bs := bsForFailModeTest("bs-with-fmp")
	bs.FailModePolicyID = "fmp-default"
	if err := r.Create(ctx, bs); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.GetByID(ctx, bs.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("business service not found")
	}
	if got.FailModePolicyID != "fmp-default" {
		t.Errorf("FailModePolicyID round-trip: want %q, got %q", "fmp-default", got.FailModePolicyID)
	}
}

func TestBusinessServiceRepo_FailModePolicyID_EmptyByDefault(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()

	bs := bsForFailModeTest("bs-no-fmp")
	if err := r.Create(ctx, bs); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := r.GetByID(ctx, bs.ID)
	if got == nil {
		t.Fatal("business service not found")
	}
	if got.FailModePolicyID != "" {
		t.Errorf("FailModePolicyID must be empty by default; got %q", got.FailModePolicyID)
	}
}

func TestBusinessServiceRepo_FailModePolicyID_UpdatePersists(t *testing.T) {
	r := NewBusinessServiceRepo()
	ctx := context.Background()

	bs := bsForFailModeTest("bs-fmp-update")
	if err := r.Create(ctx, bs); err != nil {
		t.Fatalf("Create: %v", err)
	}

	bs.FailModePolicyID = "fmp-renamed"
	if err := r.Update(ctx, bs); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := r.GetByID(ctx, bs.ID)
	if got == nil || got.FailModePolicyID != "fmp-renamed" {
		t.Errorf("Update must persist FailModePolicyID change; got %+v", got)
	}
}
