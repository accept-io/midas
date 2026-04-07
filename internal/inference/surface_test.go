package inference

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ValidateSurfaceID
// ---------------------------------------------------------------------------

func TestValidateSurfaceID_Valid(t *testing.T) {
	valid := []string{
		// spec examples
		"loan.approve",
		"claims.review.manual",
		"payment_execute",
		"fraud-check.run",
		"a",
		"a.b",
		"a_b-c.1",
		"test123",
		// additional edge cases
		"a0",
		"z9",
		"0",
		"9",
		"my-resource.id_v2",
		"payment-execution-v1.2.3",
		"abc",
		"a.b.c.d.e",
		"x123.y456.z789",
		// long but valid
		"this.is.a.very.long.but.valid.surface.identifier.with.many.segments",
	}

	for _, id := range valid {
		t.Run(id, func(t *testing.T) {
			if err := ValidateSurfaceID(id); err != nil {
				t.Errorf("expected %q to be valid, got: %v", id, err)
			}
		})
	}
}

func TestValidateSurfaceID_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		// empty
		{
			name:    "empty string",
			id:      "",
			wantErr: "must not be empty",
		},
		// uppercase
		{
			name:    "uppercase first char",
			id:      "Loan.Approve",
			wantErr: "must be lowercase",
		},
		{
			name:    "uppercase mid-id",
			id:      "loan.Approve",
			wantErr: "must be lowercase",
		},
		{
			name:    "all caps",
			id:      "LOAN",
			wantErr: "must be lowercase",
		},
		// whitespace
		{
			name:    "space in id",
			id:      "loan approve",
			wantErr: "invalid character",
		},
		{
			name:    "tab in id",
			id:      "loan\tapprove",
			wantErr: "invalid character",
		},
		// special characters
		{
			name:    "colon",
			id:      "loan:approve",
			wantErr: "invalid character",
		},
		{
			name:    "slash",
			id:      "loan/approve",
			wantErr: "invalid character",
		},
		{
			name:    "at sign",
			id:      "loan@approve",
			wantErr: "invalid character",
		},
		{
			name:    "exclamation",
			id:      "loan!approve",
			wantErr: "invalid character",
		},
		// leading characters
		{
			name:    "leading dot",
			id:      ".leadingdot",
			wantErr: "must not start with '.'",
		},
		{
			name:    "leading underscore",
			id:      "_underscore",
			wantErr: "must start with an alphanumeric",
		},
		{
			name:    "leading hyphen",
			id:      "-leading",
			wantErr: "must start with an alphanumeric",
		},
		// trailing characters
		{
			name:    "trailing dot",
			id:      "trailingdot.",
			wantErr: "must end with an alphanumeric",
		},
		{
			name:    "trailing hyphen",
			id:      "hyphen-",
			wantErr: "must end with an alphanumeric",
		},
		{
			name:    "trailing underscore",
			id:      "loan_",
			wantErr: "must end with an alphanumeric",
		},
		// consecutive dots
		{
			name:    "double dot",
			id:      "double..dot",
			wantErr: "consecutive dots",
		},
		{
			name:    "triple dot",
			id:      "triple...dot",
			wantErr: "consecutive dots",
		},
		{
			name:    "leading double dot",
			id:      "..start",
			wantErr: "must not start with '.'",
		},
		// unicode / non-ASCII
		{
			name:    "unicode first char",
			id:      "ümlaut.test",
			wantErr: "must start with an alphanumeric",
		},
		{
			name:    "unicode mid-id",
			id:      "loan.ümlaut",
			wantErr: "invalid character",
		},
		{
			// emoji as last char: caught by trailing-char check before the for-loop
			name:    "emoji trailing",
			id:      "loan.🚀",
			wantErr: "must end with an alphanumeric",
		},
		{
			// emoji mid-id (not trailing): caught by for-loop invalid-character check
			name:    "emoji mid-id",
			id:      "loan.🚀.approve",
			wantErr: "invalid character",
		},
		// punctuation-heavy
		{
			name:    "multiple invalid chars",
			id:      "loan!@#approve",
			wantErr: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSurfaceID(tt.id)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.id)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q for input %q, got: %v", tt.wantErr, tt.id, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InferStructure
// ---------------------------------------------------------------------------

func TestInferStructure(t *testing.T) {
	tests := []struct {
		name      string
		surfaceID string
		wantCap   string
		wantProc  string
	}{
		// spec examples
		{
			name:      "first-dot splitting",
			surfaceID: "loan.approve",
			wantCap:   "auto:loan",
			wantProc:  "auto:loan.approve",
		},
		{
			name:      "multiple dots — capability is only first segment",
			surfaceID: "claims.review.manual",
			wantCap:   "auto:claims",
			wantProc:  "auto:claims.review.manual",
		},
		{
			name:      "no dot — capability falls back to auto:general",
			surfaceID: "payment_execute",
			wantCap:   "auto:general",
			wantProc:  "auto:payment_execute",
		},
		{
			name:      "three segments — first wins",
			surfaceID: "a.b.c",
			wantCap:   "auto:a",
			wantProc:  "auto:a.b.c",
		},
		// edge cases
		{
			name:      "single valid character",
			surfaceID: "a",
			wantCap:   "auto:general",
			wantProc:  "auto:a",
		},
		{
			name:      "two segments",
			surfaceID: "a.b",
			wantCap:   "auto:a",
			wantProc:  "auto:a.b",
		},
		{
			name:      "underscore in no-dot id",
			surfaceID: "fraud_check",
			wantCap:   "auto:general",
			wantProc:  "auto:fraud_check",
		},
		{
			name:      "hyphen in no-dot id",
			surfaceID: "fraud-check",
			wantCap:   "auto:general",
			wantProc:  "auto:fraud-check",
		},
		{
			name:      "hyphen and dot combined",
			surfaceID: "fraud-check.run",
			wantCap:   "auto:fraud-check",
			wantProc:  "auto:fraud-check.run",
		},
		// invalid input passes through as best-effort (no panic, no validation)
		{
			name:      "empty input — best-effort",
			surfaceID: "",
			wantCap:   "auto:general",
			wantProc:  "auto:",
		},
		{
			name:      "uppercase passes through unchanged",
			surfaceID: "LOAN.APPROVE",
			wantCap:   "auto:LOAN",
			wantProc:  "auto:LOAN.APPROVE",
		},
		{
			name:      "leading dot passes through — best-effort",
			surfaceID: ".leadingdot",
			wantCap:   "auto:",
			wantProc:  "auto:.leadingdot",
		},
		{
			name:      "no-dot invalid id passes through",
			surfaceID: "INVALID",
			wantCap:   "auto:general",
			wantProc:  "auto:INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCap, gotProc := InferStructure(tt.surfaceID)
			if gotCap != tt.wantCap {
				t.Errorf("capabilityID: got %q, want %q", gotCap, tt.wantCap)
			}
			if gotProc != tt.wantProc {
				t.Errorf("processID: got %q, want %q", gotProc, tt.wantProc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsReservedNamespace
// ---------------------------------------------------------------------------

func TestIsReservedNamespace(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		// true cases
		{
			name: "auto: prefix — simple",
			id:   "auto:loan",
			want: true,
		},
		{
			name: "auto: prefix — dotted",
			id:   "auto:loan.approve",
			want: true,
		},
		{
			name: "auto: prefix — general",
			id:   "auto:general",
			want: true,
		},
		{
			name: "auto: prefix only",
			id:   "auto:",
			want: true,
		},
		// false cases
		{
			name: "normal operator id",
			id:   "lending-v1",
			want: false,
		},
		{
			name: "starts with auto but not auto: — no colon",
			id:   "automatic",
			want: false,
		},
		{
			name: "starts with auto but not auto: — different suffix",
			id:   "autopilot",
			want: false,
		},
		{
			name: "empty string",
			id:   "",
			want: false,
		},
		{
			name: "unrelated id",
			id:   "loan.approve",
			want: false,
		},
		{
			name: "auto without colon",
			id:   "auto",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsReservedNamespace(tt.id)
			if got != tt.want {
				t.Errorf("IsReservedNamespace(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
