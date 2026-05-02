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

	// ValidAISystemStatuses mirrors the chk_ai_systems_status CHECK list.
	// AISystem is status-honouring at apply time: this list controls only
	// per-document field validation, not lifecycle gating.
	ValidAISystemStatuses = []string{"active", "deprecated", "retired"}

	// ValidAISystemOrigins mirrors the chk_ai_systems_origin CHECK list.
	ValidAISystemOrigins = []string{"manual", "inferred"}

	// ValidAISystemVersionStatuses mirrors the chk_ai_versions_status CHECK
	// list. AISystemVersion is status-honouring (deliberate divergence from
	// Surface/Profile): apply persists whatever status the bundle declares.
	ValidAISystemVersionStatuses = []string{"review", "active", "deprecated", "retired"}

	// ValidBusinessServiceStatuses is narrower than ValidStatuses: the business_services
	// schema CHECK constraint allows only 'active' and 'deprecated' (not 'inactive').
	// Using ValidStatuses here would let 'inactive' pass validation and then fail
	// at the DB with a constraint error instead of a clean 422.
	ValidBusinessServiceStatuses = []string{"active", "deprecated"}

	// ValidExpectationStatuses mirrors the 5-element ExpectationStatus enum
	// in internal/governanceexpectation. Apply forces 'review' regardless
	// of what the document states; this list is used only for shape
	// validation of an explicitly-supplied lifecycle.status field.
	ValidExpectationStatuses = []string{"draft", "review", "active", "deprecated", "retired"}

	// ValidExpectationConditionTypes is the closed enum of permitted
	// condition discriminator values. Today only "risk_condition" exists;
	// a future addition is an explicit code change with its own design
	// discussion.
	ValidExpectationConditionTypes = []string{"risk_condition"}

	// ExpectationScopeKindProcessOnly is the apply-side allowlist for
	// scope_kind in #52. The domain itself accepts process,
	// business_service, and capability; apply only accepts process until
	// the matching engine in #53 supplies the additional traversal
	// validators. Operators submitting business_service or capability
	// scopes get a clean validation error instead of partial support.
	ExpectationScopeKindProcessOnly = []string{"process"}
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
	case types.BusinessServiceDocument:
		errs = append(errs, validateBusinessService(d)...)
	case types.BusinessServiceCapabilityDocument:
		errs = append(errs, validateBusinessServiceCapability(d)...)
	case types.BusinessServiceRelationshipDocument:
		errs = append(errs, validateBusinessServiceRelationship(d)...)
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
	case types.GovernanceExpectationDocument:
		errs = append(errs, validateGovernanceExpectation(d)...)
	case types.AISystemDocument:
		errs = append(errs, validateAISystem(d)...)
	case types.AISystemVersionDocument:
		errs = append(errs, validateAISystemVersion(d)...)
	case types.AISystemBindingDocument:
		errs = append(errs, validateAISystemBinding(d)...)
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

	// BusinessServiceRelationship: duplicate (source, target, relationship_type)
	// triple within the bundle. Mirrors the schema's uniq_bsr_triple UNIQUE
	// constraint so the operator gets a validator-level message rather than
	// a Postgres constraint-failure surface at apply time.
	bsrTripleFirstIdx := make(map[string]int)
	for i, doc := range docs {
		if doc.Kind != types.KindBusinessServiceRelationship {
			continue
		}
		bsrDoc, ok := doc.Doc.(types.BusinessServiceRelationshipDocument)
		if !ok {
			continue
		}
		source := strings.TrimSpace(bsrDoc.Spec.SourceBusinessServiceID)
		target := strings.TrimSpace(bsrDoc.Spec.TargetBusinessServiceID)
		relType := strings.TrimSpace(bsrDoc.Spec.RelationshipType)
		// Only meaningful when all three fields are present and well-formed —
		// the per-document validator already flagged anything missing.
		if source == "" || target == "" || relType == "" {
			continue
		}
		key := source + "\x00" + target + "\x00" + relType
		if firstIdx, seen := bsrTripleFirstIdx[key]; seen {
			errs = append(errs, types.ValidationError{
				Kind:          doc.Kind,
				ID:            doc.ID,
				Field:         "spec",
				Message:       fmt.Sprintf("duplicate business-service relationship triple: (source=%q, target=%q, relationship_type=%q) already declared in document %d", source, target, relType, firstIdx),
				DocumentIndex: i + 1,
			})
			continue
		}
		bsrTripleFirstIdx[key] = i + 1
	}

	// AISystemVersion: duplicate (ai_system_id, version) tuple within the
	// bundle. Mirrors the schema's composite PK so the operator gets a
	// validator-level message rather than a Postgres constraint failure
	// at apply time.
	aiVersionTupleFirstIdx := make(map[string]int)
	for i, doc := range docs {
		if doc.Kind != types.KindAISystemVersion {
			continue
		}
		vDoc, ok := doc.Doc.(types.AISystemVersionDocument)
		if !ok {
			continue
		}
		sysID := strings.TrimSpace(vDoc.Spec.AISystemID)
		if sysID == "" || vDoc.Spec.Version < 1 {
			continue
		}
		key := fmt.Sprintf("%s\x00%d", sysID, vDoc.Spec.Version)
		if firstIdx, seen := aiVersionTupleFirstIdx[key]; seen {
			errs = append(errs, types.ValidationError{
				Kind:          doc.Kind,
				ID:            doc.ID,
				Field:         "spec",
				Message:       fmt.Sprintf("duplicate ai system version tuple: (ai_system_id=%q, version=%d) already declared in document %d", sysID, vDoc.Spec.Version, firstIdx),
				DocumentIndex: i + 1,
			})
			continue
		}
		aiVersionTupleFirstIdx[key] = i + 1
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
	errs = append(errs, validateExternalRefSpec(doc, doc.Spec.ExternalRef)...)
	return errs
}

// validateBusinessServiceCapability validates a BusinessServiceCapability
// document — the M:N junction between BusinessService and Capability in the
// v1 service-led structural model.
//
// Per ADR-XXX, junction rows have no lifecycle. The spec carries only
// business_service_id and capability_id; both are required, both are checked
// for non-empty (after whitespace trim) and ID-format conformance. Cross-
// document referential integrity (do the referenced entities exist?) and
// duplicate detection are concerns of the apply planner, not this validator.
func validateBusinessServiceCapability(doc types.BusinessServiceCapabilityDocument) []types.ValidationError {
	var errs []types.ValidationError
	if strings.TrimSpace(doc.Spec.BusinessServiceID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.business_service_id"))
	} else if err := validateIDFormat(doc.Spec.BusinessServiceID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.business_service_id", err.Error()))
	}
	if strings.TrimSpace(doc.Spec.CapabilityID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.capability_id"))
	} else if err := validateIDFormat(doc.Spec.CapabilityID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.capability_id", err.Error()))
	}
	return errs
}

// validateBusinessServiceRelationship validates a BusinessServiceRelationship
// document — the directed junction between two BusinessServices (Epic 1, PR 1).
//
// Per the same posture as BusinessServiceCapability, junction rows have no
// lifecycle. The spec carries source / target / relationship_type / description.
// Field-level rules:
//
//   - source_business_service_id required, ID-format compliant
//   - target_business_service_id required, ID-format compliant
//   - source_business_service_id != target_business_service_id (self-ref)
//   - relationship_type ∈ {depends_on, supports, part_of}
//
// Cross-bundle resolution (do the referenced BSes exist in the bundle or
// in the persisted store?) is the apply planner's concern, not this
// validator's — mirroring BSC. Bundle-uniqueness rules (duplicate id,
// duplicate triple) live in ValidateBundle.
//
// Cycle detection scope: only direct self-reference is rejected here.
// TODO(epic-1, future PR): implement recursive cycle detection across the
// BSR graph (depends_on chains in particular). The schema accepts cyclic
// links today; the validator does not yet reject them.
func validateBusinessServiceRelationship(doc types.BusinessServiceRelationshipDocument) []types.ValidationError {
	var errs []types.ValidationError

	source := strings.TrimSpace(doc.Spec.SourceBusinessServiceID)
	target := strings.TrimSpace(doc.Spec.TargetBusinessServiceID)
	relType := strings.TrimSpace(doc.Spec.RelationshipType)

	if source == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.source_business_service_id"))
	} else if err := validateIDFormat(source); err != nil {
		errs = append(errs, fieldErr(doc, "spec.source_business_service_id", err.Error()))
	}
	if target == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.target_business_service_id"))
	} else if err := validateIDFormat(target); err != nil {
		errs = append(errs, fieldErr(doc, "spec.target_business_service_id", err.Error()))
	}

	// Self-reference is rejected even when the IDs are otherwise well-formed.
	if source != "" && target != "" && source == target {
		errs = append(errs, fieldErr(doc, "spec.target_business_service_id",
			"source_business_service_id and target_business_service_id must differ"))
	}

	if relType == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.relationship_type"))
	} else if !isValidBSRRelationshipType(relType) {
		errs = append(errs, enumErr(doc, "spec.relationship_type", relType,
			[]string{"depends_on", "supports", "part_of"}))
	}

	errs = append(errs, validateExternalRefSpec(doc, doc.Spec.ExternalRef)...)

	return errs
}

func isValidBSRRelationshipType(t string) bool {
	switch t {
	case "depends_on", "supports", "part_of":
		return true
	}
	return false
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
	if strings.TrimSpace(doc.Spec.BusinessServiceID) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.business_service_id"))
	} else if err := validateIDFormat(doc.Spec.BusinessServiceID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.business_service_id", err.Error()))
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

// validateGovernanceExpectation validates a GovernanceExpectation
// document. The contract for #52 is process-scope-only: the domain
// itself accepts business_service and capability scopes, but apply
// rejects them with an explicit "not supported by control-plane apply
// yet" message because the matching engine in #53 needs to provide the
// additional traversal validators (Surface→Process→BusinessService and
// the M:N Capability link) before those scopes can be admitted.
//
// Cross-document referential integrity (does the Process exist? does the
// Surface belong to the declared Process?) is the apply planner's
// concern, not this validator's. The planner uses the bundle pre-pass
// and the repository to resolve references and emit error messages with
// the appropriate field paths.
//
// condition_payload is opaque to apply: shape validation is the
// matching engine's responsibility (#53). All this validator does for
// the payload is accept it as a YAML map (already enforced by the
// strict-decode of types.GovernanceExpectationSpec).
func validateGovernanceExpectation(doc types.GovernanceExpectationDocument) []types.ValidationError {
	var errs []types.ValidationError

	// metadata.name
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}

	// scope_kind: required, and process-only for #52.
	scopeKind := strings.TrimSpace(doc.Spec.ScopeKind)
	if scopeKind == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.scope_kind"))
	} else if scopeKind != "process" {
		// business_service and capability are domain-valid but
		// apply-rejected in #52. Issue a tailored error so the operator
		// understands this is an apply-side scoping limitation, not a
		// general domain rejection.
		switch scopeKind {
		case "business_service", "capability":
			errs = append(errs, fieldErr(doc, "spec.scope_kind",
				fmt.Sprintf("scope_kind %q is not supported by control-plane apply yet; only \"process\" is supported", scopeKind)))
		default:
			errs = append(errs, enumErr(doc, "spec.scope_kind", scopeKind, ExpectationScopeKindProcessOnly))
		}
	}

	// scope_id: required, ID-format-checked.
	scopeID := strings.TrimSpace(doc.Spec.ScopeID)
	if scopeID == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.scope_id"))
	} else if err := validateIDFormat(scopeID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.scope_id", err.Error()))
	}

	// required_surface_id: required, ID-format-checked.
	requiredSurfaceID := strings.TrimSpace(doc.Spec.RequiredSurfaceID)
	if requiredSurfaceID == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.required_surface_id"))
	} else if err := validateIDFormat(requiredSurfaceID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.required_surface_id", err.Error()))
	}

	// condition_type: required, closed enum.
	conditionType := strings.TrimSpace(doc.Spec.ConditionType)
	if conditionType == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.condition_type"))
	} else if !contains(ValidExpectationConditionTypes, conditionType) {
		errs = append(errs, enumErr(doc, "spec.condition_type", conditionType, ValidExpectationConditionTypes))
	}

	// business_owner: required.
	if strings.TrimSpace(doc.Spec.BusinessOwner) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.business_owner"))
	} else if len(doc.Spec.BusinessOwner) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.business_owner",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	// technical_owner: required.
	if strings.TrimSpace(doc.Spec.TechnicalOwner) == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.technical_owner"))
	} else if len(doc.Spec.TechnicalOwner) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.technical_owner",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	// description: optional, length-bounded.
	if len(doc.Spec.Description) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.description",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	// lifecycle.status: optional; if present, must be one of the 5
	// ExpectationStatus values. Apply forces 'review' on persistence
	// regardless of the value here, but a typo'd status should still
	// be reported as an enum error rather than silently ignored.
	if status := strings.TrimSpace(doc.Spec.Lifecycle.Status); status != "" {
		if !contains(ValidExpectationStatuses, status) {
			errs = append(errs, enumErr(doc, "spec.lifecycle.status", status, ValidExpectationStatuses))
		}
	}

	// lifecycle dates: RFC3339 strings; if both present, until > from.
	var effectiveFrom, effectiveUntil time.Time
	var effectiveFromOK, effectiveUntilOK bool

	if s := strings.TrimSpace(doc.Spec.Lifecycle.EffectiveFrom); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.lifecycle.effective_from",
				"must be a valid RFC3339 timestamp"))
		} else {
			effectiveFrom = parsed
			effectiveFromOK = true
		}
	}

	if s := strings.TrimSpace(doc.Spec.Lifecycle.EffectiveUntil); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.lifecycle.effective_until",
				"must be a valid RFC3339 timestamp"))
		} else {
			effectiveUntil = parsed
			effectiveUntilOK = true
		}
	}

	if effectiveFromOK && effectiveUntilOK && !effectiveUntil.After(effectiveFrom) {
		errs = append(errs, fieldErr(doc, "spec.lifecycle.effective_until",
			"must be after spec.lifecycle.effective_from"))
	}

	// lifecycle.version: optional; if non-zero, must be ≥ 1. The planner
	// authors the persisted version, so a value here is informational.
	// Negative or zero-but-explicit values (which yaml.v3 cannot
	// distinguish from omitempty) we treat as "non-positive" and reject
	// when negative; a literal `version: 0` is indistinguishable from
	// "not supplied" and is therefore allowed through.
	if doc.Spec.Lifecycle.Version < 0 {
		errs = append(errs, fieldErr(doc, "spec.lifecycle.version",
			"must be >= 1 when supplied"))
	}

	return errs
}

// validateAISystem performs per-document validation for AISystem.
// Apply is status-honouring; cross-reference checks (replaces existence)
// are deferred to the planner.
func validateAISystem(doc types.AISystemDocument) []types.ValidationError {
	var errs []types.ValidationError

	if strings.TrimSpace(doc.Metadata.Name) == "" {
		errs = append(errs, requiredFieldErr(doc, "metadata.name"))
	} else if len(doc.Metadata.Name) > MaxNameLength {
		errs = append(errs, fieldErr(doc, "metadata.name",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxNameLength)))
	}

	if status := strings.TrimSpace(doc.Spec.Status); status != "" {
		if !contains(ValidAISystemStatuses, status) {
			errs = append(errs, enumErr(doc, "spec.status", status, ValidAISystemStatuses))
		}
	}

	if origin := strings.TrimSpace(doc.Spec.Origin); origin != "" {
		if !contains(ValidAISystemOrigins, origin) {
			errs = append(errs, enumErr(doc, "spec.origin", origin, ValidAISystemOrigins))
		}
	}

	// Self-replace is rejected at the schema layer (chk_ai_systems_no_self_replace);
	// surface a clean validator-level message rather than a constraint error
	// at apply time.
	if replaces := strings.TrimSpace(doc.Spec.Replaces); replaces != "" {
		if err := validateIDFormat(replaces); err != nil {
			errs = append(errs, fieldErr(doc, "spec.replaces", err.Error()))
		}
		if replaces == strings.TrimSpace(doc.Metadata.ID) {
			errs = append(errs, fieldErr(doc, "spec.replaces",
				"ai system cannot replace itself"))
		}
	}

	if len(doc.Spec.Description) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.description",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	errs = append(errs, validateExternalRefSpec(doc, doc.Spec.ExternalRef)...)

	return errs
}

// validateAISystemVersion performs per-document validation for
// AISystemVersion. Status is honoured (no review-forcing). RFC3339
// timestamp parsing is enforced here so the mapper can rely on
// well-formed input.
func validateAISystemVersion(doc types.AISystemVersionDocument) []types.ValidationError {
	var errs []types.ValidationError

	aiSystemID := strings.TrimSpace(doc.Spec.AISystemID)
	if aiSystemID == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.ai_system_id"))
	} else if err := validateIDFormat(aiSystemID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.ai_system_id", err.Error()))
	}

	if doc.Spec.Version < 1 {
		errs = append(errs, fieldErr(doc, "spec.version",
			"must be an integer >= 1"))
	}

	if status := strings.TrimSpace(doc.Spec.Status); status != "" {
		if !contains(ValidAISystemVersionStatuses, status) {
			errs = append(errs, enumErr(doc, "spec.status", status, ValidAISystemVersionStatuses))
		}
	}

	var effectiveFrom, effectiveUntil time.Time
	var effectiveFromOK, effectiveUntilOK bool

	if s := strings.TrimSpace(doc.Spec.EffectiveFrom); s == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.effective_from"))
	} else {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.effective_from",
				"must be a valid RFC3339 timestamp"))
		} else {
			effectiveFrom = parsed
			effectiveFromOK = true
		}
	}

	if s := strings.TrimSpace(doc.Spec.EffectiveUntil); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			errs = append(errs, fieldErr(doc, "spec.effective_until",
				"must be a valid RFC3339 timestamp"))
		} else {
			effectiveUntil = parsed
			effectiveUntilOK = true
		}
	}

	if effectiveFromOK && effectiveUntilOK && !effectiveUntil.After(effectiveFrom) {
		errs = append(errs, fieldErr(doc, "spec.effective_until",
			"must be strictly after spec.effective_from"))
	}

	if s := strings.TrimSpace(doc.Spec.RetiredAt); s != "" {
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			errs = append(errs, fieldErr(doc, "spec.retired_at",
				"must be a valid RFC3339 timestamp"))
		}
	}

	errs = append(errs, validateExternalRefSpec(doc, doc.Spec.ExternalRef)...)

	return errs
}

// validateAISystemBinding performs per-document validation for
// AISystemBinding. Implements rule 1 of the five cross-reference rules
// (at-least-one-context-reference); the remaining four rules require
// bundle + repo access and live in the apply planner.
func validateAISystemBinding(doc types.AISystemBindingDocument) []types.ValidationError {
	var errs []types.ValidationError

	aiSystemID := strings.TrimSpace(doc.Spec.AISystemID)
	if aiSystemID == "" {
		errs = append(errs, requiredFieldErr(doc, "spec.ai_system_id"))
	} else if err := validateIDFormat(aiSystemID); err != nil {
		errs = append(errs, fieldErr(doc, "spec.ai_system_id", err.Error()))
	}

	if doc.Spec.AISystemVersion != nil && *doc.Spec.AISystemVersion < 1 {
		errs = append(errs, fieldErr(doc, "spec.ai_system_version",
			"must be an integer >= 1 when supplied"))
	}

	bs := strings.TrimSpace(doc.Spec.BusinessServiceID)
	cap := strings.TrimSpace(doc.Spec.CapabilityID)
	proc := strings.TrimSpace(doc.Spec.ProcessID)
	surf := strings.TrimSpace(doc.Spec.SurfaceID)

	if bs == "" && cap == "" && proc == "" && surf == "" {
		errs = append(errs, fieldErr(doc, "spec",
			"binding requires at least one of business_service_id, capability_id, process_id, surface_id"))
	}

	if bs != "" {
		if err := validateIDFormat(bs); err != nil {
			errs = append(errs, fieldErr(doc, "spec.business_service_id", err.Error()))
		}
	}
	if cap != "" {
		if err := validateIDFormat(cap); err != nil {
			errs = append(errs, fieldErr(doc, "spec.capability_id", err.Error()))
		}
	}
	if proc != "" {
		if err := validateIDFormat(proc); err != nil {
			errs = append(errs, fieldErr(doc, "spec.process_id", err.Error()))
		}
	}
	if surf != "" {
		if err := validateIDFormat(surf); err != nil {
			errs = append(errs, fieldErr(doc, "spec.surface_id", err.Error()))
		}
	}

	if len(doc.Spec.Description) > MaxFieldLength {
		errs = append(errs, fieldErr(doc, "spec.description",
			fmt.Sprintf("exceeds maximum length of %d characters", MaxFieldLength)))
	}

	errs = append(errs, validateExternalRefSpec(doc, doc.Spec.ExternalRef)...)

	return errs
}

// validateExternalRefSpec runs document-level validation on an optional
// ExternalRef field (Epic 1, PR 3). Errors are returned with field
// paths prefixed by `spec.external_ref.` so operators can pinpoint the
// offending sub-field without scanning surrounding context.
//
// Rules enforced:
//
//   - source_system and source_id must either both be set or both be empty
//     (mirrors the chk_<table>_ext_consistency Postgres CHECK)
//   - last_synced_at, when present, must be a valid RFC3339 timestamp
//
// Other fields are independently optional and never error here. A nil
// ref is silently accepted (no external reference declared).
func validateExternalRefSpec(doc document, ref *types.ExternalRefSpec) []types.ValidationError {
	if ref == nil {
		return nil
	}
	var errs []types.ValidationError
	system := strings.TrimSpace(ref.SourceSystem)
	id := strings.TrimSpace(ref.SourceID)
	if (system == "") != (id == "") {
		errs = append(errs, fieldErr(doc, "spec.external_ref",
			"source_system and source_id must both be set or both be empty"))
	}
	if s := strings.TrimSpace(ref.LastSyncedAt); s != "" {
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			errs = append(errs, fieldErr(doc, "spec.external_ref.last_synced_at",
				"must be a valid RFC3339 timestamp"))
		}
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
