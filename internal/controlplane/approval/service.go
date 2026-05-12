package approval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/controlaudit"
	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/surface"
)

var (
	ErrSurfaceNotFound                  = errors.New("surface not found")
	ErrApprovalForbidden                = errors.New("approval forbidden")
	ErrInvalidStatus                    = errors.New("surface is not awaiting approval")
	ErrInvalidTransition                = errors.New("transition not permitted")
	ErrProfileNotFound                  = errors.New("profile not found")
	ErrProfileNotInReview               = errors.New("profile is not in review state")
	ErrProfileNotActive                 = errors.New("profile is not in active state")
	ErrGovernanceExpectationNotFound    = errors.New("governance expectation not found")
	ErrGovernanceExpectationNotInReview = errors.New("governance expectation is not in review state")
	// FailModePolicy lifecycle sentinels (D27j-impl-1c). The HTTP layer
	// translates these to 404 / 409 via mapApprovalError.
	ErrFailModePolicyNotFound    = errors.New("fail mode policy not found")
	ErrFailModePolicyNotInReview = errors.New("fail mode policy is not in review state")
	ErrFailModePolicyNotActive   = errors.New("fail mode policy is not in active state")

	// DriftDefinition lifecycle sentinels (Drift-1e). The HTTP layer
	// translates these to 404 / 409 / 403 via mapApprovalError.
	// ErrDriftDefinitionInvalidTransition fires when the source state
	// is wrong for the requested action (e.g. submit on a non-draft
	// revision). The narrower NotIn{Draft,Review,Active} sentinels
	// are reserved for actions whose source state is uniquely defined.
	ErrDriftDefinitionNotFound          = errors.New("drift definition not found")
	ErrDriftDefinitionNotInDraft        = errors.New("drift definition is not in draft state")
	ErrDriftDefinitionNotInReview       = errors.New("drift definition is not in review state")
	ErrDriftDefinitionNotActive         = errors.New("drift definition is not in active state")
	ErrDriftDefinitionAlreadyRetired    = errors.New("drift definition is already retired")
	ErrDriftDefinitionMakerChecker      = errors.New("approver must not be the same actor that created or last updated this revision")
	ErrDriftDefinitionInvalidTransition = errors.New("drift definition lifecycle transition not permitted from current state")
)

type SurfaceRepository interface {
	FindLatestByID(ctx context.Context, id string) (*surface.DecisionSurface, error)
	Update(ctx context.Context, s *surface.DecisionSurface) error
}

// ProfileRepository is the minimal read/write interface required by profile lifecycle operations.
type ProfileRepository interface {
	FindByIDAndVersion(ctx context.Context, id string, version int) (*authority.AuthorityProfile, error)
	Update(ctx context.Context, p *authority.AuthorityProfile) error
}

// ExpectationRepository is the minimal read/write interface required by
// GovernanceExpectation lifecycle operations (#57). Mirrors
// ProfileRepository in shape — versioned (id, version) lookup +
// lifecycle/audit-only Update — because GovernanceExpectation's
// versioning posture, narrow lifecycle graph, and Update field set all
// match Profile's exactly.
type ExpectationRepository interface {
	FindByIDAndVersion(ctx context.Context, id string, version int) (*governanceexpectation.GovernanceExpectation, error)
	Update(ctx context.Context, e *governanceexpectation.GovernanceExpectation) error
}

// FailModePolicyRepository is the minimal read/write interface required
// by FailModePolicy lifecycle operations (D27j-impl-1c). Mirrors
// ProfileRepository: versioned (id, version) lookup + lifecycle/audit-
// only Update.
type FailModePolicyRepository interface {
	FindByIDAndVersion(ctx context.Context, id string, version int) (*failmode.FailModePolicy, error)
	Update(ctx context.Context, p *failmode.FailModePolicy) error
}

// DriftDefinitionRepository is the minimal read/write interface
// required by DriftDefinition lifecycle operations (Drift-1e). Mirrors
// ProfileRepository / FailModePolicyRepository in shape: versioned
// (id, version) lookup + lifecycle/audit-only Update. Drift-1b's
// Update method mutates only parent lifecycle / approval / successor
// fields; child metric rows are immutable within a revision (atomic-
// revision invariant), so the lifecycle service never needs metric
// access here.
type DriftDefinitionRepository interface {
	FindByIDAndVersion(ctx context.Context, id string, version int) (*drift.DriftDefinition, error)
	Update(ctx context.Context, d *drift.DriftDefinition) error
}

// Service orchestrates surface lifecycle governance: approval and deprecation.
//
// If an outbox.Repository is provided (via NewServiceWithOutbox), a surface.approved
// or surface.deprecated event is appended in the same call sequence as the
// repository Update. For transactional atomicity, the SurfaceRepository and the
// outbox.Repository must be bound to the same database transaction by the caller.
//
// If a controlaudit.Repository is provided, a control-plane audit record is
// appended after each successful lifecycle transition.
type Service struct {
	repo                SurfaceRepository
	profileRepo         ProfileRepository         // nil-safe: profile operations unavailable if nil
	expectationRepo     ExpectationRepository     // nil-safe: GE operations unavailable if nil
	failModePolicyRepo  FailModePolicyRepository  // nil-safe: FailModePolicy operations unavailable if nil
	driftDefinitionRepo DriftDefinitionRepository // nil-safe: DriftDefinition lifecycle unavailable if nil
	policy              Policy
	outbox              outbox.Repository       // nil-safe: no event emitted if nil
	controlAudit        controlaudit.Repository // nil-safe: no audit record if nil
}

// NewService constructs a Service without outbox emission. Existing callers
// are unaffected; surface lifecycle transitions produce no outbox events.
func NewService(repo SurfaceRepository, policy Policy) *Service {
	return &Service{
		repo:   repo,
		policy: policy,
	}
}

// NewServiceWithOutbox constructs a Service that emits surface.approved and
// surface.deprecated outbox events via outboxRepo after each successful update.
// outboxRepo must be bound to the same transaction as repo for atomic delivery.
func NewServiceWithOutbox(repo SurfaceRepository, policy Policy, outboxRepo outbox.Repository) *Service {
	return &Service{
		repo:   repo,
		policy: policy,
		outbox: outboxRepo,
	}
}

// NewServiceWithAll constructs a Service with outbox and control-plane audit repositories.
// Either may be nil; nil repositories are no-ops.
func NewServiceWithAll(repo SurfaceRepository, policy Policy, outboxRepo outbox.Repository, controlAuditRepo controlaudit.Repository) *Service {
	return &Service{
		repo:         repo,
		policy:       policy,
		outbox:       outboxRepo,
		controlAudit: controlAuditRepo,
	}
}

// NewServiceWithProfile constructs a Service with both surface and profile repositories.
// This is the constructor to use when profile lifecycle governance (approve/deprecate) is needed.
func NewServiceWithProfile(repo SurfaceRepository, profileRepo ProfileRepository, policy Policy) *Service {
	return &Service{
		repo:        repo,
		profileRepo: profileRepo,
		policy:      policy,
	}
}

// NewServiceWithProfileAndOutbox constructs a fully-wired Service supporting
// surface and profile lifecycle governance with outbox event emission.
func NewServiceWithProfileAndOutbox(repo SurfaceRepository, profileRepo ProfileRepository, policy Policy, outboxRepo outbox.Repository, controlAuditRepo controlaudit.Repository) *Service {
	return &Service{
		repo:         repo,
		profileRepo:  profileRepo,
		policy:       policy,
		outbox:       outboxRepo,
		controlAudit: controlAuditRepo,
	}
}

// WithExpectationRepository injects the GovernanceExpectation
// repository used by ApproveGovernanceExpectation (#57). When nil, GE
// approval is unavailable — the method returns an error explaining the
// repository is not configured. Returns the receiver for chaining,
// matching the orchestrator's WithCoverageService pattern from #54.
//
// Existing call sites that don't opt in stay green: GE approval is
// strictly additive over the surface/profile lifecycle the existing
// constructors already wire.
func (s *Service) WithExpectationRepository(repo ExpectationRepository) *Service {
	s.expectationRepo = repo
	return s
}

// WithFailModePolicyRepository injects the FailModePolicy repository used
// by ApproveFailModePolicy / DeprecateFailModePolicy (D27j-impl-1c). When
// nil, FailModePolicy lifecycle methods return an error explaining the
// repository is not configured. Returns the receiver for chaining,
// matching the WithExpectationRepository builder.
func (s *Service) WithFailModePolicyRepository(repo FailModePolicyRepository) *Service {
	s.failModePolicyRepo = repo
	return s
}

// WithDriftDefinitionRepository injects the DriftDefinition repository
// used by Drift-1e lifecycle methods (SubmitDriftDefinition,
// ApproveDriftDefinition, RejectDriftDefinition,
// DeprecateDriftDefinition, RetireDriftDefinition). When nil, the
// methods return an error explaining the repository is not configured.
// Returns the receiver for chaining.
func (s *Service) WithDriftDefinitionRepository(repo DriftDefinitionRepository) *Service {
	s.driftDefinitionRepo = repo
	return s
}

// appendControlAudit appends a control-plane audit record. It is a no-op when
// the controlAudit repository is nil.
func (s *Service) appendControlAudit(ctx context.Context, rec *controlaudit.ControlAuditRecord) {
	if s.controlAudit == nil {
		return
	}
	_ = s.controlAudit.Append(ctx, rec)
}

// appendSurfaceApprovedEvent appends a surface.approved outbox event using the
// typed contract builder. It is a no-op when s.outbox is nil, preserving
// existing behaviour for callers that do not configure an outbox.
func (s *Service) appendSurfaceApprovedEvent(
	ctx context.Context,
	surf *surface.DecisionSurface,
) error {
	if s.outbox == nil {
		return nil
	}
	payload, err := outbox.BuildSurfaceApprovedEvent(surf.ID, surf.ApprovedBy)
	if err != nil {
		return fmt.Errorf("build outbox payload surface.approved: %w", err)
	}
	ev, err := outbox.New(
		outbox.EventSurfaceApproved,
		"surface",
		surf.ID,
		"midas.surfaces",
		surf.ID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("construct outbox event surface.approved: %w", err)
	}
	return s.outbox.Append(ctx, ev)
}

// appendSurfaceDeprecatedEvent appends a surface.deprecated outbox event using
// the typed contract builder. It is a no-op when s.outbox is nil.
func (s *Service) appendSurfaceDeprecatedEvent(
	ctx context.Context,
	surf *surface.DecisionSurface,
	deprecatedBy string,
) error {
	if s.outbox == nil {
		return nil
	}
	payload, err := outbox.BuildSurfaceDeprecatedEvent(surf.ID, deprecatedBy)
	if err != nil {
		return fmt.Errorf("build outbox payload surface.deprecated: %w", err)
	}
	ev, err := outbox.New(
		outbox.EventSurfaceDeprecated,
		"surface",
		surf.ID,
		"midas.surfaces",
		surf.ID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("construct outbox event surface.deprecated: %w", err)
	}
	return s.outbox.Append(ctx, ev)
}

// ApproveSurface promotes a surface from review to active.
//
// The caller supplies the submitter (who applied the surface) and the approver
// (who is authorising it). The approval policy determines whether the approver
// is permitted to approve the surface given those identities.
//
// Only surfaces in review status may be approved. Surfaces in any other status
// return ErrInvalidStatus.
func (s *Service) ApproveSurface(ctx context.Context, surfaceID string, submitter identity.Principal, approver identity.Principal) (*surface.DecisionSurface, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("approval repository not configured")
	}

	current, err := s.repo.FindLatestByID(ctx, surfaceID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrSurfaceNotFound
	}

	// Only surfaces in review status may be promoted to active.
	// Draft surfaces must be submitted (transitioned to review) before approval.
	if current.Status != surface.SurfaceStatusReview {
		return nil, ErrInvalidStatus
	}

	if err := surface.ValidateLifecycleTransition(current.Status, surface.SurfaceStatusActive); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
	}

	if !CanApproveSurface(s.policy, submitter, approver, current) {
		return nil, ErrApprovalForbidden
	}

	now := time.Now().UTC()
	current.Status = surface.SurfaceStatusActive
	current.ApprovedBy = approver.ID
	current.ApprovedAt = &now

	// If not already set, make the surface effective immediately on activation.
	if current.EffectiveFrom.IsZero() {
		current.EffectiveFrom = now
	}
	current.UpdatedAt = now

	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}

	if err := s.appendSurfaceApprovedEvent(ctx, current); err != nil {
		return nil, fmt.Errorf("outbox append surface.approved: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewSurfaceApprovedRecord(approver.ID, current.ID, current.Version))

	return current, nil
}

// DeprecateSurface transitions a surface from active to deprecated.
//
// The caller supplies the deprecatedBy actor (who is initiating the deprecation),
// a reason for deprecation, and an optional successor surface ID.
//
// Only surfaces in active status may be deprecated.
func (s *Service) DeprecateSurface(ctx context.Context, surfaceID string, deprecatedBy string, reason string, successorID string) (*surface.DecisionSurface, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("approval repository not configured")
	}

	current, err := s.repo.FindLatestByID(ctx, surfaceID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrSurfaceNotFound
	}

	if current.Status != surface.SurfaceStatusActive {
		return nil, fmt.Errorf("%w: surface must be active to deprecate (current status: %s)", ErrInvalidTransition, current.Status)
	}

	if err := surface.ValidateLifecycleTransition(current.Status, surface.SurfaceStatusDeprecated); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
	}

	now := time.Now().UTC()
	current.Status = surface.SurfaceStatusDeprecated
	current.DeprecationReason = reason
	current.SuccessorSurfaceID = successorID
	current.UpdatedAt = now

	if err := s.repo.Update(ctx, current); err != nil {
		return nil, err
	}

	if err := s.appendSurfaceDeprecatedEvent(ctx, current, deprecatedBy); err != nil {
		return nil, fmt.Errorf("outbox append surface.deprecated: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewSurfaceDeprecatedRecord(deprecatedBy, current.ID, current.Version, reason, successorID))

	return current, nil
}

// appendProfileApprovedEvent appends a profile.approved outbox event.
// It is a no-op when s.outbox is nil.
func (s *Service) appendProfileApprovedEvent(ctx context.Context, p *authority.AuthorityProfile) error {
	if s.outbox == nil {
		return nil
	}
	payload, err := outbox.BuildProfileApprovedEvent(p.ID, p.SurfaceID, p.ApprovedBy)
	if err != nil {
		return fmt.Errorf("build outbox payload profile.approved: %w", err)
	}
	ev, err := outbox.New(
		outbox.EventProfileApproved,
		"profile",
		p.ID,
		"midas.profiles",
		p.ID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("construct outbox event profile.approved: %w", err)
	}
	return s.outbox.Append(ctx, ev)
}

// appendProfileDeprecatedEvent appends a profile.deprecated outbox event.
// It is a no-op when s.outbox is nil.
func (s *Service) appendProfileDeprecatedEvent(ctx context.Context, p *authority.AuthorityProfile, deprecatedBy string) error {
	if s.outbox == nil {
		return nil
	}
	payload, err := outbox.BuildProfileDeprecatedEvent(p.ID, p.SurfaceID, deprecatedBy)
	if err != nil {
		return fmt.Errorf("build outbox payload profile.deprecated: %w", err)
	}
	ev, err := outbox.New(
		outbox.EventProfileDeprecated,
		"profile",
		p.ID,
		"midas.profiles",
		p.ID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("construct outbox event profile.deprecated: %w", err)
	}
	return s.outbox.Append(ctx, ev)
}

// ApproveProfile promotes a profile from review to active.
//
// Only profiles in review status may be approved. Profiles in any other status
// return ErrProfileNotInReview. The approver's identity is captured on the profile record.
func (s *Service) ApproveProfile(ctx context.Context, profileID string, version int, approvedBy string) (*authority.AuthorityProfile, error) {
	if s.profileRepo == nil {
		return nil, fmt.Errorf("profile repository not configured")
	}

	current, err := s.profileRepo.FindByIDAndVersion(ctx, profileID, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrProfileNotFound
	}

	if !current.CanTransitionTo(authority.ProfileStatusActive) {
		return nil, ErrProfileNotInReview
	}

	now := time.Now().UTC()
	current.Status = authority.ProfileStatusActive
	current.ApprovedBy = approvedBy
	current.ApprovedAt = &now
	current.UpdatedAt = now

	// If not already set, make the profile effective immediately on activation.
	if current.EffectiveDate.IsZero() {
		current.EffectiveDate = now
	}

	if err := s.profileRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	if err := s.appendProfileApprovedEvent(ctx, current); err != nil {
		return nil, fmt.Errorf("outbox append profile.approved: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewProfileApprovedRecord(approvedBy, current.ID, current.Version))

	return current, nil
}

// DeprecateProfile transitions an active profile to deprecated status.
//
// Only profiles in active status may be deprecated. Profiles in any other status
// return ErrProfileNotActive.
func (s *Service) DeprecateProfile(ctx context.Context, profileID string, version int, deprecatedBy string) (*authority.AuthorityProfile, error) {
	if s.profileRepo == nil {
		return nil, fmt.Errorf("profile repository not configured")
	}

	current, err := s.profileRepo.FindByIDAndVersion(ctx, profileID, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrProfileNotFound
	}

	if !current.CanTransitionTo(authority.ProfileStatusDeprecated) {
		return nil, ErrProfileNotActive
	}

	now := time.Now().UTC()
	current.Status = authority.ProfileStatusDeprecated
	current.UpdatedAt = now

	if err := s.profileRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	if err := s.appendProfileDeprecatedEvent(ctx, current, deprecatedBy); err != nil {
		return nil, fmt.Errorf("outbox append profile.deprecated: %w", err)
	}

	s.appendControlAudit(ctx, controlaudit.NewProfileDeprecatedRecord(deprecatedBy, current.ID, current.Version))

	return current, nil
}

// ApproveGovernanceExpectation promotes a GovernanceExpectation version
// from review to active (#57). Mirrors ApproveProfile in shape —
// versioned (id, version) lookup, CanTransitionTo gate, lifecycle/audit
// fields persisted via the repository's narrow Update, control-audit
// record emitted after a successful update.
//
// Only expectations in review status may be approved. Other states
// return ErrGovernanceExpectationNotInReview (deprecated, retired, or
// the unreachable draft / active states all share this error so the
// HTTP layer can map the 409 uniformly).
//
// approvedBy is recorded on the row's ApprovedBy field. Tests and the
// HTTP layer derive it from the authenticated principal (or a
// body-supplied fallback) before calling this method — same posture as
// ApproveProfile.
//
// No outbox event is emitted today; #57's brief defers it until a
// downstream consumer exists. The control-audit record is the source of
// truth for "this expectation was approved" until that consumer arrives.
func (s *Service) ApproveGovernanceExpectation(
	ctx context.Context,
	id string,
	version int,
	approvedBy string,
) (*governanceexpectation.GovernanceExpectation, error) {
	if s.expectationRepo == nil {
		return nil, fmt.Errorf("governance expectation repository not configured")
	}

	current, err := s.expectationRepo.FindByIDAndVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrGovernanceExpectationNotFound
	}

	if !current.CanTransitionTo(governanceexpectation.ExpectationStatusActive) {
		return nil, ErrGovernanceExpectationNotInReview
	}

	now := time.Now().UTC()
	current.Status = governanceexpectation.ExpectationStatusActive
	current.ApprovedBy = approvedBy
	current.ApprovedAt = &now
	current.UpdatedAt = now

	// Defensive fallback mirroring ApproveProfile. The apply mapper
	// (#52) always sets a non-zero EffectiveDate; this branch covers
	// any future code path that bypasses apply.
	if current.EffectiveDate.IsZero() {
		current.EffectiveDate = now
	}

	if err := s.expectationRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewGovernanceExpectationApprovedRecord(approvedBy, current.ID, current.Version))

	return current, nil
}

// ---------------------------------------------------------------------------
// FailModePolicy lifecycle (D27j-impl-1c)
// ---------------------------------------------------------------------------

// ApproveFailModePolicy promotes a FailModePolicy version from review to
// active. Mirrors ApproveProfile in shape — versioned (id, version)
// lookup, CanTransitionTo gate, lifecycle/audit fields persisted via the
// repository's narrow Update, control-audit record emitted after a
// successful update.
//
// Only policies in review status may be approved. Other states return
// ErrFailModePolicyNotInReview, mapped by the HTTP layer to 409.
//
// approvedBy is recorded on the row's ApprovedBy field. Tests and the
// HTTP layer derive it from the authenticated principal (or a body-
// supplied fallback) before calling this method, mirroring
// ApproveProfile.
//
// No outbox event is emitted: failmode_policy.approved has no consumer
// today. The control-audit record is the source of truth for "this
// policy was approved" until that consumer arrives.
func (s *Service) ApproveFailModePolicy(
	ctx context.Context,
	policyID string,
	version int,
	approvedBy string,
) (*failmode.FailModePolicy, error) {
	if s.failModePolicyRepo == nil {
		return nil, fmt.Errorf("fail mode policy repository not configured")
	}

	current, err := s.failModePolicyRepo.FindByIDAndVersion(ctx, policyID, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrFailModePolicyNotFound
	}

	if !current.CanTransitionTo(failmode.FailModePolicyStatusActive) {
		return nil, ErrFailModePolicyNotInReview
	}

	now := time.Now().UTC()
	current.Status = failmode.FailModePolicyStatusActive
	current.ApprovedBy = approvedBy
	current.ApprovedAt = &now
	current.UpdatedAt = now

	// Defensive fallback mirroring ApproveProfile / ApproveGovernanceExpectation.
	// The apply mapper (D27j-impl-1b) always sets a non-zero EffectiveDate;
	// this branch covers any future code path that bypasses apply.
	if current.EffectiveDate.IsZero() {
		current.EffectiveDate = now
	}

	if err := s.failModePolicyRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewFailModePolicyApprovedRecord(approvedBy, current.ID, current.Version))

	return current, nil
}

// DeprecateFailModePolicy transitions an active FailModePolicy version to
// deprecated. Mirrors DeprecateProfile in shape, with one difference:
// failmode.FailModePolicy has no DeprecationReason field, so the
// operator-supplied reason is captured only in the control-audit record
// (Metadata.DeprecationReason) and not on the persisted policy row.
//
// Only policies in active status may be deprecated. Other states return
// ErrFailModePolicyNotActive, mapped by the HTTP layer to 409.
func (s *Service) DeprecateFailModePolicy(
	ctx context.Context,
	policyID string,
	version int,
	deprecatedBy string,
	reason string,
) (*failmode.FailModePolicy, error) {
	if s.failModePolicyRepo == nil {
		return nil, fmt.Errorf("fail mode policy repository not configured")
	}

	current, err := s.failModePolicyRepo.FindByIDAndVersion(ctx, policyID, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrFailModePolicyNotFound
	}

	if !current.CanTransitionTo(failmode.FailModePolicyStatusDeprecated) {
		return nil, ErrFailModePolicyNotActive
	}

	now := time.Now().UTC()
	current.Status = failmode.FailModePolicyStatusDeprecated
	current.UpdatedAt = now

	if err := s.failModePolicyRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewFailModePolicyDeprecatedRecord(deprecatedBy, current.ID, current.Version, reason))

	return current, nil
}

// ---------------------------------------------------------------------
// DriftDefinition lifecycle (Drift-1e)
// ---------------------------------------------------------------------
//
// Five lifecycle actions: submit, approve, reject, deprecate, retire.
// Each method:
//
//   - looks up the (id, version) pair via the narrow repository
//   - validates the source state against drift's lifecycle graph
//   - mutates only parent lifecycle / approval / successor fields
//   - never touches embedded DriftMetricDefinition rows
//   - emits a control-audit record after a successful update
//
// Drift-1e mirrors the FailModePolicy / Profile / GovernanceExpectation
// posture: the repository's Update is single-row; cross-version atomic
// deprecation (deprecate prior active when activating a new revision)
// is NOT performed in this tranche, matching every other versioned
// approval Kind. Operators must explicitly deprecate prior active
// revisions through the deprecate endpoint.

// SubmitDriftDefinition transitions a DriftDefinition revision from
// draft to review. Allowed source state: draft. The reason is captured
// on the control-audit record only (drift.DriftDefinition has no
// dedicated submission_reason column).
func (s *Service) SubmitDriftDefinition(
	ctx context.Context,
	id string,
	version int,
	submittedBy string,
	reason string,
) (*drift.DriftDefinition, error) {
	if s.driftDefinitionRepo == nil {
		return nil, fmt.Errorf("drift definition repository not configured")
	}

	current, err := s.driftDefinitionRepo.FindByIDAndVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrDriftDefinitionNotFound
	}

	if current.Status != drift.DriftDefinitionStatusDraft {
		return nil, ErrDriftDefinitionNotInDraft
	}
	if !current.CanTransitionTo(drift.DriftDefinitionStatusReview) {
		return nil, ErrDriftDefinitionInvalidTransition
	}

	now := time.Now().UTC()
	current.Status = drift.DriftDefinitionStatusReview
	current.UpdatedAt = now

	if err := s.driftDefinitionRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewDriftDefinitionSubmittedRecord(submittedBy, current.ID, current.Version, reason))

	return current, nil
}

// ApproveDriftDefinition transitions a DriftDefinition revision from
// review to active. Maker-checker is enforced at the service layer:
// the approver must not equal the revision's CreatedBy field
// (case-insensitive trim). Allowed source state: review.
func (s *Service) ApproveDriftDefinition(
	ctx context.Context,
	id string,
	version int,
	approvedBy string,
) (*drift.DriftDefinition, error) {
	if s.driftDefinitionRepo == nil {
		return nil, fmt.Errorf("drift definition repository not configured")
	}

	current, err := s.driftDefinitionRepo.FindByIDAndVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrDriftDefinitionNotFound
	}

	if current.Status != drift.DriftDefinitionStatusReview {
		return nil, ErrDriftDefinitionNotInReview
	}
	if !current.CanTransitionTo(drift.DriftDefinitionStatusActive) {
		return nil, ErrDriftDefinitionInvalidTransition
	}

	// Maker-checker: trim and case-insensitive compare.
	approver := strings.ToLower(strings.TrimSpace(approvedBy))
	maker := strings.ToLower(strings.TrimSpace(current.CreatedBy))
	if approver != "" && maker != "" && approver == maker {
		return nil, ErrDriftDefinitionMakerChecker
	}

	now := time.Now().UTC()
	current.Status = drift.DriftDefinitionStatusActive
	current.ApprovedBy = approvedBy
	current.ApprovedAt = &now
	current.UpdatedAt = now

	// Defensive fallback mirroring ApproveProfile / ApproveFailModePolicy /
	// ApproveGovernanceExpectation. The apply mapper always sets a non-zero
	// EffectiveDate; this branch covers any future code path that bypasses
	// apply.
	if current.EffectiveDate.IsZero() {
		current.EffectiveDate = now
	}

	if err := s.driftDefinitionRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewDriftDefinitionApprovedRecord(approvedBy, current.ID, current.Version))

	return current, nil
}

// RejectDriftDefinition transitions a DriftDefinition revision from
// review back to draft. Allowed source state: review. The reason is
// captured on the control-audit record only; approved_by and
// approved_at are left unchanged (they are already empty in review
// state).
func (s *Service) RejectDriftDefinition(
	ctx context.Context,
	id string,
	version int,
	rejectedBy string,
	reason string,
) (*drift.DriftDefinition, error) {
	if s.driftDefinitionRepo == nil {
		return nil, fmt.Errorf("drift definition repository not configured")
	}

	current, err := s.driftDefinitionRepo.FindByIDAndVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrDriftDefinitionNotFound
	}

	if current.Status != drift.DriftDefinitionStatusReview {
		return nil, ErrDriftDefinitionNotInReview
	}
	if !current.CanTransitionTo(drift.DriftDefinitionStatusDraft) {
		return nil, ErrDriftDefinitionInvalidTransition
	}

	now := time.Now().UTC()
	current.Status = drift.DriftDefinitionStatusDraft
	current.UpdatedAt = now

	if err := s.driftDefinitionRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewDriftDefinitionRejectedRecord(rejectedBy, current.ID, current.Version, reason))

	return current, nil
}

// DeprecateDriftDefinition transitions a DriftDefinition revision from
// active to deprecated. Allowed source state: active. Successor
// information (when supplied) is persisted on the row's
// SuccessorDefinitionID and SuccessorVersion fields.
//
// Note: this method does NOT auto-activate any revision; activation
// flows through ApproveDriftDefinition. The successor fields here are
// metadata pointing at a separately-approved revision.
func (s *Service) DeprecateDriftDefinition(
	ctx context.Context,
	id string,
	version int,
	deprecatedBy string,
	reason string,
	successorDefinitionID string,
	successorVersion int,
) (*drift.DriftDefinition, error) {
	if s.driftDefinitionRepo == nil {
		return nil, fmt.Errorf("drift definition repository not configured")
	}

	current, err := s.driftDefinitionRepo.FindByIDAndVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrDriftDefinitionNotFound
	}

	if current.Status != drift.DriftDefinitionStatusActive {
		return nil, ErrDriftDefinitionNotActive
	}
	if !current.CanTransitionTo(drift.DriftDefinitionStatusDeprecated) {
		return nil, ErrDriftDefinitionInvalidTransition
	}

	now := time.Now().UTC()
	current.Status = drift.DriftDefinitionStatusDeprecated
	current.UpdatedAt = now
	if succID := strings.TrimSpace(successorDefinitionID); succID != "" {
		current.SuccessorDefinitionID = succID
	}
	if successorVersion > 0 {
		current.SuccessorVersion = successorVersion
	}

	if err := s.driftDefinitionRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewDriftDefinitionDeprecatedRecord(deprecatedBy, current.ID, current.Version, reason))

	return current, nil
}

// RetireDriftDefinition transitions a DriftDefinition revision to
// retired. Allowed source states: draft, review, active, deprecated.
// Retired is terminal — this method rejects retired sources with
// ErrDriftDefinitionAlreadyRetired.
func (s *Service) RetireDriftDefinition(
	ctx context.Context,
	id string,
	version int,
	retiredBy string,
	reason string,
) (*drift.DriftDefinition, error) {
	if s.driftDefinitionRepo == nil {
		return nil, fmt.Errorf("drift definition repository not configured")
	}

	current, err := s.driftDefinitionRepo.FindByIDAndVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrDriftDefinitionNotFound
	}

	if current.Status == drift.DriftDefinitionStatusRetired {
		return nil, ErrDriftDefinitionAlreadyRetired
	}
	if !current.CanTransitionTo(drift.DriftDefinitionStatusRetired) {
		return nil, ErrDriftDefinitionInvalidTransition
	}

	now := time.Now().UTC()
	current.Status = drift.DriftDefinitionStatusRetired
	current.UpdatedAt = now
	current.RetiredAt = &now

	if err := s.driftDefinitionRepo.Update(ctx, current); err != nil {
		return nil, err
	}

	s.appendControlAudit(ctx, controlaudit.NewDriftDefinitionRetiredRecord(retiredBy, current.ID, current.Version, reason))

	return current, nil
}
