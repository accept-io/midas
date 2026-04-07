package validate

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

const (
	MaxIDLength    = 255
	MaxNameLength  = 512
	MaxFieldLength = 4096
)

var (
	// Must start with lowercase letter or digit, then only lowercase letters,
	// digits, dots, hyphens, and underscores.
	IDFormat = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

	ValidRiskTiers        = []string{"low", "medium", "high"}
	ValidStatuses         = []string{"active", "inactive", "deprecated"}
	ValidGrantStatuses    = []string{"active", "suspended", "revoked", "expired"}
	ValidFailModes        = []string{"open", "closed"}
	ValidAgentTypes       = []string{"llm_agent", "workflow", "automation", "copilot", "rpa"}
	ValidConsequenceTypes = []string{"monetary", "risk_rating"}
	ValidServiceTypes     = []string{"customer_facing", "internal", "technical"}

	// ValidBusinessServiceStatuses is narrower than ValidStatuses: the business_services
	// schema CHECK constraint allows only 'active' and 'deprecated' (not 'inactive').
	// Using ValidStatuses here would let 'inactive' pass validation and then fail
	// at the DB with a constraint error instead of a clean 422.
	ValidBusinessServiceStatuses = []string{"active", "deprecated"}
)

type document interface {
	GetKind() string
	GetID() string
}

// ValidateDocument validates a single parsed control-plane document.
// Parser-owned checks such as apiVersion/kind presence/support are intentionally
// not repeated here.
func ValidateDocument(doc parser.ParsedDocument) []types.ValidationError {
	var errs []types.ValidationError

	errs = append(errs, validateIdentity(doc)...)

	switch d := doc.Doc.(type) {
	case types.ProcessCapabilityDocument:
		errs = append(errs, validateProcessCapability(d)...)
	case types.ProcessBusinessServiceDocument:
		errs = append(errs, validateProcessBusinessService(d)...)
	case types.BusinessServiceDocument:
		errs = append(errs, validateBusinessService(d)...)
	case types.CapabilityDocument:
		errs = append(errs, validateCapability(d)...)
	case types.ProcessDocument:
		errs = append(errs, validateProcess(d)...)
	case types.SurfaceDocument:
		errs = append(errs, validateSurface(d)...)
	case types.AgentDocument:
		errs = append(errs, validateAgent(d)...)
	case types.ProfileDocument:
		errs = append(errs, validateProfile(d)...)
	case types.GrantDocument:
		errs = append(errs, validateGrant(d)...)
	default:
		errs = append(errs, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Message: fmt.Sprintf("unsupported document type: %T", doc.Doc),
		})
	}

	return errs
}

// ValidateBundle validates a set of parsed documents.
// It performs:
// - per-document validation
// - duplicate IDs of the same kind within the same bundle
// - document index annotation for user-facing errors
func ValidateBundle(docs []parser.ParsedDocument) []types.ValidationError {
	var errs []types.ValidationError

	for i, doc := range docs {
		docErrs := ValidateDocument(doc)
		for _, err := range docErrs {
			err.DocumentIndex = i + 1 // 1-based for human readability
			errs = append(errs, err)
		}
	}

	occurrences := make(map[string][]int) // kind::id -> 1-based document indices
	for i, doc := range docs {
		if strings.TrimSpace(doc.ID) == "" {
			continue
		}
		key := doc.Kind + "::" + doc.ID
		occurrences[key] = append(occurrences[key], i+1)
	}

	for _, doc := range docs {
		if strings.TrimSpace(doc.ID) == "" {
			continue
		}
		key := doc.Kind + "::" + doc.ID
		indices := occurrences[key]
		if len(indices) > 1 {
			for _, idx := range indices {
				if idxDoc := docs[idx-1]; idxDoc.Kind == doc.Kind && idxDoc.ID == doc.ID {
					errs = append(errs, types.ValidationError{
						Kind:          idxDoc.Kind,
						ID:            idxDoc.ID,
						Field:         "metadata.id",
						Message:       fmt.Sprintf("duplicate resource id within bundle (appears in documents %v)", indices),
						DocumentIndex: idx,
					})
				}
			}
			// prevent adding duplicates repeatedly for same key
			delete(occurrences, key)
		}
	}

	return errs
}

func validateIdentity(doc parser.ParsedDocument) []types.ValidationError {
	var errs []types.ValidationError

	if strings.TrimSpace(doc.ID) == "" {
		errs = append(errs, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "metadata.id",
			Message: "metadata.id is required",
		})
		return errs
	}

	if err := validateIDFormat(doc.ID); err != nil {
		errs = append(errs, types.ValidationError{
			Kind:    doc.Kind,
			ID:      doc.ID,
			Field:   "metadata.id",
			Message: fmt.Sprintf("invalid id format: %v", err),
		})
	}

	return errs
}

func validateIDFormat(id string) error {
	if len(id) > MaxIDLength {
		return fmt.Errorf("exceeds maximum length of %d characters (got %d)", MaxIDLength, len(id))
	}
	if id != strings.TrimSpace(id) {
		return fmt.Errorf("contains leading or trailing whitespace")
	}
	if strings.Contains(id, " ") {
		return fmt.Errorf("contains spaces")
	}
	if !IDFormat.MatchString(id) {
		return fmt.Errorf("must start with lowercase letter or digit and contain only [a-z0-9._-]")
	}
	return nil
}

func validateProcessCapability(doc types.ProcessCapabilityDocument) []types.ValidationError {
	var errs []types.ValidationError
	if strings.TrimSpace(doc.Spec.ProcessID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.process_id"))
	} else if err := validateIDFormat(doc.Spec.ProcessID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.process_id", err.Error()))
	}
	if strings.TrimSpace(doc.Spec.CapabilityID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.capability_id"))
	} else if err := validateIDFormat(doc.Spec.CapabilityID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.capability_id", err.Error()))
	}
	return errs
}

func validateProcessBusinessService(doc types.ProcessBusinessServiceDocument) []types.ValidationError {
	var errs []types.ValidationError
	if strings.TrimSpace(doc.Spec.ProcessID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.process_id"))
	} else if err := validateIDFormat(doc.Spec.ProcessID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.process_id", err.Error()))
	}
	if strings.TrimSpace(doc.Spec.BusinessServiceID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.business_service_id"))
	} else if err := validateIDFormat(doc.Spec.BusinessServiceID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.business_service_id", err.Error()))
	}
	return errs
}

func validateBusinessService(doc types.BusinessServiceDocument) []types.ValidationError {
	var errs []types.ValidationError
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name", fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}
	if strings.TrimSpace(doc.Spec.ServiceType) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.service_type"))
	} else if !contains(ValidServiceTypes, doc.Spec.ServiceType) {
		errs = append(errs, enumErr(doc, "spec.service_type", doc.Spec.ServiceType, ValidServiceTypes))
	}
	if strings.TrimSpace(doc.Spec.Status) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.status"))
	} else if !contains(ValidBusinessServiceStatuses, doc.Spec.Status) {
		errs = append(errs, enumErr(doc, "spec.status", doc.Spec.Status, ValidBusinessServiceStatuses))
	}
	return errs
}

func validateCapability(doc types.CapabilityDocument) []types.ValidationError {
	var errs []types.ValidationError
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name", fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}
	if strings.TrimSpace(doc.Spec.Status) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.status"))
	} else if !contains(ValidStatuses, doc.Spec.Status) {
		errs = append(errs, enumErr(doc, "spec.status", doc.Spec.Status, ValidStatuses))
	}
	if parentID := strings.TrimSpace(doc.Spec.ParentCapabilityID); parentID != "" {
		if err := validateIDFormat(parentID); err != nil {
			errs = append(errs, fieldErr(doc, "spec.parent_capability_id", err.Error()))
		}
	}
	return errs
}

func validateProcess(doc types.ProcessDocument) []types.ValidationError {
	var errs []types.ValidationError
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name", fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}
	if strings.TrimSpace(doc.Spec.CapabilityID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.capability_id"))
	} else if err := validateIDFormat(doc.Spec.CapabilityID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.capability_id", err.Error()))
	}
	if strings.TrimSpace(doc.Spec.Status) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.status"))
	} else if !contains(ValidStatuses, doc.Spec.Status) {
		errs = append(errs, enumErr(doc, "spec.status", doc.Spec.Status, ValidStatuses))
	}
	if parentID := strings.TrimSpace(doc.Spec.ParentProcessID); parentID != "" {
		if err := validateIDFormat(parentID); err != nil {
			errs = append(errs, fieldErr(doc, "spec.parent_process_id", err.Error()))
		}
	}
	return errs
}

func validateSurface(doc types.SurfaceDocument) []types.ValidationError {
	var errs []types.ValidationError

	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}

	if strings.TrimSpace(doc.Spec.Category) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.category"))
	} else if len(doc.Spec.Category) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.category",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	if strings.TrimSpace(doc.Spec.RiskTier) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.risk_tier"))
	} else if !contains(ValidRiskTiers, doc.Spec.RiskTier) {
		errs = append(errs, enumErr(doc, "spec.risk_tier", doc.Spec.RiskTier, ValidRiskTiers))
	}

	if strings.TrimSpace(doc.Spec.Status) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.status"))
	} else if !contains(ValidStatuses, doc.Spec.Status) {
		errs = append(errs, enumErr(doc, "spec.status", doc.Spec.Status, ValidStatuses))
	}

	if len(doc.Spec.Description) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.description",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	if strings.TrimSpace(doc.Spec.ProcessID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.process_id"))
	} else if err := validateIDFormat(doc.Spec.ProcessID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.process_id", err.Error()))
	}

	return errs
}

func validateAgent(doc types.AgentDocument) []types.ValidationError {
	var errs []types.ValidationError

	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}

	if strings.TrimSpace(doc.Spec.Type) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.type"))
	} else if !contains(ValidAgentTypes, doc.Spec.Type) {
		errs = append(errs, enumErr(doc, "spec.type", doc.Spec.Type, ValidAgentTypes))
	}

	if doc.Spec.Type == "llm_agent" {
		if strings.TrimSpace(doc.Spec.Runtime.Model) == "" {
			errs = append(errs, requiredFieldErr(doc, "spec.runtime.model"))
		}
		if strings.TrimSpace(doc.Spec.Runtime.Provider) == "" {
			errs = append(errs, requiredFieldErr(doc, "spec.runtime.provider"))
		}
	}

	if strings.TrimSpace(doc.Spec.Status) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.status"))
	} else if !contains(ValidStatuses, doc.Spec.Status) {
		errs = append(errs, enumErr(doc, "spec.status", doc.Spec.Status, ValidStatuses))
	}

	return errs
}

func validateProfile(doc types.ProfileDocument) []types.ValidationError {
	var errs []types.ValidationError

	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}

	if strings.TrimSpace(doc.Spec.SurfaceID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.surface_id"))
	} else if err := validateIDFormat(doc.Spec.SurfaceID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.surface_id",
			fmt.Sprintf("invalid format: %v", err)))
	}

	threshold := doc.Spec.Authority.DecisionConfidenceThreshold
	if threshold < 0.0 || threshold > 1.0 {
		errs = append(errs, fieldErr(doc, "spec.authority.decision_confidence_threshold",
			fmt.Sprintf("must be between 0.0 and 1.0 (got %.2f)", threshold)))
	}

	ct := doc.Spec.Authority.ConsequenceThreshold
	if strings.TrimSpace(ct.Type) != "" {
		if !contains(ValidConsequenceTypes, ct.Type) {
			errs = append(errs, enumErr(doc, "spec.authority.consequence_threshold.type", ct.Type, ValidConsequenceTypes))
		}

		switch ct.Type {
		case "monetary":
			if ct.Amount < 0 {
				errs = append(errs, fieldErr(doc, "spec.authority.consequence_threshold.amount",
					"must be non-negative for monetary type"))
			}
			if strings.TrimSpace(ct.Currency) == "" {
				errs = append(errs, requiredFieldErr(doc, "spec.authority.consequence_threshold.currency"))
			}
		case "risk_rating":
			if strings.TrimSpace(ct.RiskRating) == "" {
				errs = append(errs, requiredFieldErr(doc, "spec.authority.consequence_threshold.risk_rating"))
			}
		}
	}

	if strings.TrimSpace(doc.Spec.Policy.Reference) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.policy.reference"))
	} else if len(doc.Spec.Policy.Reference) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.policy.reference",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	if strings.TrimSpace(doc.Spec.Policy.FailMode) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.policy.fail_mode"))
	} else if !contains(ValidFailModes, doc.Spec.Policy.FailMode) {
		errs = append(errs, enumErr(doc, "spec.policy.fail_mode", doc.Spec.Policy.FailMode, ValidFailModes))
	}

	for i, ctx := range doc.Spec.InputRequirements.RequiredContext {
		if strings.TrimSpace(ctx) == "" {
			errs = append(errs, fieldErr(doc,
				fmt.Sprintf("spec.input_requirements.required_context[%d]", i),
				"context key cannot be empty"))
		}
	}

	return errs
}

func validateGrant(doc types.GrantDocument) []types.ValidationError {
	var errs []types.ValidationError

	if strings.TrimSpace(doc.Spec.AgentID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.agent_id"))
	} else if err := validateIDFormat(doc.Spec.AgentID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.agent_id",
			fmt.Sprintf("invalid format: %v", err)))
	}

	if strings.TrimSpace(doc.Spec.ProfileID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.profile_id"))
	} else if err := validateIDFormat(doc.Spec.ProfileID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.profile_id",
			fmt.Sprintf("invalid format: %v", err)))
	}

	if strings.TrimSpace(doc.Spec.GrantedBy) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.granted_by"))
	} else if len(doc.Spec.GrantedBy) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.granted_by",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	if strings.TrimSpace(doc.Spec.GrantedAt) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.granted_at"))
	}
	if strings.TrimSpace(doc.Spec.EffectiveFrom) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.effective_from"))
	}
	if strings.TrimSpace(doc.Spec.Status) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.status"))
	} else if !contains(ValidGrantStatuses, doc.Spec.Status) {
		errs = append(errs, enumErr(doc, "spec.status", doc.Spec.Status, ValidGrantStatuses))
	}

	var grantedAt, effectiveFrom, effectiveUntil time.Time
	var grantedAtOK, effectiveFromOK, effectiveUntilOK bool

	if strings.TrimSpace(doc.Spec.GrantedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, doc.Spec.GrantedAt)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.granted_at", "must be a valid RFC3339 timestamp"))
		} else {
			grantedAt = parsed
			grantedAtOK = true
		}
	}

	if strings.TrimSpace(doc.Spec.EffectiveFrom) != "" {
		parsed, err := time.Parse(time.RFC3339, doc.Spec.EffectiveFrom)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.effective_from", "must be a valid RFC3339 timestamp"))
		} else {
			effectiveFrom = parsed
			effectiveFromOK = true
		}
	}

	if strings.TrimSpace(doc.Spec.EffectiveUntil) != "" {
		parsed, err := time.Parse(time.RFC3339, doc.Spec.EffectiveUntil)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.effective_until", "must be a valid RFC3339 timestamp"))
		} else {
			effectiveUntil = parsed
			effectiveUntilOK = true
		}
	}

	if effectiveFromOK && effectiveUntilOK && effectiveFrom.After(effectiveUntil) {
		errs = append(errs, fieldErr(doc, "spec.effective_from",
			"must be before or equal to spec.effective_until"))
	}

	if grantedAtOK && effectiveFromOK && grantedAt.After(effectiveFrom) {
		errs = append(errs, fieldErr(doc, "spec.granted_at",
			"must be before or equal to spec.effective_from"))
	}

	return errs
}

func requiredFieldErr(doc document, field string) types.ValidationError {
	return types.ValidationError{
		Kind:    doc.GetKind(),
		ID:      doc.GetID(),
		Field:   field,
		Message: field + " is required",
	}
}

func fieldErr(doc document, field, message string) types.ValidationError {
	return types.ValidationError{
		Kind:    doc.GetKind(),
		ID:      doc.GetID(),
		Field:   field,
		Message: message,
	}
}

func enumErr(doc document, field, value string, allowed []string) types.ValidationError {
	return types.ValidationError{
		Kind:    doc.GetKind(),
		ID:      doc.GetID(),
		Field:   field,
		Message: fmt.Sprintf("invalid value %q (must be one of: %s)", value, strings.Join(allowed, ", ")),
	}
}

func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
