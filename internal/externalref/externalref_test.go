package externalref

import (
	"errors"
	"testing"
	"time"
)

func TestIsZero(t *testing.T) {
	syncTime := time.Now().UTC()

	cases := []struct {
		name string
		ref  *ExternalRef
		want bool
	}{
		{"nil", nil, true},
		{"empty struct", &ExternalRef{}, true},
		{"only source_system", &ExternalRef{SourceSystem: "github"}, false},
		{"only source_id", &ExternalRef{SourceID: "x"}, false},
		{"only source_url", &ExternalRef{SourceURL: "https://example"}, false},
		{"only source_version", &ExternalRef{SourceVersion: "v1"}, false},
		{"only last_synced_at", &ExternalRef{LastSyncedAt: &syncTime}, false},
		{"populated and valid", &ExternalRef{SourceSystem: "github", SourceID: "x"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.IsZero(); got != tc.want {
				t.Errorf("IsZero() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		ref     *ExternalRef
		wantErr bool
	}{
		{"nil ok", nil, false},
		{"both empty ok", &ExternalRef{}, false},
		{"both set ok", &ExternalRef{SourceSystem: "github", SourceID: "x"}, false},
		{"system without id rejected", &ExternalRef{SourceSystem: "github"}, true},
		{"id without system rejected", &ExternalRef{SourceID: "x"}, true},
		{"only optional fields rejected (system without id)", &ExternalRef{SourceURL: "https://x", SourceVersion: "v1"}, false},
		{"whitespace system + valid id rejected", &ExternalRef{SourceSystem: "  ", SourceID: "x"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ref.Validate()
			if tc.wantErr && !errors.Is(err, ErrInconsistent) {
				t.Errorf("Validate() = %v, want ErrInconsistent", err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestClone_DeepCopiesTimestamp(t *testing.T) {
	original := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	src := &ExternalRef{
		SourceSystem: "github", SourceID: "x", LastSyncedAt: &original,
	}
	clone := src.Clone()
	if clone == src {
		t.Error("Clone returned the same pointer")
	}
	if clone.LastSyncedAt == src.LastSyncedAt {
		t.Error("Clone did not deep-copy LastSyncedAt — same pointer leaked")
	}

	// Mutate the clone's timestamp; original must be untouched.
	mutated := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	*clone.LastSyncedAt = mutated
	if !src.LastSyncedAt.Equal(original) {
		t.Errorf("source mutated through clone's LastSyncedAt: got %v, want %v", *src.LastSyncedAt, original)
	}
}

func TestClone_NilReturnsNil(t *testing.T) {
	var ref *ExternalRef
	if got := ref.Clone(); got != nil {
		t.Errorf("Clone() of nil = %+v, want nil", got)
	}
}

func TestCanonicalise_NilOnZero(t *testing.T) {
	if Canonicalise(nil) != nil {
		t.Error("Canonicalise(nil) should return nil")
	}
	if Canonicalise(&ExternalRef{}) != nil {
		t.Error("Canonicalise(empty) should return nil")
	}
	r := &ExternalRef{SourceSystem: "github", SourceID: "x"}
	if Canonicalise(r) != r {
		t.Error("Canonicalise of non-zero should return same pointer")
	}
}

func TestEqual(t *testing.T) {
	t1 := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	t1Other := t1 // same wall clock; Equal must treat as equal even with different pointers
	t2 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		a, b *ExternalRef
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", &ExternalRef{}, &ExternalRef{}, true},
		{"nil vs empty (canonicalised equivalent)", nil, &ExternalRef{}, true},
		{"empty vs nil (symmetric)", &ExternalRef{}, nil, true},
		{
			"identical fields",
			&ExternalRef{SourceSystem: "github", SourceID: "x"},
			&ExternalRef{SourceSystem: "github", SourceID: "x"},
			true,
		},
		{
			"different source_system",
			&ExternalRef{SourceSystem: "github", SourceID: "x"},
			&ExternalRef{SourceSystem: "leanix", SourceID: "x"},
			false,
		},
		{
			"different source_id",
			&ExternalRef{SourceSystem: "github", SourceID: "x"},
			&ExternalRef{SourceSystem: "github", SourceID: "y"},
			false,
		},
		{
			"different source_url",
			&ExternalRef{SourceSystem: "github", SourceID: "x", SourceURL: "https://a"},
			&ExternalRef{SourceSystem: "github", SourceID: "x", SourceURL: "https://b"},
			false,
		},
		{
			"different source_version",
			&ExternalRef{SourceSystem: "github", SourceID: "x", SourceVersion: "v1"},
			&ExternalRef{SourceSystem: "github", SourceID: "x", SourceVersion: "v2"},
			false,
		},
		{
			"timestamps equal at wall clock — pointer-distinct",
			&ExternalRef{SourceSystem: "github", SourceID: "x", LastSyncedAt: &t1},
			&ExternalRef{SourceSystem: "github", SourceID: "x", LastSyncedAt: &t1Other},
			true,
		},
		{
			"timestamps differ",
			&ExternalRef{SourceSystem: "github", SourceID: "x", LastSyncedAt: &t1},
			&ExternalRef{SourceSystem: "github", SourceID: "x", LastSyncedAt: &t2},
			false,
		},
		{
			"one timestamp nil, other set",
			&ExternalRef{SourceSystem: "github", SourceID: "x", LastSyncedAt: &t1},
			&ExternalRef{SourceSystem: "github", SourceID: "x"},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Equal(tc.a, tc.b); got != tc.want {
				t.Errorf("Equal() = %v, want %v", got, tc.want)
			}
		})
	}
}
