package postgres

// Shared helpers for marshalling ExternalRef into the five flat
// `ext_*` columns added to consuming tables in Epic 1 PR 3.
//
// Five entities reuse this helper:
//   - business_services
//   - business_service_relationships
//   - ai_systems
//   - ai_system_versions
//   - ai_system_bindings
//
// Each consuming repo's INSERT/UPDATE/SELECT splices in the same five
// columns at the end of its existing column list, then calls
// extRefInsertValues / scanExternalRef to bind / unmarshal.
//
// Convention (SELECT): COALESCE the four TEXT columns to '' and scan
// directly to string scratch fields; use sql.NullTime for the
// timestamp. Mirrors the AISystemVersion repo's NULL-handling pattern
// (PR 2). The BSR repo's older sql.NullString + Valid-check pattern is
// the convention drift outlier; when BSR next changes for unrelated
// reasons it should migrate to this convention.

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/accept-io/midas/internal/externalref"
)

// extRefSelectColumns is the canonical SELECT projection fragment for
// the five ext_* columns on any consuming table. Splice into the end
// of an existing SELECT column list separated by ", ".
const extRefSelectColumns = `COALESCE(ext_source_system, ''), COALESCE(ext_source_id, ''), COALESCE(ext_source_url, ''), COALESCE(ext_source_version, ''), ext_last_synced_at`

// extRefScan holds the scratch destinations for scanning the five
// ext_* columns. Each consuming Scan call appends the result of Dests()
// to its argument list, then calls ToExternalRef to materialise the
// domain pointer.
type extRefScan struct {
	SourceSystem  string
	SourceID      string
	SourceURL     string
	SourceVersion string
	LastSyncedAt  sql.NullTime
}

// Dests returns the scan destinations in the order extRefSelectColumns
// produces.
func (e *extRefScan) Dests() []any {
	return []any{&e.SourceSystem, &e.SourceID, &e.SourceURL, &e.SourceVersion, &e.LastSyncedAt}
}

// ToExternalRef materialises a *ExternalRef from the scanned scratch.
// Returns nil when every column was NULL (or empty after COALESCE) —
// the canonical "no external reference" state on the wire matches the
// canonical nil at the domain layer.
func (e *extRefScan) ToExternalRef() *externalref.ExternalRef {
	if e.SourceSystem == "" &&
		e.SourceID == "" &&
		e.SourceURL == "" &&
		e.SourceVersion == "" &&
		!e.LastSyncedAt.Valid {
		return nil
	}
	out := &externalref.ExternalRef{
		SourceSystem:  e.SourceSystem,
		SourceID:      e.SourceID,
		SourceURL:     e.SourceURL,
		SourceVersion: e.SourceVersion,
	}
	if e.LastSyncedAt.Valid {
		t := e.LastSyncedAt.Time
		out.LastSyncedAt = &t
	}
	return out
}

// extRefInsertValues returns the slice of bind values for the five
// ext_* columns in INSERT/UPDATE statements. Splice at the end of the
// existing argument list.
//
// IsZero refs canonicalise to (NULL, NULL, NULL, NULL, NULL) on the
// wire, matching the schema's CHECK invariant that source_system and
// source_id must either both be NULL or both be NOT NULL.
func extRefInsertValues(r *externalref.ExternalRef) []any {
	if r.IsZero() {
		return []any{nil, nil, nil, nil, nil}
	}
	return []any{
		nullableString(r.SourceSystem),
		nullableString(r.SourceID),
		nullableString(r.SourceURL),
		nullableString(r.SourceVersion),
		nullableTime(r.LastSyncedAt),
	}
}

// extRefConsistencyConstraints lists every chk_<table>_ext_consistency
// constraint name. mapExtRefError uses this to translate a pq.Error
// CHECK violation (code 23514) on any of these constraints to the
// externalref.ErrInconsistent sentinel.
//
// The five names are spelled out so a renamed constraint surfaces as a
// missed entry here rather than a silently swallowed wrap.
var extRefConsistencyConstraints = map[string]struct{}{
	"chk_business_services_ext_consistency":              {},
	"chk_business_service_relationships_ext_consistency": {},
	"chk_ai_systems_ext_consistency":                     {},
	"chk_ai_system_versions_ext_consistency":             {},
	"chk_ai_system_bindings_ext_consistency":             {},
}

// mapExtRefError translates a CHECK-violation on the consistency
// constraint to externalref.ErrInconsistent. Returns the input
// unchanged for any other error or nil.
//
// Consuming repos invoke this from their existing pq.Error switch (or
// alongside it) so the inconsistency is reported through a typed
// sentinel rather than the raw constraint message.
func mapExtRefError(err error) error {
	if err == nil {
		return nil
	}
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return err
	}
	if pqErr.Code != "23514" {
		return err
	}
	if _, ok := extRefConsistencyConstraints[pqErr.Constraint]; ok {
		return fmt.Errorf("%w: %s", externalref.ErrInconsistent, pqErr.Detail)
	}
	return err
}
