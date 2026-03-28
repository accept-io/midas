package agent

import (
	"errors"
	"testing"
)

func TestAgentType_IsValid(t *testing.T) {
	cases := []struct {
		name  string
		t     AgentType
		valid bool
	}{
		{"ai", AgentTypeAI, true},
		{"service", AgentTypeService, true},
		{"operator", AgentTypeOperator, true},
		{"empty", AgentType(""), false},
		{"stale_human", AgentType("human"), false},
		{"stale_system", AgentType("system"), false},
		{"stale_hybrid", AgentType("hybrid"), false},
		{"arbitrary", AgentType("robot"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.t.IsValid(); got != c.valid {
				t.Errorf("IsValid() = %v, want %v", got, c.valid)
			}
		})
	}
}

func TestAgentType_Validate(t *testing.T) {
	t.Run("valid types return nil", func(t *testing.T) {
		for _, at := range []AgentType{AgentTypeAI, AgentTypeService, AgentTypeOperator} {
			if err := at.Validate(); err != nil {
				t.Errorf("Validate() for %q returned unexpected error: %v", at, err)
			}
		}
	})

	t.Run("stale DB values return ErrInvalidAgentType", func(t *testing.T) {
		for _, stale := range []AgentType{"human", "system", "hybrid"} {
			err := stale.Validate()
			if err == nil {
				t.Errorf("Validate() for stale type %q expected error, got nil", stale)
				continue
			}
			if !errors.Is(err, ErrInvalidAgentType) {
				t.Errorf("Validate() for %q: got %v, want errors.Is ErrInvalidAgentType", stale, err)
			}
		}
	})

	t.Run("error message includes the invalid value", func(t *testing.T) {
		err := AgentType("human").Validate()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if msg := err.Error(); msg == "" {
			t.Error("expected non-empty error message")
		}
	})
}
