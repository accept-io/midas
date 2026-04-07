package apply

import (
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/surface"
)

// mapSurfaceDocumentToDecisionSurface converts a validated SurfaceDocument
// into a DecisionSurface domain model ready for persistence.
//
// Key behaviors:
// - Status is ALWAYS set to review for newly applied surfaces
// - String fields are normalized with TrimSpace
// - Enum fields are validated if present, otherwise safe defaults are used
// - Version is assigned by caller
// - Timestamps are normalized to UTC
func mapSurfaceDocumentToDecisionSurface(
	doc types.SurfaceDocument,
	now time.Time,
	createdBy string,
	version int,
) (*surface.DecisionSurface, error) {
	now = now.UTC()

	ds := &surface.DecisionSurface{
		ID:          strings.TrimSpace(doc.Metadata.ID),
		Version:     version,
		Name:        strings.TrimSpace(doc.Metadata.Name),
		Description: strings.TrimSpace(doc.Spec.Description),
		Category:    strings.TrimSpace(doc.Spec.Category),
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   strings.TrimSpace(createdBy),
	}

	ds.Domain = valueOrDefault(doc.Spec.Domain, "default")

	decisionType := strings.TrimSpace(doc.Spec.DecisionType)
	if decisionType != "" {
		if !isValidDecisionType(decisionType) {
			return nil, fmt.Errorf("invalid decision_type: %s", doc.Spec.DecisionType)
		}
		ds.DecisionType = surface.DecisionType(decisionType)
	} else {
		ds.DecisionType = surface.DecisionTypeOperational
	}

	reversibilityClass := strings.TrimSpace(doc.Spec.ReversibilityClass)
	if reversibilityClass != "" {
		if !isValidReversibilityClass(reversibilityClass) {
			return nil, fmt.Errorf("invalid reversibility_class: %s", doc.Spec.ReversibilityClass)
		}
		ds.ReversibilityClass = surface.ReversibilityClass(reversibilityClass)
	} else {
		ds.ReversibilityClass = surface.ReversibilityConditionallyReversible
	}

	ds.MinimumConfidence = doc.Spec.MinimumConfidence
	if ds.MinimumConfidence < 0.0 || ds.MinimumConfidence > 1.0 {
		return nil, fmt.Errorf("minimum_confidence must be in range [0.0, 1.0], got: %f", ds.MinimumConfidence)
	}

	failureMode := strings.TrimSpace(doc.Spec.FailureMode)
	if failureMode != "" {
		if !isValidFailureMode(failureMode) {
			return nil, fmt.Errorf("invalid failure_mode: %s", doc.Spec.FailureMode)
		}
		ds.FailureMode = surface.FailureMode(failureMode)
	} else {
		ds.FailureMode = surface.FailureModeClosed
	}

	ds.BusinessOwner = valueOrDefault(doc.Spec.BusinessOwner, "unassigned")
	ds.TechnicalOwner = valueOrDefault(doc.Spec.TechnicalOwner, "unassigned")

	ds.RequiredContext = mapContextSchema(doc.Spec.RequiredContext)
	ds.ConsequenceTypes = mapConsequenceTypes(doc.Spec.ConsequenceTypes)
	ds.MandatoryEvidence = mapEvidenceRequirements(doc.Spec.MandatoryEvidence)

	// Validate incoming status if provided, but always persist new applied
	// surfaces as review pending approval.
	status := strings.TrimSpace(doc.Spec.Status)
	if status != "" && !isValidSurfaceStatus(status) {
		return nil, fmt.Errorf("invalid status: %s", doc.Spec.Status)
	}
	ds.Status = surface.SurfaceStatusReview

	if !doc.Spec.EffectiveFrom.IsZero() {
		ds.EffectiveFrom = doc.Spec.EffectiveFrom.UTC()
	} else {
		ds.EffectiveFrom = now
	}

	ds.ProcessID = strings.TrimSpace(doc.Spec.ProcessID)

	return ds, nil
}

func valueOrDefault(val, def string) string {
	trimmed := strings.TrimSpace(val)
	if trimmed != "" {
		return trimmed
	}
	return def
}

func isValidDecisionType(s string) bool {
	switch surface.DecisionType(s) {
	case surface.DecisionTypeStrategic,
		surface.DecisionTypeTactical,
		surface.DecisionTypeOperational:
		return true
	default:
		return false
	}
}

func isValidReversibilityClass(s string) bool {
	switch surface.ReversibilityClass(s) {
	case surface.ReversibilityReversible,
		surface.ReversibilityConditionallyReversible,
		surface.ReversibilityIrreversible:
		return true
	default:
		return false
	}
}

func isValidFailureMode(s string) bool {
	switch surface.FailureMode(s) {
	case surface.FailureModeOpen,
		surface.FailureModeClosed:
		return true
	default:
		return false
	}
}

func isValidSurfaceStatus(s string) bool {
	switch surface.SurfaceStatus(s) {
	case surface.SurfaceStatusDraft,
		surface.SurfaceStatusReview,
		surface.SurfaceStatusActive,
		surface.SurfaceStatusDeprecated,
		surface.SurfaceStatusRetired:
		return true
	default:
		return false
	}
}

func mapContextSchema(src types.ContextSchema) surface.ContextSchema {
	fields := make([]surface.ContextField, 0, len(src.Fields))
	for _, f := range src.Fields {
		fields = append(fields, surface.ContextField{
			Name:        f.Name,
			Type:        surface.FieldType(f.Type),
			Required:    f.Required,
			Description: f.Description,
			Validation:  mapValidationRule(f.Validation),
			Example:     f.Example,
		})
	}
	return surface.ContextSchema{
		Fields: fields,
	}
}

func mapValidationRule(src *types.ValidationRule) *surface.ValidationRule {
	if src == nil {
		return nil
	}
	return &surface.ValidationRule{
		Pattern:          src.Pattern,
		MinLength:        src.MinLength,
		MaxLength:        src.MaxLength,
		Enum:             append([]string(nil), src.Enum...),
		Minimum:          src.Minimum,
		Maximum:          src.Maximum,
		ExclusiveMinimum: src.ExclusiveMinimum,
		ExclusiveMaximum: src.ExclusiveMaximum,
		MinItems:         src.MinItems,
		MaxItems:         src.MaxItems,
	}
}

func mapConsequenceTypes(src []types.ConsequenceType) []surface.ConsequenceType {
	if len(src) == 0 {
		return nil
	}

	out := make([]surface.ConsequenceType, 0, len(src))
	for _, ct := range src {
		out = append(out, surface.ConsequenceType{
			ID:           ct.ID,
			Name:         ct.Name,
			Description:  ct.Description,
			MeasureType:  surface.MeasureType(ct.MeasureType),
			Currency:     ct.Currency,
			DurationUnit: surface.DurationUnit(ct.DurationUnit),
			RatingScale:  append([]string(nil), ct.RatingScale...),
			ScopeScale:   append([]string(nil), ct.ScopeScale...),
			MinValue:     ct.MinValue,
			MaxValue:     ct.MaxValue,
		})
	}
	return out
}

func mapEvidenceRequirements(src []types.EvidenceRequirement) []surface.EvidenceRequirement {
	if len(src) == 0 {
		return nil
	}

	out := make([]surface.EvidenceRequirement, 0, len(src))
	for _, e := range src {
		out = append(out, surface.EvidenceRequirement{
			ID:          e.ID,
			Name:        e.Name,
			Description: e.Description,
			Required:    e.Required,
			Format:      e.Format,
		})
	}
	return out
}
