package bootstrap

// synthetic_drift_test.go — Drift-2a synthetic drift seed contract.
//
// The synthetic seed must:
//   - populate the drift repositories with a non-empty plausible dataset
//     covering every V1 drift type, every V1 target entity kind, and
//     every five-band status;
//   - be safe to re-run (no duplicates, no errors, no Update calls);
//   - produce domain entities that pass drift.Validate*;
//   - reference only IDs that the existing structural demo seed creates;
//   - remain runtime-inert (no Update / UpdateStatus /
//     UpdateOperatorStatus / Supersede / Seal / DeleteBefore calls in
//     synthetic_drift.go).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
	"github.com/accept-io/midas/internal/store"
)

// ---------------------------------------------------------------------------
// 1. Fresh seed creates the full synthetic dataset
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_FreshPopulatesDataset(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("SeedSyntheticDrift on fresh store: %v", err)
	}

	plan := syntheticDriftPlan()
	if len(plan) < 6 {
		t.Fatalf("plan must contain at least 6 definitions per Drift-2a brief; got %d", len(plan))
	}

	for _, p := range plan {
		def, err := repos.DriftDefinitions.FindByIDAndVersion(ctx, p.ID, 1)
		if err != nil {
			t.Errorf("FindByIDAndVersion(%s, 1): %v", p.ID, err)
		}
		if def == nil {
			t.Errorf("expected definition %s v1 to exist", p.ID)
			continue
		}
		if def.Status != drift.DriftDefinitionStatusActive {
			t.Errorf("%s: status = %q, want active", p.ID, def.Status)
		}
		if def.CreatedBy != syntheticDriftCreator {
			t.Errorf("%s: CreatedBy = %q, want %q", p.ID, def.CreatedBy, syntheticDriftCreator)
		}
		if def.ApprovedBy != syntheticDriftCreator {
			t.Errorf("%s: ApprovedBy = %q, want %q", p.ID, def.ApprovedBy, syntheticDriftCreator)
		}
		if def.TargetEntityID != p.TargetEntityID {
			t.Errorf("%s: TargetEntityID = %q, want %q", p.ID, def.TargetEntityID, p.TargetEntityID)
		}

		for _, m := range p.Metrics {
			seriesID := syntheticSeriesID(p.ID, m.MetricID)
			s, err := repos.DriftSeries.FindByID(ctx, seriesID)
			if err != nil {
				t.Errorf("FindByID(series %s): %v", seriesID, err)
			}
			if s == nil {
				t.Errorf("expected series %s to exist", seriesID)
				continue
			}

			points, err := repos.DriftSeriesPoints.ListBySeries(ctx, seriesID, time.Time{}, 0)
			if err != nil {
				t.Errorf("ListBySeries(%s): %v", seriesID, err)
			}
			if len(points) < syntheticPointCount {
				t.Errorf("series %s: got %d points, want >= %d", seriesID, len(points), syntheticPointCount)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 2. Coverage: all V1 drift types, target kinds, statuses, plus ≥1 backfilled
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_CoversAllV1DriftTypes(t *testing.T) {
	want := map[drift.DriftType]struct{}{
		drift.DriftTypeInvocation:  {},
		drift.DriftTypeOutcome:     {},
		drift.DriftTypeConfidence:  {},
		drift.DriftTypeLatency:     {},
		drift.DriftTypeEvidence:    {},
		drift.DriftTypeAuthority:   {},
		drift.DriftTypePolicy:      {},
		drift.DriftTypeCoverage:    {},
		drift.DriftTypeProcessPath: {},
	}
	got := map[drift.DriftType]struct{}{}
	for _, d := range syntheticDriftPlan() {
		for _, m := range d.Metrics {
			got[m.DriftType] = struct{}{}
		}
	}
	for dt := range want {
		if _, ok := got[dt]; !ok {
			t.Errorf("plan missing V1 drift type %q", dt)
		}
	}
}

func TestSeedSyntheticDrift_CoversAllV1TargetEntityKinds(t *testing.T) {
	want := map[drift.TargetEntityKind]struct{}{
		drift.TargetEntityKindBusinessService:  {},
		drift.TargetEntityKindCapability:       {},
		drift.TargetEntityKindProcess:          {},
		drift.TargetEntityKindDecisionSurface:  {},
		drift.TargetEntityKindAISystem:         {},
		drift.TargetEntityKindAISystemBinding:  {},
		drift.TargetEntityKindAgent:            {},
		drift.TargetEntityKindAuthorityProfile: {},
		drift.TargetEntityKindAuthorityGrant:   {},
	}
	got := map[drift.TargetEntityKind]struct{}{}
	for _, d := range syntheticDriftPlan() {
		got[d.TargetEntityKind] = struct{}{}
	}
	for tk := range want {
		if _, ok := got[tk]; !ok {
			t.Errorf("plan missing V1 target entity kind %q", tk)
		}
	}
}

func TestSeedSyntheticDrift_CoversAllFiveStatusBands(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)
	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("SeedSyntheticDrift: %v", err)
	}

	want := map[drift.DriftSeriesStatus]struct{}{
		drift.DriftSeriesStatusHealthy:                 {},
		drift.DriftSeriesStatusWarning:                 {},
		drift.DriftSeriesStatusBreached:                {},
		drift.DriftSeriesStatusUnknownInsufficientData: {},
		drift.DriftSeriesStatusUnknownDetectorError:    {},
	}
	got := map[drift.DriftSeriesStatus]struct{}{}
	for _, d := range syntheticDriftPlan() {
		for _, m := range d.Metrics {
			s, err := repos.DriftSeries.FindByID(ctx, syntheticSeriesID(d.ID, m.MetricID))
			if err != nil {
				t.Fatalf("FindByID series: %v", err)
			}
			if s == nil {
				continue
			}
			got[s.Status] = struct{}{}
		}
	}
	for st := range want {
		if _, ok := got[st]; !ok {
			t.Errorf("plan missing series status band %q", st)
		}
	}
}

func TestSeedSyntheticDrift_EmitsBackfilledPointAndObservation(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)
	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("SeedSyntheticDrift: %v", err)
	}

	// At least one point with ComputationMode=backfilled and a
	// BackfillRunID must exist.
	foundBackfilledPoint := false
	for _, d := range syntheticDriftPlan() {
		for _, m := range d.Metrics {
			seriesID := syntheticSeriesID(d.ID, m.MetricID)
			points, err := repos.DriftSeriesPoints.ListBySeries(ctx, seriesID, time.Time{}, 0)
			if err != nil {
				t.Fatalf("ListBySeries(%s): %v", seriesID, err)
			}
			for _, p := range points {
				if p.ComputationMode == drift.DriftPointComputationModeBackfilled {
					if p.BackfillRunID == "" {
						t.Errorf("point %s: backfilled but BackfillRunID empty", p.ID)
					}
					foundBackfilledPoint = true
				}
			}
		}
	}
	if !foundBackfilledPoint {
		t.Error("expected at least one backfilled point in the synthetic dataset")
	}

	// At least one observation must be Backfilled=true with a non-empty
	// BackfillRunID.
	foundBackfilledObs := false
	for _, d := range syntheticDriftPlan() {
		obs, err := repos.DriftObservations.ListByDefinition(ctx, d.ID)
		if err != nil {
			t.Fatalf("ListByDefinition(%s): %v", d.ID, err)
		}
		for _, o := range obs {
			if o.Backfilled {
				if o.BackfillRunID == "" {
					t.Errorf("observation %s: backfilled but BackfillRunID empty", o.ID)
				}
				foundBackfilledObs = true
			}
		}
	}
	if !foundBackfilledObs {
		t.Error("expected at least one backfilled observation in the synthetic dataset")
	}
}

func TestSeedSyntheticDrift_EmitsExpectedAnnotations(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)
	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("SeedSyntheticDrift: %v", err)
	}

	// Brief specifies two annotations: known_business_change on the
	// credit-outcome breach series, remediation_note on the
	// merchant-fraud warning series.
	creditSeriesID := syntheticSeriesID("drift-demo-credit-outcome", "approve-rate")
	merchantSeriesID := syntheticSeriesID("drift-demo-merchant-fraud-invocation", "invocation-rate")

	creditAnnos, err := repos.DriftAnnotations.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, creditSeriesID)
	if err != nil {
		t.Fatalf("ListByTarget(credit series): %v", err)
	}
	if !annotationsContain(creditAnnos, drift.DriftAnnotationTypeKnownBusinessChange) {
		t.Errorf("credit-outcome series %q missing known_business_change annotation", creditSeriesID)
	}

	merchantAnnos, err := repos.DriftAnnotations.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, merchantSeriesID)
	if err != nil {
		t.Fatalf("ListByTarget(merchant series): %v", err)
	}
	if !annotationsContain(merchantAnnos, drift.DriftAnnotationTypeRemediationNote) {
		t.Errorf("merchant-fraud series %q missing remediation_note annotation", merchantSeriesID)
	}
}

func annotationsContain(annos []*drift.DriftAnnotation, want drift.DriftAnnotationType) bool {
	for _, a := range annos {
		if a.AnnotationType == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 3. Validation: every produced entity is well-formed under drift.Validate*
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_ProducedEntitiesPassValidate(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)
	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("SeedSyntheticDrift: %v", err)
	}

	now := syntheticDriftEpoch.Add(syntheticPointCount * syntheticDayWindow).Add(time.Hour)

	for _, p := range syntheticDriftPlan() {
		def, _ := repos.DriftDefinitions.FindByIDAndVersion(ctx, p.ID, 1)
		if def == nil {
			t.Fatalf("definition %s missing", p.ID)
		}
		if errs := drift.Validate(def); len(errs) > 0 {
			t.Errorf("definition %s failed Validate: %v", p.ID, errs)
		}

		for _, m := range p.Metrics {
			seriesID := syntheticSeriesID(p.ID, m.MetricID)
			s, _ := repos.DriftSeries.FindByID(ctx, seriesID)
			if s == nil {
				t.Errorf("series %s missing", seriesID)
				continue
			}
			if errs := drift.ValidateSeries(s); len(errs) > 0 {
				t.Errorf("series %s failed ValidateSeries: %v", seriesID, errs)
			}

			points, _ := repos.DriftSeriesPoints.ListBySeries(ctx, seriesID, time.Time{}, 0)
			for _, pt := range points {
				if errs := drift.ValidatePoint(pt); len(errs) > 0 {
					t.Errorf("point %s failed ValidatePoint: %v", pt.ID, errs)
				}
			}
		}

		obs, _ := repos.DriftObservations.ListByDefinition(ctx, p.ID)
		for _, o := range obs {
			if errs := drift.ValidateObservation(o); len(errs) > 0 {
				t.Errorf("observation %s failed ValidateObservation: %v", o.ID, errs)
			}
		}
	}

	// Annotations.
	for _, p := range syntheticDriftPlan() {
		for _, m := range p.Metrics {
			if m.Annotation == nil {
				continue
			}
			seriesID := syntheticSeriesID(p.ID, m.MetricID)
			annos, _ := repos.DriftAnnotations.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, seriesID)
			for _, a := range annos {
				if errs := drift.ValidateAnnotation(a, now); len(errs) > 0 {
					t.Errorf("annotation %s failed ValidateAnnotation: %v", a.ID, errs)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Idempotency: second invocation creates no new rows
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)

	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("first SeedSyntheticDrift: %v", err)
	}

	before := snapshotDriftCounts(t, ctx, repos)

	if err := SeedSyntheticDrift(ctx, repos); err != nil {
		t.Fatalf("second SeedSyntheticDrift: %v", err)
	}

	after := snapshotDriftCounts(t, ctx, repos)

	if before != after {
		t.Errorf("idempotency violation: counts changed across invocations: before=%+v after=%+v", before, after)
	}
}

type driftCounts struct {
	Definitions  int
	Series       int
	Points       int
	Observations int
	Annotations  int
}

func snapshotDriftCounts(t *testing.T, ctx context.Context, repos *store.Repositories) driftCounts {
	t.Helper()
	plan := syntheticDriftPlan()
	c := driftCounts{}
	for _, p := range plan {
		def, _ := repos.DriftDefinitions.FindByIDAndVersion(ctx, p.ID, 1)
		if def != nil {
			c.Definitions++
		}
		obs, _ := repos.DriftObservations.ListByDefinition(ctx, p.ID)
		c.Observations += len(obs)
		for _, m := range p.Metrics {
			seriesID := syntheticSeriesID(p.ID, m.MetricID)
			s, _ := repos.DriftSeries.FindByID(ctx, seriesID)
			if s != nil {
				c.Series++
			}
			points, _ := repos.DriftSeriesPoints.ListBySeries(ctx, seriesID, time.Time{}, 0)
			c.Points += len(points)
			annos, _ := repos.DriftAnnotations.ListByTarget(ctx, drift.DriftAnnotationTargetKindSeries, seriesID)
			c.Annotations += len(annos)
		}
	}
	return c
}

// ---------------------------------------------------------------------------
// 5. Determinism: two fresh seeds yield byte-identical entity shapes
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_IsDeterministic(t *testing.T) {
	ctx := context.Background()
	reposA := freshRepos(t)
	reposB := freshRepos(t)

	if err := SeedSyntheticDrift(ctx, reposA); err != nil {
		t.Fatalf("SeedSyntheticDrift A: %v", err)
	}
	if err := SeedSyntheticDrift(ctx, reposB); err != nil {
		t.Fatalf("SeedSyntheticDrift B: %v", err)
	}

	plan := syntheticDriftPlan()
	for _, p := range plan {
		defA, _ := reposA.DriftDefinitions.FindByIDAndVersion(ctx, p.ID, 1)
		defB, _ := reposB.DriftDefinitions.FindByIDAndVersion(ctx, p.ID, 1)
		if defA == nil || defB == nil {
			t.Fatalf("definition %s missing on one side", p.ID)
		}
		if !defA.EffectiveDate.Equal(defB.EffectiveDate) {
			t.Errorf("%s: EffectiveDate differs across runs", p.ID)
		}

		for _, m := range p.Metrics {
			seriesID := syntheticSeriesID(p.ID, m.MetricID)
			ptsA, _ := reposA.DriftSeriesPoints.ListBySeries(ctx, seriesID, time.Time{}, 0)
			ptsB, _ := reposB.DriftSeriesPoints.ListBySeries(ctx, seriesID, time.Time{}, 0)
			if len(ptsA) != len(ptsB) {
				t.Errorf("series %s: point count differs across runs (%d vs %d)", seriesID, len(ptsA), len(ptsB))
				continue
			}
			for i := range ptsA {
				if ptsA[i].ID != ptsB[i].ID {
					t.Errorf("series %s pt %d: ID differs (%q vs %q)", seriesID, i, ptsA[i].ID, ptsB[i].ID)
				}
				if ptsA[i].Magnitude != ptsB[i].Magnitude {
					t.Errorf("series %s pt %d: Magnitude differs (%v vs %v)", seriesID, i, ptsA[i].Magnitude, ptsB[i].Magnitude)
				}
				if ptsA[i].Status != ptsB[i].Status {
					t.Errorf("series %s pt %d: Status differs (%q vs %q)", seriesID, i, ptsA[i].Status, ptsB[i].Status)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// 6. Target IDs all resolve in the existing demo seed
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_TargetIDsResolveInDemoSeed(t *testing.T) {
	ctx := context.Background()
	repos := freshRepos(t)
	if err := SeedDemo(ctx, repos); err != nil {
		t.Fatalf("SeedDemo: %v", err)
	}

	for _, p := range syntheticDriftPlan() {
		switch p.TargetEntityKind {
		case drift.TargetEntityKindBusinessService:
			bs, err := repos.BusinessServices.GetByID(ctx, p.TargetEntityID)
			if err != nil || bs == nil {
				t.Errorf("BusinessService %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindCapability:
			c, err := repos.Capabilities.GetByID(ctx, p.TargetEntityID)
			if err != nil || c == nil {
				t.Errorf("Capability %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindProcess:
			pr, err := repos.Processes.GetByID(ctx, p.TargetEntityID)
			if err != nil || pr == nil {
				t.Errorf("Process %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindDecisionSurface:
			s, err := repos.Surfaces.FindLatestByID(ctx, p.TargetEntityID)
			if err != nil || s == nil {
				t.Errorf("DecisionSurface %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindAISystem:
			sys, err := repos.AISystems.GetByID(ctx, p.TargetEntityID)
			if err != nil || sys == nil {
				t.Errorf("AISystem %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindAISystemBinding:
			b, err := repos.AISystemBindings.GetByID(ctx, p.TargetEntityID)
			if err != nil || b == nil {
				t.Errorf("AISystemBinding %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindAgent:
			a, err := repos.Agents.GetByID(ctx, p.TargetEntityID)
			if err != nil || a == nil {
				t.Errorf("Agent %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindAuthorityProfile:
			prof, err := repos.Profiles.FindByIDAndVersion(ctx, p.TargetEntityID, 1)
			if err != nil || prof == nil {
				t.Errorf("AuthorityProfile %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		case drift.TargetEntityKindAuthorityGrant:
			g, err := repos.Grants.FindByID(ctx, p.TargetEntityID)
			if err != nil || g == nil {
				t.Errorf("AuthorityGrant %s referenced by %s not present in demo seed (err=%v)",
					p.TargetEntityID, p.ID, err)
			}
		default:
			t.Errorf("definition %s has unrecognised TargetEntityKind %q", p.ID, p.TargetEntityKind)
		}
	}
}

// ---------------------------------------------------------------------------
// 7. Nil-safety: missing repositories are rejected with a clear error
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_RejectsMissingRepositories(t *testing.T) {
	ctx := context.Background()
	if err := SeedSyntheticDrift(ctx, nil); err == nil {
		t.Error("SeedSyntheticDrift(nil) should return an error")
	}

	repos := freshRepos(t)
	repos.DriftSeries = nil
	if err := SeedSyntheticDrift(ctx, repos); err == nil {
		t.Error("SeedSyntheticDrift with nil DriftSeries should return an error")
	}
}

// ---------------------------------------------------------------------------
// 8. Source-pin: synthetic seed remains runtime-inert
//
// Drift-2a must not invoke any mutating repo method beyond Create.
// Asserting against the source file is brittle in the small but
// extremely cheap to maintain — and prevents a future cleanup from
// silently introducing a UpdateStatus / Supersede call that would mark
// the seed runtime-active.
// ---------------------------------------------------------------------------

func TestSeedSyntheticDrift_SourceRemainsRuntimeInert(t *testing.T) {
	src := readSourceFile(t, "synthetic_drift.go")
	forbidden := []string{
		".Update(",
		".UpdateStatus(",
		".UpdateOperatorStatus(",
		".Supersede(",
		".Seal(",
		".DeleteBefore(",
	}
	for _, sub := range forbidden {
		if strings.Contains(src, sub) {
			t.Errorf("synthetic_drift.go must remain runtime-inert; found forbidden mutating call %q", sub)
		}
	}
	// And it must not import the decision package — that would put the
	// generator on the runtime side of the structural-layer split.
	if strings.Contains(src, `"github.com/accept-io/midas/internal/decision"`) {
		t.Error("synthetic_drift.go must not import internal/decision")
	}
}

func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(wd, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}
