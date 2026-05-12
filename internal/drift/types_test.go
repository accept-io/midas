package drift

import "testing"

// TestDriftType_NineV1ValuesOnly pins the V1 enumeration. The nine
// admitted values must round-trip through validDriftTypes; the five
// excluded values must NOT. The exclusion is explicit so a dropped
// check elsewhere does not let one through.
func TestDriftType_NineV1ValuesOnly(t *testing.T) {
	// 1) Admitted set is exactly the nine V1 values.
	wantAdmitted := map[DriftType]struct{}{
		DriftTypeInvocation:  {},
		DriftTypeOutcome:     {},
		DriftTypeConfidence:  {},
		DriftTypeLatency:     {},
		DriftTypeEvidence:    {},
		DriftTypeAuthority:   {},
		DriftTypePolicy:      {},
		DriftTypeCoverage:    {},
		DriftTypeProcessPath: {},
	}
	if len(validDriftTypes) != len(wantAdmitted) {
		t.Errorf("validDriftTypes size = %d, want %d", len(validDriftTypes), len(wantAdmitted))
	}
	for v := range wantAdmitted {
		if _, ok := validDriftTypes[v]; !ok {
			t.Errorf("validDriftTypes is missing V1 value %q", v)
		}
	}

	// 2) Excluded values must NOT appear in validDriftTypes.
	for _, banned := range []DriftType{"population", "data", "prediction", "performance", "concept"} {
		if _, ok := validDriftTypes[banned]; ok {
			t.Errorf("validDriftTypes must not contain V2-deferred %q", banned)
		}
		if _, ok := excludedDriftTypes[banned]; !ok {
			t.Errorf("excludedDriftTypes must list %q so the validator rejects it explicitly", banned)
		}
	}
}

// TestDriftSeriesPointStatus_FiveBandsExactly pins the five-band
// status enum. Both unknown_insufficient_data and unknown_detector_error
// must be present; a single "unknown" must not. The split exists
// because operators triage these differently.
func TestDriftSeriesPointStatus_FiveBandsExactly(t *testing.T) {
	want := []DriftSeriesPointStatus{
		DriftSeriesPointStatusHealthy,
		DriftSeriesPointStatusWarning,
		DriftSeriesPointStatusBreached,
		DriftSeriesPointStatusUnknownInsufficientData,
		DriftSeriesPointStatusUnknownDetectorError,
	}
	if len(want) != 5 {
		t.Fatalf("test fixture want has %d entries, expected 5", len(want))
	}

	// validBandStatuses is the shared underlying value set used by
	// DriftSeriesStatus, DriftSeriesPointStatus, and
	// DriftObservationDetectorStatus. It must contain exactly five
	// entries (the five band values), and must include both unknown
	// halves.
	if len(validBandStatuses) != 5 {
		t.Errorf("validBandStatuses size = %d, want 5", len(validBandStatuses))
	}
	for _, s := range want {
		if _, ok := validBandStatuses[string(s)]; !ok {
			t.Errorf("validBandStatuses missing %q", s)
		}
	}

	// Single "unknown" must NOT be a valid band value.
	if _, ok := validBandStatuses["unknown"]; ok {
		t.Error(`validBandStatuses must not contain a single "unknown" value; ` +
			`use unknown_insufficient_data or unknown_detector_error`)
	}

	// Both halves must be present under all three exposed types.
	if string(DriftSeriesStatusUnknownInsufficientData) != "unknown_insufficient_data" {
		t.Error("DriftSeriesStatusUnknownInsufficientData must be unknown_insufficient_data")
	}
	if string(DriftSeriesStatusUnknownDetectorError) != "unknown_detector_error" {
		t.Error("DriftSeriesStatusUnknownDetectorError must be unknown_detector_error")
	}
	if string(DriftObservationDetectorStatusUnknownInsufficientData) != "unknown_insufficient_data" {
		t.Error("DriftObservationDetectorStatusUnknownInsufficientData must be unknown_insufficient_data")
	}
	if string(DriftObservationDetectorStatusUnknownDetectorError) != "unknown_detector_error" {
		t.Error("DriftObservationDetectorStatusUnknownDetectorError must be unknown_detector_error")
	}
}

// TestDriftPointComputationMode_FourValuesExactly pins the four-value
// computation-mode enum. Backfill state lives here, NOT on any
// detector-status enum.
func TestDriftPointComputationMode_FourValuesExactly(t *testing.T) {
	want := map[DriftPointComputationMode]struct{}{
		DriftPointComputationModeRealtime:   {},
		DriftPointComputationModeBackfilled: {},
		DriftPointComputationModeCorrected:  {},
		DriftPointComputationModeImported:   {},
	}
	if len(validComputationModes) != 4 {
		t.Errorf("validComputationModes size = %d, want 4", len(validComputationModes))
	}
	for v := range want {
		if _, ok := validComputationModes[v]; !ok {
			t.Errorf("validComputationModes missing %q", v)
		}
	}

	// Backfill must NOT be encoded as a detector-status band.
	for _, banned := range []string{"backfilled", "imported", "corrected", "realtime"} {
		if _, ok := validBandStatuses[banned]; ok {
			t.Errorf("validBandStatuses must not contain %q (computation-provenance state belongs on DriftPointComputationMode)", banned)
		}
	}
}

// TestBaselineStrategy_V1ValuesOnly pins the five V1 strategies.
// Champion-challenger and seasonality-aware are deferred to V2.
func TestBaselineStrategy_V1ValuesOnly(t *testing.T) {
	want := map[BaselineStrategy]struct{}{
		BaselineStrategyRolling:                 {},
		BaselineStrategyFixedGoverned:           {},
		BaselineStrategyPreviousEquivalent:      {},
		BaselineStrategySinceLastGovernedChange: {},
		BaselineStrategyManuallyPinned:          {},
	}
	if len(validBaselineStrategies) != 5 {
		t.Errorf("validBaselineStrategies size = %d, want 5", len(validBaselineStrategies))
	}
	for v := range want {
		if _, ok := validBaselineStrategies[v]; !ok {
			t.Errorf("validBaselineStrategies missing %q", v)
		}
	}

	for _, banned := range []BaselineStrategy{"champion_challenger", "seasonality_aware"} {
		if _, ok := validBaselineStrategies[banned]; ok {
			t.Errorf("validBaselineStrategies must not contain V2-deferred %q", banned)
		}
	}
}

// TestTargetEntityKind_NineKinds pins the closed enumeration over the
// nine governed-entity kinds.
func TestTargetEntityKind_NineKinds(t *testing.T) {
	want := map[TargetEntityKind]struct{}{
		TargetEntityKindBusinessService:  {},
		TargetEntityKindCapability:       {},
		TargetEntityKindProcess:          {},
		TargetEntityKindDecisionSurface:  {},
		TargetEntityKindAISystem:         {},
		TargetEntityKindAISystemBinding:  {},
		TargetEntityKindAgent:            {},
		TargetEntityKindAuthorityProfile: {},
		TargetEntityKindAuthorityGrant:   {},
	}
	if len(validTargetEntityKinds) != 9 {
		t.Errorf("validTargetEntityKinds size = %d, want 9", len(validTargetEntityKinds))
	}
	for v := range want {
		if _, ok := validTargetEntityKinds[v]; !ok {
			t.Errorf("validTargetEntityKinds missing %q", v)
		}
	}
}

// TestObservationOperatorStatus_FiveValues pins the operator-side
// status enum. Decoupled from detector status.
func TestObservationOperatorStatus_FiveValues(t *testing.T) {
	want := map[DriftObservationOperatorStatus]struct{}{
		DriftObservationOperatorStatusOpen:       {},
		DriftObservationOperatorStatusTriaged:    {},
		DriftObservationOperatorStatusResolved:   {},
		DriftObservationOperatorStatusAccepted:   {},
		DriftObservationOperatorStatusSuperseded: {},
	}
	if len(validOperatorStatuses) != 5 {
		t.Errorf("validOperatorStatuses size = %d, want 5", len(validOperatorStatuses))
	}
	for v := range want {
		if _, ok := validOperatorStatuses[v]; !ok {
			t.Errorf("validOperatorStatuses missing %q", v)
		}
	}
}
