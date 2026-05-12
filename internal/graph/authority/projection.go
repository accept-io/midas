// Package authoritygraph projects authority-flavoured domain entities
// — business services, decision surfaces, authority profiles,
// authority grants, agents, and fail-mode policies — into a generic
// node/edge graph rooted at a single business service.
//
// This is the Authority Graph that powers GET /v1/graphs/authority.
// It answers the operator question:
//
//   "For this business service, which decision surfaces are governed
//   by which authority profiles, which grants, which agents, and
//   which fail-mode policies?"
//
// The Authority Graph is deliberately distinct from the Context Graph
// in /v1/graphs/context. The Context Graph projects business and
// operational context (capabilities, processes, AI systems, AI
// system bindings). The Authority Graph projects the authority spine
// (profile → grant → agent) plus fail-mode posture. No data is
// shared between the two projections; clients consume whichever
// surface fits their question.
//
// Process nodes are intentionally NOT projected by the Authority
// Graph — the BusinessService → Surface relationship is collapsed
// into a single edge for brevity. The surface node's typed data
// still carries process_id for traceability.
//
// Each Node kind carries an optional typed data pointer
// (Node.BusinessService, Node.AuthorityProfile, etc.). Exactly one
// of the typed pointers is populated per node, matched to Kind.
// Unused pointers are omitted from the wire payload via
// omitempty, so a Node serialises to a compact shape with only
// its kind's data block.
//
// Directory: internal/graph/authority/. Package declaration is
// "authoritygraph" to avoid collision with the existing
// internal/authority/ domain package.
package authoritygraph

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/accept-io/midas/internal/value"
)

// View identifies the perspective the projection is computed in.
//
// MVP supports only ViewService — root at a business service, walk
// the authority spine through surfaces. Other view kinds (agent,
// surface) are reserved for later tranches.
const (
	ViewService = "service"
)

// Depth bounds. ParseDepth applies these defaults and clamps; Service
// re-clamps defensively for callers that bypass ParseDepth.
//
// DefaultDepth is intentionally 4 (one higher than the Context
// Graph's 3) so that the default service view reaches the agent
// nodes at the end of the BS → Surface → Profile → Grant → Agent
// chain. MaxDepth matches Context Graph for cross-graph consistency.
const (
	DefaultDepth = 4
	MaxDepth     = 5
)

// Typed errors — the handler maps each to the documented status:
//
//	ErrInvalidView   → 400
//	ErrInvalidID     → 400
//	ErrInvalidDepth  → 400
//	ErrNotFound      → 404
//
// Wrap with fmt.Errorf("...: %w", err) when adding context.
var (
	ErrInvalidView  = errors.New("authoritygraph: invalid view")
	ErrInvalidID    = errors.New("authoritygraph: invalid id")
	ErrInvalidDepth = errors.New("authoritygraph: invalid depth")
	ErrNotFound     = errors.New("authoritygraph: root entity not found")

	// ErrEscalationTargetReaderNotConfigured signals that the
	// projection encountered a non-empty AuthorityProfile.EscalationTargetID
	// but no EscalationTargetResolver was supplied via Readers. The
	// projector returns a wrapped error rather than silently dropping
	// the target so wiring regressions surface loudly. Production
	// wiring lives in cmd/midas/main.go; the seeded test runs through
	// the same code path.
	ErrEscalationTargetReaderNotConfigured = errors.New("authoritygraph: escalation target reader not configured")
)

// Authority Graph node kinds — the complete allowed set. The
// Authority Graph emits no other kinds; the absence of process,
// ai_system, ai_system_binding, authority_summary, coverage,
// stop_authority, delegation, evidence_envelope, audit_event,
// escalation_policy, escalation_rule, and any knowledge-graph
// kinds is the structural guard for this package.
//
// D31l adds escalation_target so operators can see WHERE an
// authority profile escalates to. EscalationPolicy / EscalationRule
// remain deferred — the graph projects the resolved target only.
const (
	NodeKindBusinessService   = "business_service"
	NodeKindDecisionSurface   = "decision_surface"
	NodeKindAuthorityProfile  = "authority_profile"
	NodeKindAuthorityGrant    = "authority_grant"
	NodeKindAgent             = "agent"
	NodeKindFailModePolicy    = "fail_mode_policy"
	NodeKindEscalationTarget  = "escalation_target"
)

// Authority Graph edge kinds — the complete allowed set.
//
// Edge directionality is fixed (src → dst):
//
//   business_service_has_surface          business_service → decision_surface
//   surface_uses_profile                  decision_surface → authority_profile
//   profile_has_grant                     authority_profile → authority_grant
//   grant_authorises_agent                authority_grant → agent
//   surface_has_fail_mode_policy          decision_surface → fail_mode_policy (label="override")
//   business_service_has_fail_mode_policy business_service → fail_mode_policy (label="default")
//   profile_escalates_to                  authority_profile → escalation_target (D31l)
const (
	EdgeKindBusinessServiceHasSurface         = "business_service_has_surface"
	EdgeKindSurfaceUsesProfile                = "surface_uses_profile"
	EdgeKindProfileHasGrant                   = "profile_has_grant"
	EdgeKindGrantAuthorisesAgent              = "grant_authorises_agent"
	EdgeKindSurfaceHasFailModePolicy          = "surface_has_fail_mode_policy"
	EdgeKindBusinessServiceHasFailModePolicy  = "business_service_has_fail_mode_policy"
	EdgeKindProfileEscalatesTo                = "profile_escalates_to"
)

// Edge labels disambiguate the two fail-mode-policy edges so
// renderers can show "override" vs "default" without inspecting the
// edge kind. Operators see at a glance whether a fail-mode reference
// is a surface override or a BS-level default.
const (
	EdgeLabelOverride = "override"
	EdgeLabelDefault  = "default"
)

// NodeRef is the (kind, id) pair that uniquely identifies a node in
// a projection. Used by Edge.Src and Edge.Dst, and by Projection.Root.
type NodeRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Node is one entity in the projected Authority Graph.
//
// Kind, ID, and Label are always populated. Exactly one typed-data
// pointer is non-nil per node, matched to Kind. The omitempty tags
// collapse unused pointers so a Node serialises to a compact shape
// carrying only its kind's data block. There is no generic Attrs
// map.
type Node struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`

	// Typed per-kind data — the field name matches the kind so the
	// wire payload is self-describing.
	BusinessService  *BusinessServiceData  `json:"business_service,omitempty"`
	DecisionSurface  *DecisionSurfaceData  `json:"decision_surface,omitempty"`
	AuthorityProfile *AuthorityProfileData `json:"authority_profile,omitempty"`
	AuthorityGrant   *AuthorityGrantData   `json:"authority_grant,omitempty"`
	Agent            *AgentData            `json:"agent,omitempty"`
	FailModePolicy   *FailModePolicyData   `json:"fail_mode_policy,omitempty"`
	EscalationTarget *EscalationTargetData `json:"escalation_target,omitempty"`
}

// Edge is one directed link in the projected graph. The semantic
// directionality is encoded in (Src → Dst) per the per-kind table
// above. Label is populated only for fail-mode-policy edges (carrying
// "override" or "default"); other edge kinds leave Label empty.
type Edge struct {
	Kind  string  `json:"kind"`
	Src   NodeRef `json:"src"`
	Dst   NodeRef `json:"dst"`
	Label string  `json:"label,omitempty"`
}

// Projection is the Authority Graph response payload.
//
// Nodes are sorted by (Kind, ID) ascending; edges are sorted by
// (Kind, Src.Kind, Src.ID, Dst.Kind, Dst.ID) ascending. Both slices
// are non-nil; an empty depth=0 projection still carries the root
// in Nodes and an empty Edges slice.
//
// D31g additions:
//
//   - Summary is a backend-computed posture rollup describing the
//     FULL service-view projection (before depth filtering).
//     Operators reading the summary at depth=0 still see the
//     authority posture, even though Nodes/Edges are pruned.
//   - Diagnostics is the deterministic list of operator-actionable
//     conditions the projection observed while building. Like
//     Summary, it describes the full pre-depth-filter graph.
//
// D31m additions:
//
//   - DiagnosticSummary is a per-severity rollup over the full pre-
//     depth Diagnostics array. Frontends can render a single
//     business-service-level severity badge without re-deriving the
//     rollup. HighestSeverity is one of "none" / "info" / "warning" /
//     "critical".
//   - SurfacePosture is a deterministic per-emitted-surface status
//     record covering authority/profile/grant/agent/fail-mode/
//     escalation axes plus per-surface diagnostic-kind set and
//     highest-severity rollup. Sorted by surface id ascending.
//
// All four fields (Summary, Diagnostics, DiagnosticSummary,
// SurfacePosture) describe the FULL pre-depth-filter projection.
// Depth filtering affects Nodes and Edges only.
//
// Every field uses omitempty so a client that ignores the new
// fields sees an unchanged wire shape relative to D31l.
type Projection struct {
	Root              NodeRef                   `json:"root"`
	View              string                    `json:"view"`
	Depth             int                       `json:"depth"`
	Nodes             []Node                    `json:"nodes"`
	Edges             []Edge                    `json:"edges"`
	Summary           *Summary                  `json:"summary,omitempty"`
	Diagnostics       []Diagnostic              `json:"diagnostics,omitempty"`
	DiagnosticSummary *DiagnosticSummary        `json:"diagnostic_summary,omitempty"`
	SurfacePosture    []SurfaceAuthorityPosture `json:"surface_posture,omitempty"`
}

// ---------------------------------------------------------------------------
// Summary + Diagnostics (D31g)
// ---------------------------------------------------------------------------

// Summary is the backend-computed posture rollup for a service-view
// Authority Graph projection. All count fields are always present
// (defaulting to 0); the *Without* and *Missing* list fields use
// omitempty so healthy projections produce a compact wire shape.
//
// Counts derive from the deduped pre-depth-filter node set:
//   - SurfaceCount       = distinct decision_surface nodes emitted
//   - ActiveProfileCount = distinct authority_profile nodes emitted
//   - ActiveGrantCount   = distinct authority_grant nodes emitted
//   - ActiveAgentCount   = distinct agent nodes emitted (deduped)
//   - FailModePolicyCount = distinct fail_mode_policy nodes emitted
//
// CompleteAuthorityPaths / IncompleteAuthorityPaths are per-surface
// (NOT per-grant). A surface counts as complete when it has at least
// one effective profile → effective grant → resolved agent chain.
//
// SurfacesWith*/SurfacesInheriting*/PoliciesMissing* fields are
// computed during projection and remain useful at depth=0 because
// they describe backend posture rather than visible nodes.
type Summary struct {
	SurfaceCount                           int       `json:"surface_count"`
	ActiveProfileCount                     int       `json:"active_profile_count"`
	ActiveGrantCount                       int       `json:"active_grant_count"`
	ActiveAgentCount                       int       `json:"active_agent_count"`
	FailModePolicyCount                    int       `json:"fail_mode_policy_count"`
	CompleteAuthorityPaths                 int       `json:"complete_authority_paths"`
	IncompleteAuthorityPaths               int       `json:"incomplete_authority_paths"`
	SurfacesWithoutProfiles                []NodeRef `json:"surfaces_without_profiles,omitempty"`
	ProfilesWithoutGrants                  []NodeRef `json:"profiles_without_grants,omitempty"`
	GrantsWithoutAgents                    []NodeRef `json:"grants_without_agents,omitempty"`
	SurfacesWithoutEffectiveFailModePolicy []NodeRef `json:"surfaces_without_effective_fail_mode_policy,omitempty"`
	SurfacesWithPolicyOverride             int       `json:"surfaces_with_policy_override"`
	SurfacesInheritingBSPolicy             int       `json:"surfaces_inheriting_bs_policy"`
	PoliciesMissingActiveVersion           []NodeRef `json:"policies_missing_active_version,omitempty"`

	// D31i posture rollups for the capability + constraint additions.
	//
	// GrantsWithStopCapability counts emitted authority_grant nodes
	// whose Capabilities slice contains "stop". Operators reading
	// summary at depth=0 see at a glance how broadly stop authority
	// is distributed across the service.
	//
	// GrantsWithConstraints counts emitted authority_grant nodes
	// that carry at least one Constraint. Empty-constraint grants
	// are the common case; this counter surfaces the subset of
	// grants whose runtime semantics are narrowed by typed
	// conditions.
	//
	// GrantsWithoutCapabilities lists emitted grants that carry zero
	// Capabilities — paired with the
	// grant_has_no_capabilities warning diagnostic. omitempty when no
	// such grants exist.
	GrantsWithStopCapability  int       `json:"grants_with_stop_capability"`
	GrantsWithConstraints     int       `json:"grants_with_constraints"`
	GrantsWithoutCapabilities []NodeRef `json:"grants_without_capabilities,omitempty"`

	// D31l: escalation-target posture rollups.
	//
	// EscalationTargetCount counts deduped escalation_target nodes
	// emitted in the full pre-depth-filter projection.
	//
	// ProfilesWithEscalationTarget counts effective profiles whose
	// EscalationTargetID resolved to an active target.
	//
	// ProfilesWithoutEscalationTarget lists effective profiles with
	// an empty EscalationTargetID — these are profiles where
	// escalation goes nowhere explicit (paired with the info
	// diagnostic profile_has_no_escalation_target). Use omitempty so
	// healthy projections produce a compact wire shape.
	//
	// ProfilesWithDanglingEscalationTarget lists effective profiles
	// whose EscalationTargetID did not resolve to an active target
	// (paired with the warning diagnostic
	// escalation_target_reference_dangling).
	EscalationTargetCount                int       `json:"escalation_target_count"`
	ProfilesWithEscalationTarget         int       `json:"profiles_with_escalation_target"`
	ProfilesWithoutEscalationTarget      []NodeRef `json:"profiles_without_escalation_target,omitempty"`
	ProfilesWithDanglingEscalationTarget []NodeRef `json:"profiles_with_dangling_escalation_target,omitempty"`
}

// ---------------------------------------------------------------------------
// DiagnosticSummary + SurfaceAuthorityPosture (D31m)
// ---------------------------------------------------------------------------

// DiagnosticSummary is a per-severity rollup over the full pre-depth
// Diagnostics array. Frontends consume HighestSeverity for a single
// business-service-level badge and ByKind for a per-kind count
// drill-down.
//
// Invariants:
//
//   - Info + Warning + Critical == len(Diagnostics)
//   - HighestSeverity is exactly one of:
//       - "critical" when Critical > 0
//       - "warning"  when Critical == 0 && Warning > 0
//       - "info"     when Critical == 0 && Warning == 0 && Info > 0
//       - "none"     when Critical == 0 && Warning == 0 && Info == 0
//   - ByKind uses omitempty so an all-zero / empty-diagnostics summary
//     produces a compact wire shape.
type DiagnosticSummary struct {
	Info            int            `json:"info"`
	Warning         int            `json:"warning"`
	Critical        int            `json:"critical"`
	HighestSeverity string         `json:"highest_severity"`
	ByKind          map[string]int `json:"by_kind,omitempty"`
}

// SurfaceAuthorityPosture is a backend-computed per-surface status
// record covering every authority axis the frontend would otherwise
// have to derive by walking edges and joining diagnostics.
//
// Status enums are open-text discriminators (not Go enum types) so
// the wire format stays self-describing without a separate enum
// indirection — the canonical values are pinned by the
// AuthorityStatus* / ProfileStatus* / GrantStatus* / AgentStatus* /
// FailModePolicyStatus* / EscalationStatus* / HighestSeverity*
// constants below.
//
// DiagnosticKinds lists the unique diagnostic kinds whose NodeRefs
// associate to this surface (via direct surface ref, or via a
// profile/grant/agent ref known to attach to this surface).
//
// Posture describes the FULL pre-depth-filter projection — depth=0
// callers still receive the per-surface posture for every emitted
// active surface.
type SurfaceAuthorityPosture struct {
	Surface NodeRef `json:"surface"`

	AuthorityStatus      string `json:"authority_status"`
	ProfileStatus        string `json:"profile_status"`
	GrantStatus          string `json:"grant_status"`
	AgentStatus          string `json:"agent_status"`
	FailModePolicyStatus string `json:"fail_mode_policy_status"`
	EscalationStatus     string `json:"escalation_status"`

	CompletePaths   int `json:"complete_paths"`
	IncompletePaths int `json:"incomplete_paths"`

	HighestSeverity string   `json:"highest_severity"`
	DiagnosticKinds []string `json:"diagnostic_kinds,omitempty"`
}

// HighestSeverity* — the four severity-rollup values used by
// DiagnosticSummary.HighestSeverity and SurfaceAuthorityPosture.
// HighestSeverityNone is the only value defined here; the other
// three share string identity with DiagnosticSeverityInfo,
// DiagnosticSeverityWarning, and DiagnosticSeverityCritical so the
// rollup uses the canonical severity literals.
const (
	HighestSeverityNone     = "none"
	HighestSeverityInfo     = DiagnosticSeverityInfo
	HighestSeverityWarning  = DiagnosticSeverityWarning
	HighestSeverityCritical = DiagnosticSeverityCritical
)

// AuthorityStatus* — surface-level authority posture roll-up.
// Precedence (worst → best):
//
//	uncovered  > incomplete > degraded > complete
//
// Computed from per-surface accumulators:
//
//   - uncovered: surface has no effective profile.
//   - incomplete: surface has at least one effective profile but
//     zero complete profile→grant→agent paths.
//   - degraded: surface has at least one complete path AND at least
//     one warning- or critical-severity diagnostic applies to it.
//   - complete: surface has at least one complete path AND no
//     warning/critical diagnostic applies (info-only is OK).
const (
	AuthorityStatusComplete   = "complete"
	AuthorityStatusIncomplete = "incomplete"
	AuthorityStatusDegraded   = "degraded"
	AuthorityStatusUncovered  = "uncovered"
)

// ProfileStatus* — surface-level profile coverage.
const (
	ProfileStatusCovered = "covered"
	ProfileStatusMissing = "missing"
)

// GrantStatus* — surface-level grant coverage (at least one
// effective grant under any profile attached to the surface).
const (
	GrantStatusCovered = "covered"
	GrantStatusMissing = "missing"
)

// AgentStatus* — surface-level agent coverage.
//
// Precedence:
//
//	blocked > missing > covered
//
//   - covered: at least one complete profile→grant→agent path
//     exists with an active agent; no critical missing/inactive-agent
//     diagnostic applies.
//   - missing: profiles/grants exist under the surface but no grant
//     resolves to an agent record.
//   - blocked: at least one grant under the surface references a
//     missing or inactive agent (a critical agent diagnostic
//     applies). Blocked supersedes covered even when another grant
//     is healthy — the posture surfaces the worst-applicable case.
const (
	AgentStatusCovered = "covered"
	AgentStatusMissing = "missing"
	AgentStatusBlocked = "blocked"
)

// FailModePolicyStatus* — surface-level fail-mode posture.
//
//   - override: surface's own FailModePolicyID resolved.
//   - inherited: surface has no override (or has an override that
//     resolved); BS default supplied the effective policy.
//   - missing: no effective policy resolves at any level.
//   - dangling: a non-empty policy reference (surface override or
//     BS default) failed to resolve to an active version. Dangling
//     wins over inherited and missing — operators should see that
//     configuration is broken even when fallback exists.
const (
	FailModePolicyStatusOverride  = "override"
	FailModePolicyStatusInherited = "inherited"
	FailModePolicyStatusMissing   = "missing"
	FailModePolicyStatusDangling  = "dangling"
)

// EscalationStatus* — surface-level escalation routing posture.
//
//   - targeted: at least one effective profile under the surface
//     references an EscalationTarget that resolved to an active
//     version.
//   - not_targeted: no effective profile references an escalation
//     target (every profile under the surface has empty
//     EscalationTargetID).
//   - dangling: at least one effective profile references a target
//     id that does NOT resolve to an active version. Dangling wins
//     over targeted — configuration drift should surface.
const (
	EscalationStatusTargeted    = "targeted"
	EscalationStatusNotTargeted = "not_targeted"
	EscalationStatusDangling    = "dangling"
)

// Diagnostic is one operator-actionable condition observed during
// projection. The kind comes from the DiagnosticKind* constants; the
// severity comes from DiagnosticSeverity*. NodeRefs identifies the
// entities the diagnostic applies to (typically the most-specific
// entity first, with supporting context refs trailing).
//
// Diagnostics are sorted deterministically by:
//
//	1. severity rank (critical < warning < info)
//	2. then kind
//	3. then first NodeRef.Kind
//	4. then first NodeRef.ID
type Diagnostic struct {
	Kind     string    `json:"kind"`
	Severity string    `json:"severity"`
	NodeRefs []NodeRef `json:"node_refs,omitempty"`
	Message  string    `json:"message,omitempty"`
}

// DiagnosticSeverity* — the three operator-facing severity levels.
// Pin in OpenAPI; tests pin both the constants and the rank ordering
// used for diagnostic sorting.
const (
	DiagnosticSeverityCritical = "critical"
	DiagnosticSeverityWarning  = "warning"
	DiagnosticSeverityInfo     = "info"
)

// DiagnosticKind* — the operator-facing diagnostic vocabulary.
// Pin in OpenAPI's AuthorityGraphDiagnosticKind enum.
const (
	DiagnosticKindBusinessServiceHasNoActiveSurface        = "business_service_has_no_active_surface"
	DiagnosticKindSurfaceHasNoActiveProfile                = "surface_has_no_active_profile"
	DiagnosticKindProfileHasNoActiveGrant                  = "profile_has_no_active_grant"
	DiagnosticKindGrantReferencesMissingAgent              = "grant_references_missing_agent"
	DiagnosticKindGrantReferencesInactiveAgent             = "grant_references_inactive_agent"
	DiagnosticKindFailModePolicyReferenceDangling          = "fail_mode_policy_reference_dangling"
	DiagnosticKindSurfaceInheritsBusinessServicePolicy     = "surface_inherits_business_service_policy"
	DiagnosticKindSurfaceOverridesBusinessServiceDefault   = "surface_overrides_business_service_default"
	DiagnosticKindSurfaceOverrideMatchesBSDefault          = "surface_override_matches_business_service_default"
	DiagnosticKindProfileFutureDated                       = "profile_future_dated"
	DiagnosticKindProfileExpired                           = "profile_expired"
	DiagnosticKindGrantFutureDated                         = "grant_future_dated"
	DiagnosticKindGrantExpired                             = "grant_expired"
	DiagnosticKindDuplicateActiveProfileVersionsForSurface = "duplicate_active_profile_versions_for_surface"

	// D31i: warning emitted when an emitted authority_grant carries
	// zero Capabilities. The grant is structurally valid (it links
	// an agent to a profile) but it authorises no concrete action —
	// the orchestrator's capability check would reject any
	// capability-typed request against it.
	DiagnosticKindGrantHasNoCapabilities = "grant_has_no_capabilities"

	// D31l escalation-target diagnostics:
	//
	//   profile_has_no_escalation_target (info) — an effective
	//   AuthorityProfile carries an empty EscalationTargetID. Not a
	//   configuration error per se: a profile may rely on manual
	//   routing or be intentionally non-routed. Info severity
	//   surfaces the gap without alarming operators.
	//
	//   escalation_target_reference_dangling (warning) — the profile's
	//   EscalationTargetID is non-empty but no active target version
	//   resolves at the current instant. This is a configuration
	//   problem; the runtime preserves the escalation outcome and
	//   emits ESCALATION_TARGET_RESOLUTION_FAILED (see D31k), so a
	//   warning is the appropriate severity.
	DiagnosticKindProfileHasNoEscalationTarget       = "profile_has_no_escalation_target"
	DiagnosticKindEscalationTargetReferenceDangling  = "escalation_target_reference_dangling"
)

// ValidityStatus* — the time-window classification an authority
// profile or grant carries on its typed data. Emitted nodes always
// carry "effective"; future_dated and expired entities are filtered
// out of the node set (and produce diagnostics instead).
const (
	ValidityStatusEffective   = "effective"
	ValidityStatusFutureDated = "future_dated"
	ValidityStatusExpired     = "expired"
)

// EffectivePolicySource* — the resolved fail-mode-policy origin per
// decision surface. Emitted on DecisionSurfaceData so the UI can show
// whether the policy was an explicit surface override, a BS-level
// fallback, or no effective policy at all.
const (
	EffectivePolicySourceOverride               = "override"
	EffectivePolicySourceBusinessServiceDefault = "business_service_default"
	EffectivePolicySourceNone                   = "none"
)

// ---------------------------------------------------------------------------
// Per-kind typed data
// ---------------------------------------------------------------------------

// ExternalRefData mirrors the externalref wire shape. Defined here
// to keep the authoritygraph package from depending on internal/httpapi
// helpers and to keep externalref-to-wire concerns isolated.
type ExternalRefData struct {
	SourceSystem  string  `json:"source_system"`
	SourceID      string  `json:"source_id"`
	SourceURL     string  `json:"source_url,omitempty"`
	SourceVersion string  `json:"source_version,omitempty"`
	LastSyncedAt  *string `json:"last_synced_at,omitempty"`
}

// BusinessServiceData is the typed data for a business_service node.
// FailModePolicyID is carried verbatim from the domain even when no
// policy is currently active; the projection also emits the resolved
// fail_mode_policy node + edge separately when the reference
// resolves to an active version (see Service.Project).
type BusinessServiceData struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Status           string           `json:"status"`
	Owner            string           `json:"owner,omitempty"`
	ServiceType      string           `json:"service_type,omitempty"`
	ExternalRef      *ExternalRefData `json:"external_ref,omitempty"`
	FailModePolicyID string           `json:"fail_mode_policy_id,omitempty"`
}

// DecisionSurfaceData is the typed data for a decision_surface node.
// BusinessServiceID is derived during projection from the parent
// process; the surface domain type does not own a BS pointer
// directly. ProcessID is carried for traceability even though the
// Authority Graph collapses process traversal into the BS→Surface
// edge.
//
// D31g adds resolved fail-mode-policy metadata: the surface knows
// its effective policy without the UI having to traverse to a policy
// node. Operators see at a glance whether a surface has an explicit
// override, inherits the BS-level default, or has no effective
// policy. The raw FailModePolicyID is the surface's stored override
// reference (verbatim from the domain); EffectivePolicyID is the
// resolved policy id after BS-default fallback.
type DecisionSurfaceData struct {
	ID                     string `json:"id"`
	Version                int    `json:"version"`
	Name                   string `json:"name"`
	Status                 string `json:"status"`
	ProcessID              string `json:"process_id,omitempty"`
	BusinessServiceID      string `json:"business_service_id,omitempty"`
	FailModePolicyID       string `json:"fail_mode_policy_id,omitempty"`
	EffectivePolicySource  string `json:"effective_policy_source,omitempty"`
	EffectivePolicyID      string `json:"effective_policy_id,omitempty"`
	EffectivePolicyVersion int    `json:"effective_policy_version,omitempty"`
	InheritsBSPolicy       bool   `json:"inherits_bs_policy,omitempty"`
}

// ConsequenceThresholdData is the wire shape for an authority
// profile's consequence threshold. Exactly one variant is populated
// based on Type — monetary (Amount + Currency) or risk_rating
// (RiskRating). Mirrors authority.Consequence.
type ConsequenceThresholdData struct {
	Type       string  `json:"type"`
	Amount     float64 `json:"amount,omitempty"`
	Currency   string  `json:"currency,omitempty"`
	RiskRating string  `json:"risk_rating,omitempty"`
}

// AuthorityProfileData is the typed data for an authority_profile
// node. Includes lifecycle fields (status, effective_date,
// effective_until, approved_by/at) plus the runtime-relevant
// configuration (thresholds, escalation_mode, fail_mode,
// policy_reference). Noisy fields like RequiredContextKeys and full
// audit trails are deliberately omitted at MVP.
//
// D31g adds:
//
//   - EffectiveUntil — domain field for the validity-window upper
//     bound; emitted when set on the profile.
//   - ValidityStatus — always "effective" on emitted nodes (future-
//     dated and expired profiles are filtered out and produce
//     diagnostics rather than nodes). Explicit value pins the
//     wire contract so the UI never has to guess.
type AuthorityProfileData struct {
	ID                   string                    `json:"id"`
	Version              int                       `json:"version"`
	SurfaceID            string                    `json:"surface_id"`
	Name                 string                    `json:"name"`
	Status               string                    `json:"status"`
	EffectiveDate        *string                   `json:"effective_date,omitempty"`
	EffectiveUntil       *string                   `json:"effective_until,omitempty"`
	ValidityStatus       string                    `json:"validity_status,omitempty"`
	ConfidenceThreshold  float64                   `json:"confidence_threshold"`
	ConsequenceThreshold *ConsequenceThresholdData `json:"consequence_threshold,omitempty"`
	EscalationMode       string                    `json:"escalation_mode,omitempty"`
	FailMode             string                    `json:"fail_mode,omitempty"`
	PolicyReference      string                    `json:"policy_reference,omitempty"`
	ApprovedBy           string                    `json:"approved_by,omitempty"`
	ApprovedAt           *string                   `json:"approved_at,omitempty"`

	// D31l: the configured EscalationTargetID, carried verbatim from
	// the domain. Empty when the profile has no explicit target. The
	// resolved active target appears as a separate escalation_target
	// node + profile_escalates_to edge — operators read the edge to
	// see the resolved target (whose version + kind + handle live on
	// the EscalationTargetData typed-data payload).
	EscalationTargetID string `json:"escalation_target_id,omitempty"`
}

// AuthorityGrantData is the typed data for an authority_grant node.
// Grant references the profile by logical id (not by version) — the
// profile node carries its own version separately. Noisy lifecycle
// fields (suspend/revoke metadata, GrantReason) are intentionally
// excluded from MVP.
//
// D31g adds ValidityStatus — always "effective" on emitted nodes,
// matching the convention on AuthorityProfileData.
//
// D31i adds:
//
//   - Capabilities — the SET of canonical Capability values the grant
//     authorises (recommend, approve, reject, escalate, stop). Empty
//     when the grant carries no capabilities; the Authority Graph
//     emits a warning diagnostic for such grants.
//   - Constraints  — the typed list of runtime constraints the grant
//     applies (one entry per kind). Empty / omitted when the grant
//     has no constraints.
//
// Both fields are read-model projections of the domain values; the
// orchestrator is the source of truth for runtime enforcement, the
// Authority Graph mirrors them so operators see at a glance what each
// grant authorises and under what restrictions.
type AuthorityGrantData struct {
	ID             string             `json:"id"`
	ProfileID      string             `json:"profile_id"`
	AgentID        string             `json:"agent_id"`
	Status         string             `json:"status"`
	EffectiveDate  *string            `json:"effective_date,omitempty"`
	ExpiresAt      *string            `json:"expires_at,omitempty"`
	ValidityStatus string             `json:"validity_status,omitempty"`
	GrantedBy      string             `json:"granted_by,omitempty"`
	Capabilities   []string           `json:"capabilities,omitempty"`
	Constraints    []ConstraintData   `json:"constraints,omitempty"`
}

// ConstraintData is the wire shape for one grant constraint on the
// Authority Graph. Mirrors authority.Constraint: Kind plus the
// per-kind payload, with each variant field marked omitempty so a
// human_only / ai_only entry serialises to just {"kind": "..."}.
type ConstraintData struct {
	Kind           string                    `json:"kind"`
	MinConfidence  *float64                  `json:"min_confidence,omitempty"`
	MaxConsequence *ConsequenceThresholdData `json:"max_consequence,omitempty"`
	StartTime      *string                   `json:"start_time,omitempty"`
	EndTime        *string                   `json:"end_time,omitempty"`
}

// AgentData is the typed data for an agent node. Agent.Endpoint is
// intentionally not exposed via the graph — it's an internal URL
// not appropriate for a read-only projection.
type AgentData struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Owner            string `json:"owner,omitempty"`
	ModelVersion     string `json:"model_version,omitempty"`
	OperationalState string `json:"operational_state"`
}

// RuleCountByClassData is the per-correctness-class rule count
// summary for a FailModePolicy. Exposes which classes are covered
// without forcing the full rules array into the graph payload (see
// GET /v1/fail_mode_policies/{id} for the full rule list).
//
// All five CorrectnessClass values are present, defaulting to zero.
type RuleCountByClassData struct {
	GovernanceIntegrity int `json:"governance_integrity"`
	Persistence         int `json:"persistence"`
	Input               int `json:"input"`
	Resource            int `json:"resource"`
	Consistency         int `json:"consistency"`
}

// EscalationTargetData is the typed data for an escalation_target
// node (D31l). Mirrors the escalation.EscalationTarget domain fields
// the graph projects as a read model. Cross-context existence (e.g.
// that an agent target's Handle is a real agent id) is NOT enforced
// here — the projection emits the resolved target verbatim and
// leaves cross-context validation to higher layers.
//
// The Authority Graph never becomes the source of truth for an
// EscalationTarget; the read endpoints under /v1/escalation_targets
// own that role. EscalationTargetData is graph-specific so the
// Authority Graph package stays free of direct escalation-package
// wire shape coupling.
type EscalationTargetData struct {
	ID          string `json:"id"`
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	Kind   string `json:"kind"`
	Handle string `json:"handle"`

	Status         string  `json:"status"`
	EffectiveDate  *string `json:"effective_date,omitempty"`
	EffectiveUntil *string `json:"effective_until,omitempty"`

	BusinessOwner  string `json:"business_owner,omitempty"`
	TechnicalOwner string `json:"technical_owner,omitempty"`

	ApprovedBy string  `json:"approved_by,omitempty"`
	ApprovedAt *string `json:"approved_at,omitempty"`
}

// FailModePolicyData is the typed data for a fail_mode_policy node.
// Carries identity, lifecycle, and ownership; the full Rules array
// is omitted (operator-noisy) but the per-class rule count is
// exposed so operators see coverage at a glance.
//
// D31g adds EffectiveUntil — domain field for the validity-window
// upper bound. Emitted when set; omitted otherwise.
type FailModePolicyData struct {
	ID               string                `json:"id"`
	Version          int                   `json:"version"`
	Name             string                `json:"name"`
	Status           string                `json:"status"`
	EffectiveDate    *string               `json:"effective_date,omitempty"`
	EffectiveUntil   *string               `json:"effective_until,omitempty"`
	BusinessOwner    string                `json:"business_owner,omitempty"`
	TechnicalOwner   string                `json:"technical_owner,omitempty"`
	Origin           string                `json:"origin,omitempty"`
	Managed          bool                  `json:"managed"`
	RuleCountByClass *RuleCountByClassData `json:"rule_count_by_class,omitempty"`
}

// MarshalIndent / MarshalJSON helpers are not needed — the default
// encoding/json behaviour suffices for the wire shape pinned here.
// The reference is kept compile-time live so future test helpers can
// rely on the import.
var _ = json.Marshal

// timePtrString renders a UTC RFC3339 string for non-zero times,
// returning nil for zero values. Used by projection builders to map
// time.Time / *time.Time fields onto wire-format optional strings.
func timePtrString(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func timePtrFromPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return timePtrString(*t)
}

// consequenceThresholdString renders a value.ConsequenceType safely
// onto the wire as a lowercase string. Kept here to centralise the
// mapping; if ConsequenceType ever gains new variants, this is the
// single place to extend.
func consequenceTypeString(t value.ConsequenceType) string {
	return string(t)
}
