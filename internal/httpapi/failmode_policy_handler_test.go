package httpapi

// failmode_policy_handler_test.go — D29d Part A. Tests the three GET
// routes under /v1/fail_mode_policies/{id}/... plus a source-pin
// runtime-inertness test asserting the handler file does not call any
// mutating or lifecycle method on the underlying repository.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/failmode"
	"github.com/accept-io/midas/internal/store/memory"
)

// newFailModePolicyHandlerServer wires a Server with a
// FailModePolicyReadService backed by the in-memory repo. Seed entries
// are inserted via the repo's Create before the server is returned.
// Passing a nil slice yields a server with an empty (but configured)
// reader; pass an empty service to test the 501 path separately.
func newFailModePolicyHandlerServer(t *testing.T, seeds []*failmode.FailModePolicy) *Server {
	t.Helper()
	repo := memory.NewFailModePolicyRepo()
	for _, p := range seeds {
		if err := repo.Create(context.Background(), p); err != nil {
			t.Fatalf("seed FailModePolicy %q v%d: %v", p.ID, p.Version, err)
		}
	}
	svc := NewFailModePolicyReadService(repo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithFailModePolicyReadService(svc)
	return srv
}

func makeTestFailModePolicy(id string, version int, status failmode.FailModePolicyStatus) *failmode.FailModePolicy {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           "Fail Mode " + id,
		Status:         status,
		EffectiveDate:  now,
		BusinessOwner:  "owner",
		TechnicalOwner: "owner",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed, EnforcementState: failmode.EnforcementStateEvidenceOnly, Outcome: failmode.OutcomeEscalate},
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "alice",
	}
}

// ---------------------------------------------------------------------
// GET /v1/fail_mode_policies/{id}
// ---------------------------------------------------------------------

func TestFailModePolicyHandler_GetPolicy_HappyPath(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/fmp-x", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp failModePolicyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "fmp-x" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Status != string(failmode.FailModePolicyStatusActive) {
		t.Errorf("Status = %q", resp.Status)
	}
	if len(resp.Rules) != 5 {
		t.Errorf("Rules len = %d, want 5", len(resp.Rules))
	}
	// D29b axis fields surface in the wire shape.
	for _, r := range resp.Rules {
		if r.CorrectnessClass == "" || r.PermittedMode == "" || r.EnforcementState == "" || r.Outcome == "" {
			t.Errorf("rule missing axis fields: %+v", r)
		}
	}
}

// FindByID returns the highest-version row when multiple exist.
func TestFailModePolicyHandler_GetPolicy_ReturnsLatestVersion(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusDeprecated),
		makeTestFailModePolicy("fmp-x", 2, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/fmp-x", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp failModePolicyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Version != 2 {
		t.Errorf("Version = %d, want 2", resp.Version)
	}
}

func TestFailModePolicyHandler_GetPolicy_NotFound(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/missing", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestFailModePolicyHandler_GetPolicy_NoReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/x", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501; body=%s", rec.Code, rec.Body.String())
	}
}

// Constructing the service with a nil reader keeps HasPolicies() false
// and yields 501 just like when the service itself is unwired.
func TestFailModePolicyHandler_GetPolicy_NilReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithFailModePolicyReadService(NewFailModePolicyReadService(nil))
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/x", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestFailModePolicyHandler_MethodNotAllowed(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, nil)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, m, "/v1/fail_mode_policies/x", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want 405", m, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodGet {
			t.Errorf("%s: Allow header = %q, want %q", m, got, http.MethodGet)
		}
	}
}

func TestFailModePolicyHandler_EmptyIDDispatch_Returns404(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestFailModePolicyHandler_UnknownSubpath_Returns404(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/fmp-x/unknown", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------
// GET /v1/fail_mode_policies/{id}/versions
// ---------------------------------------------------------------------

func TestFailModePolicyHandler_ListVersions_HappyPath(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusDeprecated),
		makeTestFailModePolicy("fmp-x", 2, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/fmp-x/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp failModePolicyListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.FailModePolicies) != 2 {
		t.Fatalf("FailModePolicies len = %d, want 2", len(resp.FailModePolicies))
	}
	// Memory repo's ListVersions returns descending by Version.
	if resp.FailModePolicies[0].Version != 2 || resp.FailModePolicies[1].Version != 1 {
		t.Errorf("versions order = [%d, %d], want [2, 1]",
			resp.FailModePolicies[0].Version, resp.FailModePolicies[1].Version)
	}
}

func TestFailModePolicyHandler_ListVersions_UnknownID_Returns404(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/missing/versions", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestFailModePolicyHandler_ListVersions_NoReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/x/versions", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

// ---------------------------------------------------------------------
// GET /v1/fail_mode_policies/{id}/versions/{version}
// ---------------------------------------------------------------------

func TestFailModePolicyHandler_GetVersion_HappyPath(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusDeprecated),
		makeTestFailModePolicy("fmp-x", 2, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/fmp-x/versions/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp failModePolicyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Version != 1 || resp.Status != string(failmode.FailModePolicyStatusDeprecated) {
		t.Errorf("version=%d status=%q, want 1 / deprecated", resp.Version, resp.Status)
	}
}

func TestFailModePolicyHandler_GetVersion_NotFound(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusActive),
	})
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/fmp-x/versions/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestFailModePolicyHandler_GetVersion_BadVersion(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusActive),
	})
	// Empty version segment is intentionally not tested here: a path
	// ending in "/versions/" collapses to ["fmp-x","versions"] after
	// trim+split and routes to the list-versions handler — see
	// TestFailModePolicyHandler_ListVersions_HappyPath for that path.
	for _, v := range []string{"abc", "0", "-1"} {
		path := "/v1/fail_mode_policies/fmp-x/versions/" + v
		rec := performRequest(t, srv, http.MethodGet, path, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("version=%q: status = %d, want 400", v, rec.Code)
		}
	}
}

func TestFailModePolicyHandler_GetVersion_NoReader_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/fail_mode_policies/x/versions/1", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

// ---------------------------------------------------------------------
// Coexistence with the existing mutate handler at
// /v1/controlplane/fail_mode_policies/. Go's ServeMux uses
// longest-prefix matching, so a request to a controlplane sub-path
// must not be routed into the read dispatcher and vice-versa.
// ---------------------------------------------------------------------

func TestFailModePolicyHandler_DoesNotShadowControlplane(t *testing.T) {
	srv := newFailModePolicyHandlerServer(t, []*failmode.FailModePolicy{
		makeTestFailModePolicy("fmp-x", 1, failmode.FailModePolicyStatusActive),
	})
	// A POST to the controlplane prefix must NOT land in our GET-only
	// read dispatcher (which would produce 405). It must reach the
	// controlplane handler (without an approval service it returns
	// 501, which is the configured behaviour for unwired test
	// servers — what matters is that it is not 405).
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/fail_mode_policies/fmp-x/approve", []byte(`{"version":1,"approved_by":"u"}`))
	if rec.Code == http.StatusMethodNotAllowed {
		t.Fatalf("controlplane POST was shadowed by read dispatcher (got 405); body=%s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// Source-pin runtime-inertness. The file backing the read API must
// not reach any mutating or lifecycle method on the failmode or
// approval surface — every persisted FailModePolicy lifecycle
// transition lives in failmode_approval_handler.go behind
// /v1/controlplane/fail_mode_policies/.
// ---------------------------------------------------------------------

func TestFailModePolicyHandler_SourceIsRuntimeInert(t *testing.T) {
	src, err := os.ReadFile("failmode_policy_handler.go")
	if err != nil {
		t.Fatalf("read handler source: %v", err)
	}
	body := string(src)
	// Mutating method substrings the read handler must not call. We
	// match a leading dot or whitespace + identifier to avoid false
	// positives on words that appear in comments documenting that
	// these calls are absent.
	forbidden := []string{
		".Create(", ".Update(", ".Delete(",
		".Approve(", ".Deprecate(", ".Submit(", ".Reject(", ".Retire(",
		".Save(", ".Insert(",
		"ApproveFailModePolicy", "DeprecateFailModePolicy",
	}
	for _, s := range forbidden {
		if strings.Contains(body, s) {
			t.Errorf("handler source contains forbidden mutating substring %q; "+
				"D29d read API must remain runtime-inert. Move mutating logic to "+
				"failmode_approval_handler.go.", s)
		}
	}
	// Positive pin: the file must invoke the three read methods.
	for _, s := range []string{"FindByID(", "FindByIDAndVersion(", "ListVersions("} {
		if !strings.Contains(body, s) {
			t.Errorf("handler source missing expected read method substring %q", s)
		}
	}
}
