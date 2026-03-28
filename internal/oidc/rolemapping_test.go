package oidc

import (
	"reflect"
	"testing"

	"github.com/accept-io/midas/internal/identity"
)

func TestMapExternalRoles_BasicMapping(t *testing.T) {
	mappings := []RoleMapping{
		{External: "midas-admins", Internal: "admin"},
		{External: "midas-operators", Internal: "operator"},
	}
	got := MapExternalRoles([]string{"midas-admins", "midas-operators"}, mappings)
	want := []string{identity.RolePlatformAdmin, identity.RolePlatformOperator}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMapExternalRoles_UnknownGroupsIgnored(t *testing.T) {
	mappings := []RoleMapping{
		{External: "midas-admins", Internal: identity.RolePlatformAdmin},
	}
	got := MapExternalRoles([]string{"some-other-group", "midas-admins"}, mappings)
	want := []string{identity.RolePlatformAdmin}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMapExternalRoles_EmptyGroups_ReturnsEmpty(t *testing.T) {
	mappings := []RoleMapping{
		{External: "midas-admins", Internal: identity.RolePlatformAdmin},
	}
	got := MapExternalRoles(nil, mappings)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestMapExternalRoles_EmptyMappings_ReturnsEmpty(t *testing.T) {
	got := MapExternalRoles([]string{"midas-admins"}, nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestMapExternalRoles_Deduplication(t *testing.T) {
	// Two external groups mapping to the same internal role.
	mappings := []RoleMapping{
		{External: "midas-admins-eu", Internal: identity.RolePlatformAdmin},
		{External: "midas-admins-us", Internal: identity.RolePlatformAdmin},
	}
	got := MapExternalRoles([]string{"midas-admins-eu", "midas-admins-us"}, mappings)
	want := []string{identity.RolePlatformAdmin}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMapExternalRoles_MapsCanonicalThroughNormalize(t *testing.T) {
	// Internal value is a legacy role name; NormalizeRoles should canonicalize it.
	mappings := []RoleMapping{
		{External: "midas-approvers", Internal: "approver"},
	}
	got := MapExternalRoles([]string{"midas-approvers"}, mappings)
	want := []string{identity.RoleGovernanceApprover}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMapExternalRoles_DeterministicOrder(t *testing.T) {
	mappings := []RoleMapping{
		{External: "g-viewer", Internal: identity.RolePlatformViewer},
		{External: "g-admin", Internal: identity.RolePlatformAdmin},
		{External: "g-operator", Internal: identity.RolePlatformOperator},
	}
	// Run twice to confirm ordering is stable.
	a := MapExternalRoles([]string{"g-viewer", "g-admin", "g-operator"}, mappings)
	b := MapExternalRoles([]string{"g-operator", "g-viewer", "g-admin"}, mappings)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("non-deterministic: %v vs %v", a, b)
	}
}

func TestMapExternalRoles_ExactCaseMatch(t *testing.T) {
	// Group matching is exact (case-sensitive) — Entra returns exact names.
	mappings := []RoleMapping{
		{External: "MIDAS-Admins", Internal: identity.RolePlatformAdmin},
	}
	// Lower-cased group should NOT match.
	got := MapExternalRoles([]string{"midas-admins"}, mappings)
	if len(got) != 0 {
		t.Errorf("expected no match for wrong case, got %v", got)
	}
	// Exact case should match.
	got = MapExternalRoles([]string{"MIDAS-Admins"}, mappings)
	want := []string{identity.RolePlatformAdmin}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMapExternalRoles_GovernanceRoles(t *testing.T) {
	mappings := []RoleMapping{
		{External: "midas-approvers", Internal: identity.RoleGovernanceApprover},
		{External: "midas-reviewers", Internal: identity.RoleGovernanceReviewer},
	}
	got := MapExternalRoles([]string{"midas-approvers", "midas-reviewers"}, mappings)
	want := []string{identity.RoleGovernanceApprover, identity.RoleGovernanceReviewer}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
