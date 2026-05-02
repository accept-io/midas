// Package externalref defines the shared ExternalRef value object used
// to attach structured metadata about external systems (ServiceNow,
// LeanIX, GitHub, internal catalogues, custom labels) to MIDAS entities
// without coupling MIDAS to any connector.
//
// MIDAS is the source of truth. ExternalRef records what an external
// system asserted about an entity at a point in time; MIDAS never
// validates the SourceSystem against an enum and never resolves the
// reference by calling the external system. Connectors that produce
// these values live out-of-tree.
//
// PR 3 (Epic 1) introduces this type and attaches it as an optional
// field on five entities: BusinessService, BusinessServiceRelationship
// (PR 1), AISystem, AISystemVersion, and AISystemBinding (PR 2).
//
// Storage shape: five flat columns per consuming table, all nullable,
// prefixed `ext_`. A schema-level CHECK enforces the consistency rule
// that SourceSystem and SourceID must either both be set or both be
// NULL — see the Validate method below for the corresponding
// application-layer enforcement.
package externalref

import (
	"errors"
	"strings"
	"time"
)

// ErrInconsistent is returned by Validate when the consistency rule is
// violated: SourceSystem and SourceID must either both be non-empty or
// both be empty. Mirrors the chk_<table>_ext_consistency CHECK in the
// Postgres schema.
var ErrInconsistent = errors.New("external_ref: source_system and source_id must both be set or both be empty")

// ExternalRef is a structured reference to a record in an external
// system. All fields are optional except for the consistency rule
// enforced by Validate.
//
// Fields:
//
//   - SourceSystem  — caller-asserted label (e.g. "servicenow",
//     "leanix", "github", "custom"). MIDAS treats this as opaque.
//   - SourceID      — identifier in the source system.
//   - SourceURL     — optional deep link.
//   - SourceVersion — optional version label assigned by the source.
//   - LastSyncedAt  — optional timestamp the source last produced this
//     data. Stored in UTC.
type ExternalRef struct {
	SourceSystem  string
	SourceID      string
	SourceURL     string
	SourceVersion string
	LastSyncedAt  *time.Time
}

// IsZero reports whether the ExternalRef has no meaningful content.
// Used by callers to canonicalise empty-but-non-nil pointers to nil
// at the storage boundary, preventing mixed in-memory representations.
//
// A nil receiver is treated as zero. A non-nil receiver is zero when
// every field is empty / nil after the consistency rule is satisfied.
func (r *ExternalRef) IsZero() bool {
	if r == nil {
		return true
	}
	return r.SourceSystem == "" &&
		r.SourceID == "" &&
		r.SourceURL == "" &&
		r.SourceVersion == "" &&
		r.LastSyncedAt == nil
}

// Validate enforces the consistency rule: SourceSystem and SourceID
// must either both be non-empty or both be empty.
//
// A nil receiver is valid (no external reference). Whitespace-only
// values are treated as empty for the purposes of the rule, so a
// caller cannot accidentally satisfy the rule by submitting "  " for
// SourceID — the mapper trims at the document boundary, and Validate
// is consistent with that behaviour.
//
// Other fields (SourceURL, SourceVersion, LastSyncedAt) are
// independently optional and never trigger ErrInconsistent.
func (r *ExternalRef) Validate() error {
	if r == nil {
		return nil
	}
	system := strings.TrimSpace(r.SourceSystem)
	id := strings.TrimSpace(r.SourceID)
	if (system == "") != (id == "") {
		return ErrInconsistent
	}
	return nil
}

// Clone returns a deep copy of r. The LastSyncedAt pointer is
// independently allocated so that mutating either copy's timestamp
// does not affect the other.
//
// Returns nil when r is nil. Used by memory repositories to defend
// against caller mutation of stored values.
func (r *ExternalRef) Clone() *ExternalRef {
	if r == nil {
		return nil
	}
	cp := *r
	if r.LastSyncedAt != nil {
		t := *r.LastSyncedAt
		cp.LastSyncedAt = &t
	}
	return &cp
}

// Canonicalise returns nil when r is nil or IsZero, otherwise r
// itself. Used by storage paths to enforce a single representation
// of "no external reference" so equality checks and round-trip
// assertions are unambiguous.
func Canonicalise(r *ExternalRef) *ExternalRef {
	if r.IsZero() {
		return nil
	}
	return r
}

// Equal reports whether two ExternalRef values are semantically equal.
//
// Equality posture:
//
//   - nil == nil
//   - nil == IsZero (an empty-but-non-nil pointer is equivalent to nil)
//   - IsZero == nil (symmetric)
//   - non-zero values compare field-by-field; LastSyncedAt comparison
//     uses time.Time.Equal so wall-clock-equal but monotonic-clock-
//     different timestamps are still considered equal
//
// Equal is forward-looking infrastructure for the future
// ApplyActionUpdate framework variant. The current Conflict-on-existing
// planners do not consult it (they short-circuit on ID existence), so
// Equal does not affect today's apply behaviour. When ApplyActionUpdate
// lands, the Update planner can use Equal to decide NoOp-vs-Update for
// each entity that carries an ExternalRef field.
func Equal(a, b *ExternalRef) bool {
	aZero := a.IsZero()
	bZero := b.IsZero()
	if aZero && bZero {
		return true
	}
	if aZero != bZero {
		return false
	}
	// Both non-nil and non-zero from here.
	if a.SourceSystem != b.SourceSystem ||
		a.SourceID != b.SourceID ||
		a.SourceURL != b.SourceURL ||
		a.SourceVersion != b.SourceVersion {
		return false
	}
	if (a.LastSyncedAt == nil) != (b.LastSyncedAt == nil) {
		return false
	}
	if a.LastSyncedAt != nil && !a.LastSyncedAt.Equal(*b.LastSyncedAt) {
		return false
	}
	return true
}
