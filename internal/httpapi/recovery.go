package httpapi

import (
	"context"
	"time"

	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Recovery result types — surfaces
// ---------------------------------------------------------------------------

// SurfaceVersionSummary is a compact version entry used in the profile recovery response.
type SurfaceVersionSummary struct {
	Version       int       `json:"version"`
	Status        string    `json:"status"`
	EffectiveFrom time.Time `json:"effective_from"`
}

// SurfaceRecoveryResult is the assembled recovery analysis for a decision surface.
// It is read-only — computed from persisted state, no writes occur.
//
// Fields use *int / *string for version/status pairs so the caller can
// distinguish "no active version" (nil) from "version 0" or "".
type SurfaceRecoveryResult struct {
	SurfaceID          string  `json:"surface_id"`
	LatestVersion      int     `json:"latest_version"`
	LatestStatus       string  `json:"latest_status"`
	ActiveVersion      *int    `json:"active_version"`  // null when no active version exists
	ActiveStatus       *string `json:"active_status"`   // null when no active version exists
	SuccessorSurfaceID string  `json:"successor_surface_id"`
	DeprecationReason  string  `json:"deprecation_reason"`
	VersionCount       int     `json:"version_count"`
	Warnings           []string `json:"warnings"`
	RecommendedNextActions []string `json:"recommended_next_actions"`
}

// ProfileVersionEntry is one row in the profile recovery versions list.
type ProfileVersionEntry struct {
	Version       int       `json:"version"`
	Status        string    `json:"status"`
	EffectiveFrom time.Time `json:"effective_from"`
}

// ProfileRecoveryResult is the assembled recovery analysis for an authority profile.
// It is read-only — computed from persisted state, no writes occur.
type ProfileRecoveryResult struct {
	ProfileID     string `json:"profile_id"`
	SurfaceID     string `json:"surface_id"`
	LatestVersion int    `json:"latest_version"`
	LatestStatus  string `json:"latest_status"`
	// ActiveVersion is nil when no version is currently effective (e.g. future effective_date).
	ActiveVersion *int    `json:"active_version"`
	ActiveStatus  *string `json:"active_status"`
	VersionCount  int     `json:"version_count"`
	Versions      []ProfileVersionEntry `json:"versions"`
	// ActiveGrantCount is the number of active grants linked to this profile.
	// -1 means the grant repository is not available.
	ActiveGrantCount int    `json:"active_grant_count"`
	// CapabilityNote is an honest description of the current profile lifecycle behaviour.
	CapabilityNote string   `json:"capability_note"`
	Warnings       []string `json:"warnings"`
	RecommendedNextActions []string `json:"recommended_next_actions"`
}

// ---------------------------------------------------------------------------
// Recovery service methods on IntrospectionService
// ---------------------------------------------------------------------------

// GetSurfaceRecovery computes a read-only recovery analysis for the given surface ID.
// Returns nil, nil when the surface does not exist.
func (s *IntrospectionService) GetSurfaceRecovery(ctx context.Context, id string) (*SurfaceRecoveryResult, error) {
	versions, err := s.surfaces.ListVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}

	// ListVersions returns descending order; first element is latest.
	latest := versions[0]

	// Find current active version (status == active, effective window covers now).
	now := time.Now().UTC()
	var activeVersion *surface.DecisionSurface
	for _, v := range versions {
		if v.Status != surface.SurfaceStatusActive {
			continue
		}
		if v.EffectiveFrom.After(now) {
			continue
		}
		if v.EffectiveUntil != nil && !v.EffectiveUntil.After(now) {
			continue
		}
		if activeVersion == nil || v.Version > activeVersion.Version {
			activeVersion = v
		}
	}

	result := &SurfaceRecoveryResult{
		SurfaceID:          id,
		LatestVersion:      latest.Version,
		LatestStatus:       string(latest.Status),
		SuccessorSurfaceID: latest.SuccessorSurfaceID,
		DeprecationReason:  latest.DeprecationReason,
		VersionCount:       len(versions),
	}

	if activeVersion != nil {
		v := activeVersion.Version
		st := string(activeVersion.Status)
		result.ActiveVersion = &v
		result.ActiveStatus = &st
	}

	result.Warnings = buildSurfaceRecoveryWarnings(latest, activeVersion)
	result.RecommendedNextActions = buildSurfaceRecoveryActions(latest, activeVersion, versions)

	return result, nil
}

// buildSurfaceRecoveryWarnings emits deterministic warnings from actual surface state.
func buildSurfaceRecoveryWarnings(latest *surface.DecisionSurface, active *surface.DecisionSurface) []string {
	var w []string
	if latest.Status == surface.SurfaceStatusDeprecated {
		w = append(w, "surface is deprecated and should not receive new grants")
	}
	if latest.Status == surface.SurfaceStatusRetired {
		w = append(w, "surface is retired and is no longer operational")
	}
	if active == nil && latest.Status != surface.SurfaceStatusDeprecated && latest.Status != surface.SurfaceStatusRetired {
		w = append(w, "no active version — evaluation requests will be rejected")
	}
	if w == nil {
		w = []string{}
	}
	return w
}

// buildSurfaceRecoveryActions emits deterministic recommended actions derived from actual state.
func buildSurfaceRecoveryActions(latest *surface.DecisionSurface, active *surface.DecisionSurface, allVersions []*surface.DecisionSurface) []string {
	var actions []string

	latestStatus := latest.Status
	hasActive := active != nil

	// Check if any version is deprecated (not just the latest).
	hasDeprecatedVersion := false
	var deprecatedVersion *surface.DecisionSurface
	for _, v := range allVersions {
		if v.Status == surface.SurfaceStatusDeprecated {
			hasDeprecatedVersion = true
			if deprecatedVersion == nil || v.Version > deprecatedVersion.Version {
				deprecatedVersion = v
			}
		}
	}

	// Check if there are any review versions.
	hasReviewVersion := false
	for _, v := range allVersions {
		if v.Status == surface.SurfaceStatusReview {
			hasReviewVersion = true
			break
		}
	}

	switch {
	case latestStatus == surface.SurfaceStatusDeprecated && latest.SuccessorSurfaceID != "":
		actions = append(actions, "inspect successor surface '"+latest.SuccessorSurfaceID+"' and plan grant migration")

	case latestStatus == surface.SurfaceStatusDeprecated && latest.SuccessorSurfaceID == "" && !hasReviewVersion:
		actions = append(actions, "apply replacement surface")

	case latestStatus == surface.SurfaceStatusDeprecated && latest.SuccessorSurfaceID == "" && hasReviewVersion:
		actions = append(actions, "approve replacement surface")

	case latestStatus == surface.SurfaceStatusReview && hasDeprecatedVersion && !hasActive:
		// A review version exists as a replacement for a deprecated surface.
		actions = append(actions, "approve replacement surface")

	case latestStatus == surface.SurfaceStatusReview && !hasActive && !hasDeprecatedVersion:
		actions = append(actions, "approve latest review version to enable evaluation")

	case latestStatus == surface.SurfaceStatusReview && hasActive:
		actions = append(actions, "approve latest review version to activate updated configuration")

	case latestStatus == surface.SurfaceStatusActive && hasReviewVersion:
		actions = append(actions, "deprecate this surface with successor_id pointing to the replacement")
	}

	_ = hasDeprecatedVersion // already used above

	if actions == nil {
		actions = []string{}
	}
	return actions
}

// GetProfileRecovery computes a read-only recovery analysis for the given profile ID.
// Returns nil, nil when the profile does not exist.
func (s *IntrospectionService) GetProfileRecovery(ctx context.Context, id string) (*ProfileRecoveryResult, error) {
	if s.profiles == nil {
		return nil, nil
	}

	versions, err := s.profiles.ListVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}

	// ListVersions returns descending order; first element is latest.
	latest := versions[0]

	// Find currently active version (status == active, effective_date <= now).
	now := time.Now().UTC()
	var activeVersion *authority.AuthorityProfile
	for _, v := range versions {
		if v.Status != authority.ProfileStatusActive {
			continue
		}
		if v.EffectiveDate.After(now) {
			continue
		}
		if v.EffectiveUntil != nil && !v.EffectiveUntil.After(now) {
			continue
		}
		if activeVersion == nil || v.Version > activeVersion.Version {
			activeVersion = v
		}
	}

	// Count active grants if grants reader is wired.
	activeGrantCount := -1
	if s.grants != nil {
		grants, err := s.grants.ListByProfile(ctx, id)
		if err != nil {
			return nil, err
		}
		activeGrantCount = 0
		for _, g := range grants {
			if g.Status == authority.GrantStatusActive {
				activeGrantCount++
			}
		}
	}

	// Build version summary list (descending, all versions).
	versionEntries := make([]ProfileVersionEntry, 0, len(versions))
	for _, v := range versions {
		versionEntries = append(versionEntries, ProfileVersionEntry{
			Version:       v.Version,
			Status:        string(v.Status),
			EffectiveFrom: v.EffectiveDate,
		})
	}

	result := &ProfileRecoveryResult{
		ProfileID:        id,
		SurfaceID:        latest.SurfaceID,
		LatestVersion:    latest.Version,
		LatestStatus:     string(latest.Status),
		VersionCount:     len(versions),
		Versions:         versionEntries,
		ActiveGrantCount: activeGrantCount,
		// Profiles follow a governed lifecycle: apply creates status=review,
		// explicit approval via POST /v1/controlplane/profiles/{id}/approve
		// transitions to active. Deprecation is explicit via POST
		// /v1/controlplane/profiles/{id}/deprecate.
		CapabilityNote: "profiles follow a governed review→active→deprecated lifecycle; " +
			"apply creates a review version, explicit approval is required before runtime use",
	}

	if activeVersion != nil {
		v := activeVersion.Version
		st := string(activeVersion.Status)
		result.ActiveVersion = &v
		result.ActiveStatus = &st
	}

	result.Warnings = buildProfileRecoveryWarnings(latest, activeVersion, versions)
	result.RecommendedNextActions = buildProfileRecoveryActions(activeVersion, activeGrantCount, versions)

	return result, nil
}

// buildProfileRecoveryWarnings emits deterministic warnings from actual profile state.
func buildProfileRecoveryWarnings(latest *authority.AuthorityProfile, active *authority.AuthorityProfile, allVersions []*authority.AuthorityProfile) []string {
	var w []string

	// Check if no active version but there are future-dated versions.
	if active == nil {
		hasFutureDated := false
		now := time.Now().UTC()
		for _, v := range allVersions {
			if v.Status == authority.ProfileStatusActive && v.EffectiveDate.After(now) {
				hasFutureDated = true
				break
			}
		}
		if hasFutureDated {
			w = append(w, "no version is currently effective; a version exists with a future effective_date")
		} else {
			w = append(w, "no active version — evaluation requests referencing this profile will fail")
		}
	}

	if latest.Status == authority.ProfileStatusDeprecated {
		w = append(w, "profile is deprecated")
	}

	if w == nil {
		w = []string{}
	}
	return w
}

// buildProfileRecoveryActions emits deterministic recommended actions derived from actual state.
func buildProfileRecoveryActions(active *authority.AuthorityProfile, activeGrantCount int, allVersions []*authority.AuthorityProfile) []string {
	var actions []string

	now := time.Now().UTC()

	// Count review versions pending approval.
	var reviewCount int
	for _, v := range allVersions {
		if v.Status == authority.ProfileStatusReview {
			reviewCount++
		}
	}

	if active == nil {
		if reviewCount > 0 {
			// No active version but there is a review version — needs approval.
			actions = append(actions, "approve latest review version to activate updated authority")
		}
		// Check if there are future-dated active versions causing the gap.
		for _, v := range allVersions {
			if v.Status == authority.ProfileStatusActive && v.EffectiveDate.After(now) {
				actions = append(actions, "re-apply profile with an effective_date in the past to restore evaluation eligibility")
				break
			}
		}
	} else {
		// Active version exists — surface approval or deprecation guidance.
		if reviewCount > 0 {
			actions = append(actions, "approve latest review version to activate updated authority")
		}
		if activeGrantCount > 0 {
			actions = append(actions, "inspect dependent grants before deprecating this profile version")
		}
	}

	if actions == nil {
		actions = []string{}
	}
	return actions
}
