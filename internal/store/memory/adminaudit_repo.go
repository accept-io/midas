package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/accept-io/midas/internal/adminaudit"
)

// AdminAuditRepo is a thread-safe in-memory implementation of
// adminaudit.Repository intended for tests and memory-store deployments.
type AdminAuditRepo struct {
	mu      sync.RWMutex
	records []*adminaudit.AdminAuditRecord
}

func NewAdminAuditRepo() *AdminAuditRepo {
	return &AdminAuditRepo{}
}

// Append stores a defensive copy so callers cannot mutate persisted records.
func (r *AdminAuditRepo) Append(_ context.Context, rec *adminaudit.AdminAuditRecord) error {
	if rec == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *rec
	if rec.Details != nil {
		d := *rec.Details
		if len(rec.Details.ProcessesDeleted) > 0 {
			d.ProcessesDeleted = append([]string(nil), rec.Details.ProcessesDeleted...)
		}
		if len(rec.Details.CapabilitiesDeleted) > 0 {
			d.CapabilitiesDeleted = append([]string(nil), rec.Details.CapabilitiesDeleted...)
		}
		cp.Details = &d
	}
	r.records = append(r.records, &cp)
	return nil
}

// List returns records matching the filter in descending occurred_at order.
func (r *AdminAuditRepo) List(_ context.Context, f adminaudit.ListFilter) ([]*adminaudit.AdminAuditRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := effectiveAdminAuditLimit(f.Limit)

	var matched []*adminaudit.AdminAuditRecord
	for _, rec := range r.records {
		if !matchesAdminAuditFilter(rec, f) {
			continue
		}
		cp := *rec
		matched = append(matched, &cp)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].OccurredAt.After(matched[j].OccurredAt)
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}

func matchesAdminAuditFilter(rec *adminaudit.AdminAuditRecord, f adminaudit.ListFilter) bool {
	if f.Action != "" && rec.Action != f.Action {
		return false
	}
	if f.Outcome != "" && rec.Outcome != f.Outcome {
		return false
	}
	if f.ActorID != "" && rec.ActorID != f.ActorID {
		return false
	}
	if f.TargetType != "" && rec.TargetType != f.TargetType {
		return false
	}
	if f.TargetID != "" && rec.TargetID != f.TargetID {
		return false
	}
	return true
}

func effectiveAdminAuditLimit(requested int) int {
	if requested <= 0 {
		return adminaudit.DefaultListLimit
	}
	if requested > adminaudit.MaxListLimit {
		return adminaudit.MaxListLimit
	}
	return requested
}

var _ adminaudit.Repository = (*AdminAuditRepo)(nil)
