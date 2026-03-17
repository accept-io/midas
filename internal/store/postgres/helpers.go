package postgres

import (
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// ---------------------------------------------------------------------------
// Shared nullable helpers for SQL parameter binding
// ---------------------------------------------------------------------------

// nullableString converts empty string to SQL NULL
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableTime converts nil pointer to SQL NULL
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// nullableInt converts zero to SQL NULL
func nullableInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}

// nullableOutcome converts empty Outcome to SQL NULL
func nullableOutcome(o eval.Outcome) any {
	if o == "" {
		return nil
	}
	return string(o)
}

// nullableReasonCode converts empty ReasonCode to SQL NULL
func nullableReasonCode(r eval.ReasonCode) any {
	if r == "" {
		return nil
	}
	return string(r)
}

// ---------------------------------------------------------------------------
// Profile-specific helpers for consequence threshold
// ---------------------------------------------------------------------------

func nullableAmount(c authority.Consequence) any {
	if c.Type == value.ConsequenceTypeMonetary {
		return c.Amount
	}
	return nil
}

func nullableCurrency(c authority.Consequence) any {
	if c.Type == value.ConsequenceTypeMonetary {
		return c.Currency
	}
	return nil
}

func nullableRiskRating(c authority.Consequence) any {
	if c.Type == value.ConsequenceTypeRiskRating {
		return string(c.RiskRating)
	}
	return nil
}
