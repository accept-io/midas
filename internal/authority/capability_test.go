package authority

import (
	"errors"
	"testing"
)

func TestCapability_IsValid(t *testing.T) {
	for _, c := range []Capability{
		CapabilityRecommend, CapabilityApprove, CapabilityReject,
		CapabilityEscalate, CapabilityStop,
	} {
		if !c.IsValid() {
			t.Errorf("canonical capability %q reported invalid", string(c))
		}
		if !IsValidCapability(c) {
			t.Errorf("IsValidCapability(%q) = false; want true", string(c))
		}
	}
	for _, c := range []Capability{"", "delegate", "Approve", "RECOMMEND"} {
		if c.IsValid() {
			t.Errorf("non-canonical capability %q reported valid", string(c))
		}
	}
}

func TestValidateCapabilities_ValidSet(t *testing.T) {
	caps := []Capability{CapabilityRecommend, CapabilityApprove, CapabilityStop}
	if err := ValidateCapabilities(caps); err != nil {
		t.Errorf("ValidateCapabilities(valid): %v", err)
	}
}

func TestValidateCapabilities_EmptyIsValid(t *testing.T) {
	if err := ValidateCapabilities(nil); err != nil {
		t.Errorf("nil capabilities should validate: %v", err)
	}
	if err := ValidateCapabilities([]Capability{}); err != nil {
		t.Errorf("empty capabilities should validate: %v", err)
	}
}

func TestValidateCapabilities_RejectsInvalidValue(t *testing.T) {
	err := ValidateCapabilities([]Capability{CapabilityApprove, "delegate"})
	if err == nil {
		t.Fatal("expected error for invalid capability")
	}
	if !errors.Is(err, ErrInvalidCapability) {
		t.Errorf("error chain should wrap ErrInvalidCapability; got %v", err)
	}
}

func TestValidateCapabilities_RejectsDuplicates(t *testing.T) {
	err := ValidateCapabilities([]Capability{CapabilityApprove, CapabilityApprove})
	if err == nil {
		t.Fatal("expected error for duplicate capability")
	}
	if !errors.Is(err, ErrDuplicateCapability) {
		t.Errorf("error chain should wrap ErrDuplicateCapability; got %v", err)
	}
}

func TestHasCapability(t *testing.T) {
	caps := []Capability{CapabilityRecommend, CapabilityApprove, CapabilityStop}
	cases := map[Capability]bool{
		CapabilityRecommend: true,
		CapabilityApprove:   true,
		CapabilityStop:      true,
		CapabilityReject:    false,
		CapabilityEscalate:  false,
		"":                  false,
	}
	for want, expect := range cases {
		got := HasCapability(caps, want)
		if got != expect {
			t.Errorf("HasCapability(%q) = %v; want %v", string(want), got, expect)
		}
	}
}

func TestCapabilityConstants_Stop_IsCanonicalValue(t *testing.T) {
	// Pin the literal value: stop authority is a CAPABILITY,
	// not a failmode.Outcome — the wire string is "stop", not
	// "halt" or "kill_switch".
	if string(CapabilityStop) != "stop" {
		t.Errorf("CapabilityStop literal: want %q, got %q", "stop", string(CapabilityStop))
	}
}
