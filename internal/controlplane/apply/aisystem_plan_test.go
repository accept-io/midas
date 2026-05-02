package apply

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/surface"
)

// ---------------------------------------------------------------------------
// Stubs (apply-side, narrow)
// ---------------------------------------------------------------------------

type stubAISystemRepo struct {
	items map[string]*aisystem.AISystem
}

func newStubAISystemRepo() *stubAISystemRepo {
	return &stubAISystemRepo{items: map[string]*aisystem.AISystem{}}
}

func (r *stubAISystemRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.items[id]
	return ok, nil
}

func (r *stubAISystemRepo) GetByID(_ context.Context, id string) (*aisystem.AISystem, error) {
	sys, ok := r.items[id]
	if !ok {
		return nil, aisystem.ErrAISystemNotFound
	}
	return sys, nil
}

func (r *stubAISystemRepo) Create(_ context.Context, sys *aisystem.AISystem) error {
	r.items[sys.ID] = sys
	return nil
}

func (r *stubAISystemRepo) Update(_ context.Context, sys *aisystem.AISystem) error {
	r.items[sys.ID] = sys
	return nil
}

type stubAIVersionRepo struct {
	items map[string]*aisystem.AISystemVersion // key = sysID + "\x00" + versionStr
}

func newStubAIVersionRepo() *stubAIVersionRepo {
	return &stubAIVersionRepo{items: map[string]*aisystem.AISystemVersion{}}
}

func (r *stubAIVersionRepo) GetByIDAndVersion(_ context.Context, sysID string, version int) (*aisystem.AISystemVersion, error) {
	key := versionKey(sysID, version)
	ver, ok := r.items[key]
	if !ok {
		return nil, aisystem.ErrAISystemVersionNotFound
	}
	return ver, nil
}

func (r *stubAIVersionRepo) Create(_ context.Context, ver *aisystem.AISystemVersion) error {
	r.items[versionKey(ver.AISystemID, ver.Version)] = ver
	return nil
}

func versionKey(sysID string, v int) string {
	out := sysID + "\x00"
	// inline integer formatting to avoid pulling strconv into a test stub.
	if v == 0 {
		return out + "0"
	}
	digits := make([]byte, 0, 10)
	n := v
	if n < 0 {
		out += "-"
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return out + string(digits)
}

type stubAIBindingRepo struct {
	items map[string]*aisystem.AISystemBinding
}

func newStubAIBindingRepo() *stubAIBindingRepo {
	return &stubAIBindingRepo{items: map[string]*aisystem.AISystemBinding{}}
}

func (r *stubAIBindingRepo) GetByID(_ context.Context, id string) (*aisystem.AISystemBinding, error) {
	b, ok := r.items[id]
	if !ok {
		return nil, aisystem.ErrAISystemBindingNotFound
	}
	return b, nil
}

func (r *stubAIBindingRepo) Create(_ context.Context, b *aisystem.AISystemBinding) error {
	r.items[b.ID] = b
	return nil
}

// stubProcessRepoFull adapts the apply-side ProcessRepository contract on a
// Go map. Only the methods the planner actually calls are populated.
type stubProcessRepoFull struct {
	items map[string]*process.Process
}

func (r *stubProcessRepoFull) Exists(_ context.Context, id string) (bool, error) {
	_, ok := r.items[id]
	return ok, nil
}

func (r *stubProcessRepoFull) GetByID(_ context.Context, id string) (*process.Process, error) {
	return r.items[id], nil
}

func (r *stubProcessRepoFull) Create(_ context.Context, _ *process.Process) error { return nil }

type stubCapabilityRepoFull struct {
	exists map[string]bool
}

func (r *stubCapabilityRepoFull) Exists(_ context.Context, id string) (bool, error) {
	return r.exists[id], nil
}

func (r *stubCapabilityRepoFull) GetByID(_ context.Context, id string) (*capability.Capability, error) {
	if r.exists[id] {
		return &capability.Capability{ID: id, Status: "active"}, nil
	}
	return nil, nil
}

func (r *stubCapabilityRepoFull) Create(_ context.Context, _ *capability.Capability) error {
	return nil
}

type stubBSCRepoFull struct {
	pairs map[string]bool // key = bs + "\x00" + cap
}

func (r *stubBSCRepoFull) Exists(_ context.Context, bsID, capID string) (bool, error) {
	return r.pairs[bsID+"\x00"+capID], nil
}

func (r *stubBSCRepoFull) Create(_ context.Context, _ *businessservicecapability.BusinessServiceCapability) error {
	return nil
}

type stubSurfaceRepoFull struct {
	items map[string]*surface.DecisionSurface
}

func (r *stubSurfaceRepoFull) FindLatestByID(_ context.Context, id string) (*surface.DecisionSurface, error) {
	return r.items[id], nil
}

func (r *stubSurfaceRepoFull) Create(_ context.Context, _ *surface.DecisionSurface) error { return nil }

// ---------------------------------------------------------------------------
// Mapper tests
// ---------------------------------------------------------------------------

func TestMapper_AISystem_TrimsAndDefaults(t *testing.T) {
	doc := types.AISystemDocument{
		Metadata: types.DocumentMetadata{ID: "  ai-1  ", Name: "  AI 1  "},
		Spec: types.AISystemSpec{
			Description: "  desc  ",
			Owner:       " team ",
			Status:      "", // mapper supplies "active"
			Origin:      "", // mapper supplies "manual"
		},
	}
	now := time.Now()
	sys := mapAISystemDocumentToAISystem(doc, now, "  user  ")
	if sys.ID != "ai-1" || sys.Name != "AI 1" || sys.Owner != "team" {
		t.Errorf("trim mismatch: %+v", sys)
	}
	if sys.Status != "active" || sys.Origin != "manual" {
		t.Errorf("default mismatch: status=%q origin=%q", sys.Status, sys.Origin)
	}
	if !sys.Managed {
		t.Errorf("Managed must default true")
	}
	if !sys.CreatedAt.Equal(now.UTC()) {
		t.Errorf("CreatedAt should be UTC of now: got %v", sys.CreatedAt)
	}
	if sys.CreatedBy != "user" {
		t.Errorf("CreatedBy: %q", sys.CreatedBy)
	}
}

func TestMapper_AISystem_HonoursDeclaredStatus(t *testing.T) {
	doc := types.AISystemDocument{
		Metadata: types.DocumentMetadata{ID: "ai-1", Name: "AI 1"},
		Spec:     types.AISystemSpec{Status: "deprecated", Origin: "inferred"},
	}
	sys := mapAISystemDocumentToAISystem(doc, time.Now(), "u")
	if sys.Status != "deprecated" || sys.Origin != "inferred" {
		t.Errorf("status-honour mismatch: %+v", sys)
	}
}

func TestMapper_AISystemVersion_TrimsAndParsesDates(t *testing.T) {
	doc := types.AISystemVersionDocument{
		Metadata: types.DocumentMetadata{ID: "aiv-1"},
		Spec: types.AISystemVersionSpec{
			AISystemID:           "  ai-1  ",
			Version:              2,
			ReleaseLabel:         "  r1  ",
			Status:               "review", // honoured, not forced
			EffectiveFrom:        "2026-04-15T00:00:00Z",
			EffectiveUntil:       "2026-05-15T00:00:00Z",
			ComplianceFrameworks: []string{" iso-42001 ", "", "soc2"},
		},
	}
	now := time.Now()
	ver, err := mapAISystemVersionDocumentToAISystemVersion(doc, now, "u")
	if err != nil {
		t.Fatalf("mapper: %v", err)
	}
	if ver.AISystemID != "ai-1" || ver.Version != 2 || ver.ReleaseLabel != "r1" {
		t.Errorf("trim mismatch: %+v", ver)
	}
	if ver.Status != "review" {
		t.Errorf("status must be honoured (no review-forcing): got %q", ver.Status)
	}
	if ver.EffectiveFrom.IsZero() || ver.EffectiveUntil == nil {
		t.Errorf("date parse failed: %+v", ver)
	}
	if len(ver.ComplianceFrameworks) != 2 || ver.ComplianceFrameworks[0] != "iso-42001" {
		t.Errorf("compliance trim/empty-skip failed: %v", ver.ComplianceFrameworks)
	}
}

func TestMapper_AISystemBinding_PreservesVersionPointer(t *testing.T) {
	v := 7
	doc := types.AISystemBindingDocument{
		Metadata: types.DocumentMetadata{ID: "bind-1"},
		Spec: types.AISystemBindingSpec{
			AISystemID:        "ai-1",
			AISystemVersion:   &v,
			BusinessServiceID: "bs-x",
		},
	}
	b := mapAISystemBindingDocumentToAISystemBinding(doc, time.Now(), "u")
	if b.AISystemVersion == nil || *b.AISystemVersion != 7 {
		t.Errorf("version pointer not preserved: %v", b.AISystemVersion)
	}
}

// ---------------------------------------------------------------------------
// Planner: AISystem
// ---------------------------------------------------------------------------

func makeAISystemPlannerDoc(id, replaces string) parser.ParsedDocument {
	doc := types.AISystemDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAISystem,
		Metadata:   types.DocumentMetadata{ID: id, Name: id},
		Spec:       types.AISystemSpec{Status: "active", Origin: "manual", Replaces: replaces},
	}
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

func TestPlanAISystem_CreateWhenNotPersisted(t *testing.T) {
	svc := &Service{aiSystemRepo: newStubAISystemRepo()}
	entry := ApplyPlanEntry{Kind: types.KindAISystem, ID: "ai-new", DocumentIndex: 1}
	svc.planAISystemEntry(context.Background(), makeAISystemPlannerDoc("ai-new", ""), nil, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create, got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
}

func TestPlanAISystem_ConflictWhenExists(t *testing.T) {
	repo := newStubAISystemRepo()
	repo.items["ai-existing"] = &aisystem.AISystem{ID: "ai-existing"}
	svc := &Service{aiSystemRepo: repo}
	entry := ApplyPlanEntry{Kind: types.KindAISystem, ID: "ai-existing", DocumentIndex: 1}
	svc.planAISystemEntry(context.Background(), makeAISystemPlannerDoc("ai-existing", ""), nil, &entry)
	if entry.Action != ApplyActionConflict {
		t.Errorf("Action: want Conflict, got %s", entry.Action)
	}
	if !strings.Contains(entry.Message, "already exists") {
		t.Errorf("expected already-exists message; got %q", entry.Message)
	}
}

func TestPlanAISystem_InvalidWhenReplacesMissing(t *testing.T) {
	svc := &Service{aiSystemRepo: newStubAISystemRepo()}
	entry := ApplyPlanEntry{Kind: types.KindAISystem, ID: "ai-new", DocumentIndex: 1}
	svc.planAISystemEntry(context.Background(), makeAISystemPlannerDoc("ai-new", "ai-ghost"), nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (replaces unresolved), got %s", entry.Action)
	}
}

func TestPlanAISystem_CreateWhenReplacesInBundle(t *testing.T) {
	svc := &Service{aiSystemRepo: newStubAISystemRepo()}
	entry := ApplyPlanEntry{Kind: types.KindAISystem, ID: "ai-new", DocumentIndex: 1}
	bundle := map[string]struct{}{"ai-old": {}}
	svc.planAISystemEntry(context.Background(), makeAISystemPlannerDoc("ai-new", "ai-old"), bundle, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create (bundle-resolved replaces), got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// Planner: AISystemVersion
// ---------------------------------------------------------------------------

func makeAIVersionPlannerDoc(id, sysID string, version int) parser.ParsedDocument {
	doc := types.AISystemVersionDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAISystemVersion,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec: types.AISystemVersionSpec{
			AISystemID:    sysID,
			Version:       version,
			Status:        "active",
			EffectiveFrom: "2026-04-15T00:00:00Z",
		},
	}
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

func TestPlanAIVersion_CreateWhenNotPersisted(t *testing.T) {
	sysRepo := newStubAISystemRepo()
	sysRepo.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	svc := &Service{aiSystemRepo: sysRepo, aiVersionRepo: newStubAIVersionRepo()}
	entry := ApplyPlanEntry{Kind: types.KindAISystemVersion, ID: "aiv-1", DocumentIndex: 1}
	svc.planAISystemVersionEntry(context.Background(), makeAIVersionPlannerDoc("aiv-1", "ai-1", 1), nil, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create, got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
	if entry.NewVersion != 1 {
		t.Errorf("NewVersion: want 1, got %d", entry.NewVersion)
	}
}

func TestPlanAIVersion_ConflictWhenTupleExists(t *testing.T) {
	sysRepo := newStubAISystemRepo()
	sysRepo.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	verRepo := newStubAIVersionRepo()
	verRepo.items[versionKey("ai-1", 1)] = &aisystem.AISystemVersion{AISystemID: "ai-1", Version: 1}
	svc := &Service{aiSystemRepo: sysRepo, aiVersionRepo: verRepo}
	entry := ApplyPlanEntry{Kind: types.KindAISystemVersion, ID: "aiv-1", DocumentIndex: 1}
	svc.planAISystemVersionEntry(context.Background(), makeAIVersionPlannerDoc("aiv-1", "ai-1", 1), nil, &entry)
	if entry.Action != ApplyActionConflict {
		t.Errorf("Action: want Conflict, got %s", entry.Action)
	}
}

func TestPlanAIVersion_InvalidWhenSystemMissing(t *testing.T) {
	svc := &Service{aiSystemRepo: newStubAISystemRepo(), aiVersionRepo: newStubAIVersionRepo()}
	entry := ApplyPlanEntry{Kind: types.KindAISystemVersion, ID: "aiv-1", DocumentIndex: 1}
	svc.planAISystemVersionEntry(context.Background(), makeAIVersionPlannerDoc("aiv-1", "ai-ghost", 1), nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid, got %s", entry.Action)
	}
}

func TestPlanAIVersion_CreateWhenSystemInBundle(t *testing.T) {
	svc := &Service{aiSystemRepo: newStubAISystemRepo(), aiVersionRepo: newStubAIVersionRepo()}
	bundle := map[string]struct{}{"ai-bndl": {}}
	entry := ApplyPlanEntry{Kind: types.KindAISystemVersion, ID: "aiv-1", DocumentIndex: 1}
	svc.planAISystemVersionEntry(context.Background(), makeAIVersionPlannerDoc("aiv-1", "ai-bndl", 1), bundle, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create (bundle-resolved system), got %s", entry.Action)
	}
}

// ---------------------------------------------------------------------------
// Planner: AISystemBinding — covers the five cross-reference rules
// ---------------------------------------------------------------------------

func makeAIBindingPlannerDoc(id string, mut func(spec *types.AISystemBindingSpec)) parser.ParsedDocument {
	spec := types.AISystemBindingSpec{
		AISystemID:        "ai-1",
		BusinessServiceID: "bs-1",
	}
	if mut != nil {
		mut(&spec)
	}
	doc := types.AISystemBindingDocument{
		APIVersion: types.APIVersionV1,
		Kind:       types.KindAISystemBinding,
		Metadata:   types.DocumentMetadata{ID: id},
		Spec:       spec,
	}
	return parser.ParsedDocument{Kind: doc.Kind, ID: doc.Metadata.ID, Doc: doc}
}

// newAIBindingPlannerSvc wires every repo the binding planner can touch with
// happy-path data: ai-1 system, bs-1 BS, cap-1 capability, proc-1 process whose
// BS=bs-1, surf-1 whose process=proc-1, and BSC(bs-1, cap-1).
func newAIBindingPlannerSvc() *Service {
	sys := newStubAISystemRepo()
	sys.items["ai-1"] = &aisystem.AISystem{ID: "ai-1"}
	ver := newStubAIVersionRepo()
	ver.items[versionKey("ai-1", 1)] = &aisystem.AISystemVersion{AISystemID: "ai-1", Version: 1}
	binding := newStubAIBindingRepo()
	bs := &stubBSExistsRepo{exists: map[string]bool{"bs-1": true, "bs-other": true}}
	cap := &stubCapabilityRepoFull{exists: map[string]bool{"cap-1": true, "cap-2": true}}
	proc := &stubProcessRepoFull{items: map[string]*process.Process{
		"proc-1": {ID: "proc-1", BusinessServiceID: "bs-1"},
		"proc-x": {ID: "proc-x", BusinessServiceID: "bs-other"},
	}}
	bsc := &stubBSCRepoFull{pairs: map[string]bool{"bs-1\x00cap-1": true}}
	surf := &stubSurfaceRepoFull{items: map[string]*surface.DecisionSurface{
		"surf-1":     {ID: "surf-1", ProcessID: "proc-1"},
		"surf-other": {ID: "surf-other", ProcessID: "proc-x"},
	}}
	return &Service{
		aiSystemRepo:                  sys,
		aiVersionRepo:                 ver,
		aiBindingRepo:                 binding,
		businessServiceRepo:           bs,
		capabilityRepo:                cap,
		processRepo:                   proc,
		businessServiceCapabilityRepo: bsc,
		surfaceRepo:                   surf,
	}
}

func TestPlanAIBinding_CreateHappyPath(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	v := 1
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-ok", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-ok", func(s *types.AISystemBindingSpec) {
		s.AISystemVersion = &v
		s.CapabilityID = "cap-1"
		s.ProcessID = "proc-1"
		s.SurfaceID = "surf-1"
	})
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create, got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
}

func TestPlanAIBinding_InvalidWhenAISystemMissing(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-1", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-1", func(s *types.AISystemBindingSpec) { s.AISystemID = "ai-ghost" })
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (ai_system missing), got %s", entry.Action)
	}
}

func TestPlanAIBinding_InvalidWhenPinnedVersionMissing(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	bad := 99
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-1", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-1", func(s *types.AISystemBindingSpec) { s.AISystemVersion = &bad })
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (pinned version missing), got %s", entry.Action)
	}
}

// Rule 2: (surface, process) consistency.
func TestPlanAIBinding_Invalid_Rule2_SurfaceProcessMismatch(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-1", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-1", func(s *types.AISystemBindingSpec) {
		s.ProcessID = "proc-1"     // proc-1 belongs to bs-1
		s.SurfaceID = "surf-other" // surf-other lives in proc-x, not proc-1
	})
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (rule 2: surface.process_id != binding.process_id), got %s", entry.Action)
	}
	if !errsAnyContain(entry.ValidationErrors, "does not match surface") {
		t.Errorf("expected rule-2 message; got %+v", entry.ValidationErrors)
	}
}

// Rule 3: (process, business_service) consistency.
func TestPlanAIBinding_Invalid_Rule3_ProcessBSMismatch(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-1", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-1", func(s *types.AISystemBindingSpec) {
		s.BusinessServiceID = "bs-other" // proc-1's BS is bs-1, not bs-other
		s.ProcessID = "proc-1"
	})
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (rule 3: process.business_service_id mismatch), got %s", entry.Action)
	}
}

// Rule 4: (business_service, capability) BSC link.
func TestPlanAIBinding_Invalid_Rule4_BSC_NotLinked(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-1", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-1", func(s *types.AISystemBindingSpec) {
		s.CapabilityID = "cap-2" // cap-2 not linked to bs-1 in the BSC stub
	})
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (rule 4: BSC missing), got %s", entry.Action)
	}
}

// Rule 5: (process, capability) transitive via BSC. Trigger when bs is unset.
func TestPlanAIBinding_Invalid_Rule5_TransitiveBSC_NotLinked(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-1", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-1", func(s *types.AISystemBindingSpec) {
		s.BusinessServiceID = "" // omit BS so rule 5 fires
		s.ProcessID = "proc-1"   // proc-1 → bs-1
		s.CapabilityID = "cap-2" // cap-2 not BSC-linked to bs-1
	})
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionInvalid {
		t.Errorf("Action: want Invalid (rule 5: transitive BSC missing), got %s", entry.Action)
	}
}

// Rule 5 happy-path: BSC link via process's BS.
func TestPlanAIBinding_Create_Rule5_TransitiveBSC_Linked(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-r5", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-r5", func(s *types.AISystemBindingSpec) {
		s.BusinessServiceID = "" // rule 5 active
		s.ProcessID = "proc-1"
		s.CapabilityID = "cap-1" // cap-1 IS linked to bs-1 (proc-1's BS)
	})
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create (rule 5 satisfied), got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
}

// ID-collision conflict.
func TestPlanAIBinding_ConflictWhenIDExists(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	stub := svc.aiBindingRepo.(*stubAIBindingRepo)
	stub.items["bind-existing"] = &aisystem.AISystemBinding{ID: "bind-existing"}
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-existing", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-existing", nil)
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionConflict {
		t.Errorf("Action: want Conflict (id exists), got %s", entry.Action)
	}
}

// Bundle-first resolution: the (sysID, version) pair is in the bundle map; no
// repo round-trip should occur and the binding plans Create even though the
// version is not present in the persisted store stub.
func TestPlanAIBinding_BundlePinnedVersion_Resolved(t *testing.T) {
	svc := newAIBindingPlannerSvc()
	v := 99
	entry := ApplyPlanEntry{Kind: types.KindAISystemBinding, ID: "bind-bndl", DocumentIndex: 1}
	doc := makeAIBindingPlannerDoc("bind-bndl", func(s *types.AISystemBindingSpec) {
		s.AISystemVersion = &v
	})
	// Key format matches planAISystemBindingEntry's fmt.Sprintf("%s\x00%d", ...).
	bundleVersionPairs := map[string]struct{}{"ai-1\x0099": {}}
	svc.planAISystemBindingEntry(context.Background(), doc,
		nil, bundleVersionPairs, nil, nil, nil, nil, nil, nil, nil, &entry)
	if entry.Action != ApplyActionCreate {
		t.Errorf("Action: want Create (bundle-resolved version), got %s; errs=%+v", entry.Action, entry.ValidationErrors)
	}
}

// errsAnyContain reports whether any error message contains substr.
func errsAnyContain(errs []types.ValidationError, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Message, substr) {
			return true
		}
	}
	return false
}
