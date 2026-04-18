package httpapi

// Test helpers specific to the admin-audit emission tests (Issue #41).
// Keep narrow — these just wrap request construction with headers that the
// emission path depends on.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/adminaudit"
	"github.com/accept-io/midas/internal/localiam"
)

func mustYAMLRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/yaml")
	return req
}

func mustJSONRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func performWithRequest(t *testing.T, srv *Server, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// newLocalIAMServiceForTest constructs a local IAM service wired with the
// admin-audit repository. The newIAMServer helper in localiam_handler_test.go
// calls Bootstrap before returning, which defeats tests that need to observe
// the bootstrap record — so we avoid using it.
func newLocalIAMServiceForTest(users localiam.UserRepository, sessions localiam.SessionRepository, adminAudit adminaudit.Repository) *localiam.Service {
	svc := localiam.NewService(users, sessions, localiam.Config{SessionTTL: time.Hour})
	if adminAudit != nil {
		svc.WithAdminAudit(adminAudit)
	}
	return svc
}

// ensure context import isn't marked unused if the helpers shift.
var _ = context.Background
