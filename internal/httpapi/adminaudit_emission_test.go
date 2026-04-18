package httpapi

// Emission tests for platform administrative audit (Issue #41). Each in-scope
// emission point is covered by a focused test that proves: (a) a record is
// written; (b) the action type is correct; (c) actor and actor_type are
// correct; (d) outcome is correct; (e) request-level context is present when
// applicable; (f) no secret material is written.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/adminaudit"
	cpTypes "github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/inference"
	"github.com/accept-io/midas/internal/store/memory"
)

// assertNoSecretMaterial verifies that no record fingerprint contains
// password-like strings. This is a blanket safety guard against accidental
// leakage through any field.
func assertNoSecretMaterial(t *testing.T, rec *adminaudit.AdminAuditRecord) {
	t.Helper()
	b, _ := json.Marshal(rec)
	haystack := strings.ToLower(string(b))
	for _, needle := range []string{
		"\"password\"",
		"password_hash",
		"passwordhash",
		"bcrypt",
		"$2a$",
		"$2b$",
		"$2y$",
	} {
		if strings.Contains(haystack, needle) {
			t.Errorf("admin-audit record contains potential secret material %q: %s", needle, string(b))
		}
	}
}

// ---------------------------------------------------------------------------
// Apply invocation
// ---------------------------------------------------------------------------

func TestAdminAudit_ApplyInvocation_EmitsRecord(t *testing.T) {
	repo := memory.NewAdminAuditRepo()

	result := &cpTypes.ApplyResult{}
	result.AddCreated("Surface", "surf-1")
	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return result, nil
		},
	}

	srv := NewServerFull(&mockOrchestrator{}, mockCP, nil, nil, nil, nil).WithAdminAudit(repo)

	body := []byte("kind: Surface\n")
	req := mustYAMLRequest(t, http.MethodPost, "/v1/controlplane/apply", body)
	req.Header.Set("X-MIDAS-ACTOR", "alice")
	req.Header.Set("X-Request-Id", "req-abc")
	req.RemoteAddr = "192.0.2.10:54321"
	rec := performWithRequest(t, srv, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	records, err := repo.List(context.Background(), adminaudit.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly 1 admin-audit record (request-level, not per-resource), got %d", len(records))
	}
	r := records[0]
	if r.Action != adminaudit.ActionApplyInvoked {
		t.Errorf("action = %q, want %q", r.Action, adminaudit.ActionApplyInvoked)
	}
	if r.Outcome != adminaudit.OutcomeSuccess {
		t.Errorf("outcome = %q, want success", r.Outcome)
	}
	if r.ActorID != "alice" {
		t.Errorf("actor_id = %q, want alice", r.ActorID)
	}
	if r.ActorType != adminaudit.ActorTypeUser {
		t.Errorf("actor_type = %q, want user", r.ActorType)
	}
	if r.TargetType != adminaudit.TargetTypeBundle {
		t.Errorf("target_type = %q, want bundle", r.TargetType)
	}
	if r.RequestID != "req-abc" {
		t.Errorf("request_id = %q, want req-abc", r.RequestID)
	}
	if r.ClientIP == "" {
		t.Error("client_ip unexpectedly empty")
	}
	if r.RequiredPermission != "controlplane:apply" {
		t.Errorf("required_permission = %q, want controlplane:apply", r.RequiredPermission)
	}
	if r.Details == nil || r.Details.BundleBytes != len(body) || r.Details.CreatedCount != 1 {
		t.Errorf("details wrong: got %+v", r.Details)
	}
	assertNoSecretMaterial(t, r)
}

func TestAdminAudit_ApplyInvocation_FailureRecordsOutcomeFailure(t *testing.T) {
	repo := memory.NewAdminAuditRepo()

	mockCP := &mockControlPlane{
		applyBundleFn: func(_ context.Context, _ []byte, _ string) (*cpTypes.ApplyResult, error) {
			return nil, errors.New("boom")
		},
	}
	srv := NewServerFull(&mockOrchestrator{}, mockCP, nil, nil, nil, nil).WithAdminAudit(repo)

	req := mustYAMLRequest(t, http.MethodPost, "/v1/controlplane/apply", []byte("kind: Surface\n"))
	req.Header.Set("X-MIDAS-ACTOR", "alice")
	req.RemoteAddr = "192.0.2.10:54321"
	performWithRequest(t, srv, req)

	records, _ := repo.List(context.Background(), adminaudit.ListFilter{})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Outcome != adminaudit.OutcomeFailure {
		t.Errorf("outcome = %q, want failure", records[0].Outcome)
	}
	if records[0].Details == nil || records[0].Details.Error == "" {
		t.Error("expected failure details.error to be populated")
	}
}

// ---------------------------------------------------------------------------
// Promote
// ---------------------------------------------------------------------------

func TestAdminAudit_Promote_EmitsRecord(t *testing.T) {
	repo := memory.NewAdminAuditRepo()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithPromotion(successMock(2)).
		WithAdminAudit(repo)

	req := mustJSONRequest(t, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)
	req.Header.Set("X-MIDAS-ACTOR", "operator-1")
	req.Header.Set("X-Request-Id", "req-prom")
	rec := performWithRequest(t, srv, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	records, _ := repo.List(context.Background(), adminaudit.ListFilter{})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.Action != adminaudit.ActionPromoteExecuted {
		t.Errorf("action = %q", r.Action)
	}
	if r.Outcome != adminaudit.OutcomeSuccess {
		t.Errorf("outcome = %q", r.Outcome)
	}
	if r.ActorID != "operator-1" {
		t.Errorf("actor_id = %q", r.ActorID)
	}
	if r.TargetType != adminaudit.TargetTypeProcess {
		t.Errorf("target_type = %q", r.TargetType)
	}
	if r.RequiredPermission != "controlplane:promote" {
		t.Errorf("required_permission = %q", r.RequiredPermission)
	}
	if r.Details == nil || r.Details.SurfacesMigrated != 2 {
		t.Errorf("details wrong: %+v", r.Details)
	}
	assertNoSecretMaterial(t, r)
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

type fixedCleanupSvc struct {
	result inference.CleanupResult
	err    error
}

func (f *fixedCleanupSvc) CleanupInferredEntities(_ context.Context, _ time.Time) (inference.CleanupResult, error) {
	return f.result, f.err
}

func TestAdminAudit_Cleanup_EmitsRecord(t *testing.T) {
	repo := memory.NewAdminAuditRepo()
	svc := &fixedCleanupSvc{result: inference.CleanupResult{
		ProcessesDeleted:    []string{"auto:p1"},
		CapabilitiesDeleted: []string{"auto:c1"},
	}}
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithCleanup(svc).
		WithAdminAudit(repo)

	req := mustJSONRequest(t, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":7}`))
	req.Header.Set("X-MIDAS-ACTOR", "ops-1")
	rec := performWithRequest(t, srv, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	records, _ := repo.List(context.Background(), adminaudit.ListFilter{})
	if len(records) != 1 {
		t.Fatalf("expected 1, got %d", len(records))
	}
	r := records[0]
	if r.Action != adminaudit.ActionCleanupExecuted {
		t.Errorf("action = %q", r.Action)
	}
	if r.Outcome != adminaudit.OutcomeSuccess {
		t.Errorf("outcome = %q", r.Outcome)
	}
	if r.TargetType != adminaudit.TargetTypePlatform {
		t.Errorf("target_type = %q", r.TargetType)
	}
	if r.RequiredPermission != "controlplane:cleanup" {
		t.Errorf("required_permission = %q", r.RequiredPermission)
	}
	if r.Details == nil || r.Details.OlderThanDays != 7 {
		t.Errorf("details wrong: %+v", r.Details)
	}
	if len(r.Details.ProcessesDeleted) != 1 || len(r.Details.CapabilitiesDeleted) != 1 {
		t.Errorf("deletion lists wrong: %+v", r.Details)
	}
	assertNoSecretMaterial(t, r)
}

// ---------------------------------------------------------------------------
// Password change
// ---------------------------------------------------------------------------

func TestAdminAudit_PasswordChange_EmitsRecord_NoSecrets(t *testing.T) {
	repo := memory.NewAdminAuditRepo()
	srv, _ := newIAMServer(t)
	srv.WithAdminAudit(repo)

	// Log in as bootstrap admin.
	loginRec := doLogin(t, srv, "admin", "admin")
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}
	cookie := sessionCookie(loginRec)
	if cookie == "" {
		t.Fatal("no session cookie after login")
	}

	body := marshalJSON(t, map[string]string{
		"current_password": "admin",
		"new_password":     "a-new-secure-passphrase-1234",
	})
	rec := requestWithCookie(t, srv, http.MethodPost, "/auth/change-password", body, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	records, _ := repo.List(context.Background(), adminaudit.ListFilter{
		Action: adminaudit.ActionPasswordChanged,
	})
	if len(records) != 1 {
		t.Fatalf("expected 1 password-change record, got %d", len(records))
	}
	r := records[0]
	if r.Outcome != adminaudit.OutcomeSuccess {
		t.Errorf("outcome = %q", r.Outcome)
	}
	if r.ActorType != adminaudit.ActorTypeUser {
		t.Errorf("actor_type = %q", r.ActorType)
	}
	if r.TargetType != adminaudit.TargetTypeUser || r.TargetID == "" {
		t.Errorf("target wrong: %+v", r)
	}
	// Safety: no password material should be anywhere in the record.
	assertNoSecretMaterial(t, r)
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "a-new-secure-passphrase-1234") {
		t.Errorf("new password leaked into record: %s", string(b))
	}
	if strings.Contains(string(b), "admin") && strings.Contains(string(b), "current_password") {
		t.Errorf("current password leaked into record: %s", string(b))
	}
}

// ---------------------------------------------------------------------------
// Bootstrap admin creation
// ---------------------------------------------------------------------------

func TestAdminAudit_BootstrapAdminCreation_EmitsRecord(t *testing.T) {
	// We exercise bootstrap directly via the IAM server helper, but attach
	// the admin-audit repo to the iam service BEFORE bootstrap runs.
	// newIAMServer already calls Bootstrap — we need to run it ourselves
	// with the audit repo in place first.
	repo := memory.NewAdminAuditRepo()

	users := newStubUserRepo()
	sessions := newStubSessionRepo()
	svc := newLocalIAMServiceForTest(users, sessions, repo)
	if err := svc.Bootstrap(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	records, _ := repo.List(context.Background(), adminaudit.ListFilter{
		Action: adminaudit.ActionBootstrapAdminCreated,
	})
	if len(records) != 1 {
		t.Fatalf("expected 1 bootstrap record, got %d", len(records))
	}
	r := records[0]
	if r.ActorType != adminaudit.ActorTypeSystem {
		t.Errorf("actor_type = %q, want system", r.ActorType)
	}
	if r.ActorID != "system:bootstrap" {
		t.Errorf("actor_id = %q, want system:bootstrap", r.ActorID)
	}
	if r.TargetType != adminaudit.TargetTypeUser || r.TargetID == "" {
		t.Errorf("target wrong: %+v", r)
	}
	if r.Outcome != adminaudit.OutcomeSuccess {
		t.Errorf("outcome = %q", r.Outcome)
	}
	assertNoSecretMaterial(t, r)
	b, _ := json.Marshal(r)
	if strings.Contains(strings.ToLower(string(b)), "admin\"") && strings.Contains(strings.ToLower(string(b)), "\"password\"") {
		t.Errorf("default password may have leaked: %s", string(b))
	}
}

// ---------------------------------------------------------------------------
// Idempotent bootstrap does not emit a second record
// ---------------------------------------------------------------------------

func TestAdminAudit_BootstrapIdempotent_EmitsOnlyOnce(t *testing.T) {
	repo := memory.NewAdminAuditRepo()
	users := newStubUserRepo()
	sessions := newStubSessionRepo()
	svc := newLocalIAMServiceForTest(users, sessions, repo)

	for i := 0; i < 3; i++ {
		if err := svc.Bootstrap(context.Background()); err != nil {
			t.Fatalf("bootstrap[%d]: %v", i, err)
		}
	}

	records, _ := repo.List(context.Background(), adminaudit.ListFilter{
		Action: adminaudit.ActionBootstrapAdminCreated,
	})
	if len(records) != 1 {
		t.Errorf("expected 1 bootstrap record across 3 calls, got %d", len(records))
	}
}
