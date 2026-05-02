package apply

import (
	"fmt"
	"strings"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// mapAISystemDocumentToAISystem maps a validated AISystemDocument into the
// domain type. Apply is status-honouring: spec.status flows through verbatim.
// origin defaults to "manual" when unset (mirrors the BusinessService /
// Capability default).
func mapAISystemDocumentToAISystem(doc types.AISystemDocument, now time.Time, createdBy string) *aisystem.AISystem {
	now = now.UTC()
	status := strings.TrimSpace(doc.Spec.Status)
	if status == "" {
		status = aisystem.AISystemStatusActive
	}
	origin := strings.TrimSpace(doc.Spec.Origin)
	if origin == "" {
		origin = aisystem.AISystemOriginManual
	}
	return &aisystem.AISystem{
		ID:          strings.TrimSpace(doc.Metadata.ID),
		Name:        strings.TrimSpace(doc.Metadata.Name),
		Description: strings.TrimSpace(doc.Spec.Description),
		Owner:       strings.TrimSpace(doc.Spec.Owner),
		Vendor:      strings.TrimSpace(doc.Spec.Vendor),
		SystemType:  strings.TrimSpace(doc.Spec.SystemType),
		Status:      status,
		Origin:      origin,
		Managed:     true,
		Replaces:    strings.TrimSpace(doc.Spec.Replaces),
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   strings.TrimSpace(createdBy),
	}
}

// mapAISystemVersionDocumentToAISystemVersion maps a validated
// AISystemVersionDocument into the domain type. Status is honoured (no
// review-forcing). RFC3339 timestamps are parsed at this layer; the
// validator has already confirmed they parse cleanly. ComplianceFrameworks
// is normalised to a non-nil slice.
//
// Returns an error only on programmer-error timestamp re-parse failures —
// a validated doc should not produce one.
func mapAISystemVersionDocumentToAISystemVersion(doc types.AISystemVersionDocument, now time.Time, createdBy string) (*aisystem.AISystemVersion, error) {
	now = now.UTC()
	status := strings.TrimSpace(doc.Spec.Status)
	if status == "" {
		status = aisystem.AISystemVersionStatusActive
	}

	effectiveFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(doc.Spec.EffectiveFrom))
	if err != nil {
		return nil, fmt.Errorf("ai system version %s v%d: parse effective_from: %w", doc.Spec.AISystemID, doc.Spec.Version, err)
	}
	var effectiveUntil *time.Time
	if eu := strings.TrimSpace(doc.Spec.EffectiveUntil); eu != "" {
		t, err := time.Parse(time.RFC3339, eu)
		if err != nil {
			return nil, fmt.Errorf("ai system version %s v%d: parse effective_until: %w", doc.Spec.AISystemID, doc.Spec.Version, err)
		}
		effectiveUntil = &t
	}
	var retiredAt *time.Time
	if ra := strings.TrimSpace(doc.Spec.RetiredAt); ra != "" {
		t, err := time.Parse(time.RFC3339, ra)
		if err != nil {
			return nil, fmt.Errorf("ai system version %s v%d: parse retired_at: %w", doc.Spec.AISystemID, doc.Spec.Version, err)
		}
		retiredAt = &t
	}

	frameworks := make([]string, 0, len(doc.Spec.ComplianceFrameworks))
	for _, f := range doc.Spec.ComplianceFrameworks {
		if t := strings.TrimSpace(f); t != "" {
			frameworks = append(frameworks, t)
		}
	}

	return &aisystem.AISystemVersion{
		AISystemID:           strings.TrimSpace(doc.Spec.AISystemID),
		Version:              doc.Spec.Version,
		ReleaseLabel:         strings.TrimSpace(doc.Spec.ReleaseLabel),
		ModelArtifact:        strings.TrimSpace(doc.Spec.ModelArtifact),
		ModelHash:            strings.TrimSpace(doc.Spec.ModelHash),
		Endpoint:             strings.TrimSpace(doc.Spec.Endpoint),
		Status:               status,
		EffectiveFrom:        effectiveFrom.UTC(),
		EffectiveUntil:       utcPtr(effectiveUntil),
		RetiredAt:            utcPtr(retiredAt),
		ComplianceFrameworks: frameworks,
		DocumentationURL:     strings.TrimSpace(doc.Spec.DocumentationURL),
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            strings.TrimSpace(createdBy),
	}, nil
}

// mapAISystemBindingDocumentToAISystemBinding maps a validated
// AISystemBindingDocument into the domain type. Junction posture: no
// status, no lifecycle. AISystemVersion is preserved as a *int so the
// "no version pin" case is distinguishable from version 0.
func mapAISystemBindingDocumentToAISystemBinding(doc types.AISystemBindingDocument, now time.Time, createdBy string) *aisystem.AISystemBinding {
	now = now.UTC()
	var version *int
	if doc.Spec.AISystemVersion != nil {
		v := *doc.Spec.AISystemVersion
		version = &v
	}
	return &aisystem.AISystemBinding{
		ID:                strings.TrimSpace(doc.Metadata.ID),
		AISystemID:        strings.TrimSpace(doc.Spec.AISystemID),
		AISystemVersion:   version,
		BusinessServiceID: strings.TrimSpace(doc.Spec.BusinessServiceID),
		CapabilityID:      strings.TrimSpace(doc.Spec.CapabilityID),
		ProcessID:         strings.TrimSpace(doc.Spec.ProcessID),
		SurfaceID:         strings.TrimSpace(doc.Spec.SurfaceID),
		Role:              strings.TrimSpace(doc.Spec.Role),
		Description:       strings.TrimSpace(doc.Spec.Description),
		CreatedAt:         now,
		CreatedBy:         strings.TrimSpace(createdBy),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
