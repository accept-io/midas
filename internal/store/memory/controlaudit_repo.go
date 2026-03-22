package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/accept-io/midas/internal/controlaudit"
)

// ControlAuditRepo is a thread-safe in-memory implementation of
// controlaudit.Repository. It is intended for tests only.
type ControlAuditRepo struct {
	mu      sync.RWMutex
	records []*controlaudit.ControlAuditRecord
}

// NewControlAuditRepo constructs an empty ControlAuditRepo.
func NewControlAuditRepo() *ControlAuditRepo {
	return &ControlAuditRepo{}
}

// Append appends an audit record. A defensive copy is stored so callers cannot
// mutate the record after appending.
func (r *ControlAuditRepo) Append(_ context.Context, rec *controlaudit.ControlAuditRecord) error {
	if rec == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	cp := *rec
	if rec.ResourceVersion != nil {
		v := *rec.ResourceVersion
		cp.ResourceVersion = &v
	}
	if rec.Metadata != nil {
		m := *rec.Metadata
		cp.Metadata = &m
	}
	r.records = append(r.records, &cp)
	return nil
}

// List returns records matching the filter in descending occurred_at order.
// A zero-value filter returns all records up to the effective limit.
func (r *ControlAuditRepo) List(_ context.Context, f controlaudit.ListFilter) ([]*controlaudit.ControlAuditRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := effectiveLimit(f.Limit)

	// Collect matching records in insertion order, then sort newest-first.
	var matched []*controlaudit.ControlAuditRecord
	for _, rec := range r.records {
		if !matchesFilter(rec, f) {
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

func matchesFilter(rec *controlaudit.ControlAuditRecord, f controlaudit.ListFilter) bool {
	if f.ResourceKind != "" && rec.ResourceKind != f.ResourceKind {
		return false
	}
	if f.ResourceID != "" && rec.ResourceID != f.ResourceID {
		return false
	}
	if f.Actor != "" && rec.Actor != f.Actor {
		return false
	}
	if f.Action != "" && rec.Action != f.Action {
		return false
	}
	return true
}

func effectiveLimit(requested int) int {
	if requested <= 0 {
		return controlaudit.DefaultListLimit
	}
	if requested > controlaudit.MaxListLimit {
		return controlaudit.MaxListLimit
	}
	return requested
}

var _ controlaudit.Repository = (*ControlAuditRepo)(nil)
