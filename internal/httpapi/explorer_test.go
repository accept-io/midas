package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/config"
)

func TestExplorer_Disabled_Returns404(t *testing.T) {
	// Server constructed without WithExplorerEnabled — routes not registered.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

func TestExplorer_Enabled_ReturnsHTML(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("want Content-Type text/html, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
}

func TestExplorer_Config_ReturnsJSON(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeRequired).
		WithPolicyMeta("noop", "NoOpPolicyEvaluator").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if got, ok := resp["running"].(bool); !ok || !got {
		t.Errorf("want running=true, got %v", resp["running"])
	}
	if resp["authMode"] != "required" {
		t.Errorf("want authMode=required, got %v", resp["authMode"])
	}
	if resp["policyMode"] != "noop" {
		t.Errorf("want policyMode=noop, got %v", resp["policyMode"])
	}
}

func TestExplorer_Config_MethodNotAllowed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodPost, "/explorer/config", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestExplorer_Assets_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// /explorer/ routes to handleExplorerAssets; the FileServer finds index.html
	// in the explorer/ directory and serves it. (Requesting /explorer/index.html
	// directly triggers a FileServer redirect to the directory URL, which is
	// standard Go http.FileServer behaviour for index files.)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /explorer/, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want body to contain 'MIDAS Explorer'")
	}
}
