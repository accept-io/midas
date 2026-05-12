package apply

import (
	"context"
	"fmt"
	"strings"

	"github.com/accept-io/midas/internal/controlplane/types"
)

// checkDriftReferences resolves apply-time cross-document references
// declared in a DriftDefinitionDocument:
//
//   - spec.target.kind + spec.target.id → existence in the matching
//     entity repository (one of the nine V1 entity kinds).
//   - spec.metrics[].governance_expectation_ref → existence in the
//     GovernanceExpectation repository (latest version).
//
// Posture (Drift-1c per brief decision 5):
//
//   - When the relevant repository is nil, the planner degrades to
//     structural-only validation for that target kind. This matches
//     the convention used by every other Kind's planner.
//   - When the repository is present and the target ID does not
//     exist, the entry is marked Invalid with a clear validation
//     error.
//
// GovernanceExpectation reference (Drift-1c per brief decision 6):
//   - Existence-of-latest only. Version-pinned validation is deferred
//     until the apply-side GovernanceExpectationRepository interface
//     is extended with FindByIDAndVersion (a later tranche).
func (s *Service) checkDriftReferences(ctx context.Context, doc types.DriftDefinitionDocument) []types.ValidationError {
	var errs []types.ValidationError

	targetKind := strings.TrimSpace(doc.Spec.Target.Kind)
	targetID := strings.TrimSpace(doc.Spec.Target.ID)

	if targetKind != "" && targetID != "" {
		if e := s.checkDriftTargetEntity(ctx, doc, targetKind, targetID); e != nil {
			errs = append(errs, *e)
		}
	}

	if s.governanceExpectationRepo != nil {
		seen := make(map[string]struct{})
		for _, m := range doc.Spec.Metrics {
			ref := strings.TrimSpace(m.GovernanceExpectationRef)
			if ref == "" {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			ge, err := s.governanceExpectationRepo.FindByID(ctx, ref)
			if err != nil {
				errs = append(errs, types.ValidationError{
					Kind:    types.KindDriftDefinition,
					ID:      doc.Metadata.ID,
					Field:   "spec.metrics[].governance_expectation_ref",
					Message: fmt.Sprintf("repository error resolving governance_expectation_ref %q: %v", ref, err),
				})
				continue
			}
			if ge == nil {
				errs = append(errs, types.ValidationError{
					Kind:    types.KindDriftDefinition,
					ID:      doc.Metadata.ID,
					Field:   "spec.metrics[].governance_expectation_ref",
					Message: fmt.Sprintf("governance_expectation_ref %q does not resolve to a known GovernanceExpectation", ref),
				})
			}
		}
	}

	return errs
}

// checkDriftTargetEntity resolves the target reference against the
// matching apply-side repository. Returns nil when the target exists,
// the repo is unavailable (degrade to structural-only), or the target
// kind has no apply-side repo wired. Returns an error when the repo
// is present and the target does not exist.
func (s *Service) checkDriftTargetEntity(ctx context.Context, doc types.DriftDefinitionDocument, kind, id string) *types.ValidationError {
	missing := func(format string, args ...any) *types.ValidationError {
		return &types.ValidationError{
			Kind:    types.KindDriftDefinition,
			ID:      doc.Metadata.ID,
			Field:   "spec.target.id",
			Message: fmt.Sprintf(format, args...),
		}
	}

	switch kind {
	case "business_service":
		if s.businessServiceRepo == nil {
			return nil
		}
		ok, err := s.businessServiceRepo.Exists(ctx, id)
		if err != nil {
			return missing("repository error resolving target business_service %q: %v", id, err)
		}
		if !ok {
			return missing("target business_service %q does not exist", id)
		}
	case "capability":
		if s.capabilityRepo == nil {
			return nil
		}
		ok, err := s.capabilityRepo.Exists(ctx, id)
		if err != nil {
			return missing("repository error resolving target capability %q: %v", id, err)
		}
		if !ok {
			return missing("target capability %q does not exist", id)
		}
	case "process":
		if s.processRepo == nil {
			return nil
		}
		ok, err := s.processRepo.Exists(ctx, id)
		if err != nil {
			return missing("repository error resolving target process %q: %v", id, err)
		}
		if !ok {
			return missing("target process %q does not exist", id)
		}
	case "decision_surface":
		if s.surfaceRepo == nil {
			return nil
		}
		surf, err := s.surfaceRepo.FindLatestByID(ctx, id)
		if err != nil {
			return missing("repository error resolving target decision_surface %q: %v", id, err)
		}
		if surf == nil {
			return missing("target decision_surface %q does not exist", id)
		}
	case "ai_system":
		if s.aiSystemRepo == nil {
			return nil
		}
		ok, err := s.aiSystemRepo.Exists(ctx, id)
		if err != nil {
			return missing("repository error resolving target ai_system %q: %v", id, err)
		}
		if !ok {
			return missing("target ai_system %q does not exist", id)
		}
	case "ai_system_binding":
		if s.aiBindingRepo == nil {
			return nil
		}
		b, err := s.aiBindingRepo.GetByID(ctx, id)
		if err != nil {
			return missing("repository error resolving target ai_system_binding %q: %v", id, err)
		}
		if b == nil {
			return missing("target ai_system_binding %q does not exist", id)
		}
	case "agent":
		if s.agentRepo == nil {
			return nil
		}
		a, err := s.agentRepo.GetByID(ctx, id)
		if err != nil {
			return missing("repository error resolving target agent %q: %v", id, err)
		}
		if a == nil {
			return missing("target agent %q does not exist", id)
		}
	case "authority_profile":
		if s.profileRepo == nil {
			return nil
		}
		p, err := s.profileRepo.FindByID(ctx, id)
		if err != nil {
			return missing("repository error resolving target authority_profile %q: %v", id, err)
		}
		if p == nil {
			return missing("target authority_profile %q does not exist", id)
		}
	case "authority_grant":
		if s.grantRepo == nil {
			return nil
		}
		g, err := s.grantRepo.FindByID(ctx, id)
		if err != nil {
			return missing("repository error resolving target authority_grant %q: %v", id, err)
		}
		if g == nil {
			return missing("target authority_grant %q does not exist", id)
		}
	}
	return nil
}
