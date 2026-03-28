package identity

import (
	"reflect"
	"testing"
)

func TestNormalizeRoles_LegacyToCanonical(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
	}{
		{[]string{"admin"}, []string{RolePlatformAdmin}},
		{[]string{"operator"}, []string{RolePlatformOperator}},
		{[]string{"approver"}, []string{RoleGovernanceApprover}},
		{[]string{"reviewer"}, []string{RoleGovernanceReviewer}},
		// All four legacy roles together
		{
			[]string{"admin", "operator", "approver", "reviewer"},
			[]string{RoleGovernanceApprover, RoleGovernanceReviewer, RolePlatformAdmin, RolePlatformOperator},
		},
	}

	for _, tc := range tests {
		got := NormalizeRoles(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("NormalizeRoles(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRoles_CanonicalPassThrough(t *testing.T) {
	in := []string{RolePlatformAdmin, RolePlatformOperator, RolePlatformViewer, RoleGovernanceApprover, RoleGovernanceReviewer}
	got := NormalizeRoles(in)
	want := []string{RoleGovernanceApprover, RoleGovernanceReviewer, RolePlatformAdmin, RolePlatformOperator, RolePlatformViewer}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeRoles(canonical) = %v, want %v", got, want)
	}
}

func TestNormalizeRoles_UnknownPreserved(t *testing.T) {
	in := []string{"some_custom_role", "another"}
	got := NormalizeRoles(in)
	want := []string{"another", "some_custom_role"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeRoles(unknown) = %v, want %v", got, want)
	}
}

func TestNormalizeRoles_Deduplication(t *testing.T) {
	// "admin" and "ADMIN" both normalize to platform.admin — only one entry.
	in := []string{"admin", "ADMIN", "admin"}
	got := NormalizeRoles(in)
	if len(got) != 1 || got[0] != RolePlatformAdmin {
		t.Errorf("NormalizeRoles dedup: got %v, want [%s]", got, RolePlatformAdmin)
	}
}

func TestNormalizeRoles_CaseInsensitiveLegacyLookup(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ADMIN", RolePlatformAdmin},
		{"Admin", RolePlatformAdmin},
		{"OPERATOR", RolePlatformOperator},
		{"Approver", RoleGovernanceApprover},
		{"REVIEWER", RoleGovernanceReviewer},
	}
	for _, tc := range tests {
		got := NormalizeRoles([]string{tc.in})
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("NormalizeRoles([%q]) = %v, want [%s]", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeRoles_EmptyInput(t *testing.T) {
	got := NormalizeRoles(nil)
	if len(got) != 0 {
		t.Errorf("NormalizeRoles(nil) = %v, want empty", got)
	}
	got = NormalizeRoles([]string{})
	if len(got) != 0 {
		t.Errorf("NormalizeRoles([]) = %v, want empty", got)
	}
}

func TestNormalizeRoles_Deterministic(t *testing.T) {
	// Same input produces the same sorted output on every call.
	in := []string{"reviewer", "admin", "operator", "approver"}
	first := NormalizeRoles(in)
	second := NormalizeRoles(in)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("NormalizeRoles not deterministic: %v vs %v", first, second)
	}
}
