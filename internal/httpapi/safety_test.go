package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// installPanickyRoute registers a /test/panic handler on the server's mux
// that panics with the given value. Used by panic-recovery tests.
func installPanickyRoute(s *Server, panicValue any) {
	s.mux.HandleFunc("/test/panic", func(w http.ResponseWriter, r *http.Request) {
		panic(panicValue)
	})
}

// installSlowRoute registers a /test/slow handler that blocks until the
// request context is cancelled, then returns. Used by timeout tests.
func installSlowRoute(s *Server) {
	s.mux.HandleFunc("/test/slow", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		// Try to write a response after cancellation — the buffering writer
		// will accept the bytes; the outer goroutine has already written
		// the timeout response, so this is harmless.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("late response"))
	})
}

// installNormalRoute registers a /test/ok handler that returns 200 OK
// immediately. Used to verify the safety wrapper does not break normal
// handlers.
func installNormalRoute(s *Server) {
	s.mux.HandleFunc("/test/ok", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

// ---------------------------------------------------------------------------
// Panic recovery
// ---------------------------------------------------------------------------

func TestSafety_PanicRecovery_Returns500JSON(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(2 * time.Second)
	installPanickyRoute(srv, "boom")

	rec := performRequest(t, srv, http.MethodGet, "/test/panic", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"error"`) {
		t.Errorf("body: want JSON with \"error\" field, got %q", body)
	}
	if !strings.Contains(body, "internal server error") {
		t.Errorf("body: want \"internal server error\", got %q", body)
	}
	if strings.Contains(body, "goroutine") || strings.Contains(body, ".go:") {
		t.Errorf("body must not contain stack trace, got %q", body)
	}
}

func TestSafety_PanicRecovery_NormalRouteUnaffected(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(2 * time.Second)
	installNormalRoute(srv)

	rec := performRequest(t, srv, http.MethodGet, "/test/ok", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("normal route status: want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("normal route body: want ok payload, got %q", rec.Body.String())
	}
}

func TestSafety_PanicRecovery_ServerSurvives(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(2 * time.Second)
	installPanickyRoute(srv, "first boom")
	installNormalRoute(srv)

	// First request panics — should recover.
	rec1 := performRequest(t, srv, http.MethodGet, "/test/panic", nil)
	if rec1.Code != http.StatusInternalServerError {
		t.Fatalf("first request: want 500, got %d", rec1.Code)
	}
	// Second request must still work.
	rec2 := performRequest(t, srv, http.MethodGet, "/test/ok", nil)
	if rec2.Code != http.StatusOK {
		t.Errorf("subsequent request after panic: want 200, got %d", rec2.Code)
	}
}

// ---------------------------------------------------------------------------
// Per-handler timeout
// ---------------------------------------------------------------------------

func TestSafety_Timeout_BlocksReturn503JSON(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(20 * time.Millisecond)
	installSlowRoute(srv)

	start := time.Now()
	rec := performRequest(t, srv, http.MethodGet, "/test/slow", nil)
	elapsed := time.Since(start)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "request timed out") {
		t.Errorf("body: want \"request timed out\", got %q", rec.Body.String())
	}
	if elapsed > 1*time.Second {
		t.Errorf("elapsed: want <1s for 20ms timeout, got %v", elapsed)
	}
}

func TestSafety_Timeout_ContextCancelledForHandler(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(15 * time.Millisecond)
	gotCancellation := make(chan struct{}, 1)
	srv.mux.HandleFunc("/test/observe-cancel", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			gotCancellation <- struct{}{}
		case <-time.After(2 * time.Second):
			// fail-safe: handler should be cancelled before this fires
		}
	})

	_ = performRequest(t, srv, http.MethodGet, "/test/observe-cancel", nil)

	select {
	case <-gotCancellation:
		// good
	case <-time.After(1 * time.Second):
		t.Error("handler did not observe context cancellation within 1s")
	}
}

func TestSafety_Timeout_Disabled_PassesThrough(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}) // no WithHandlerTimeout call → timeout = 0
	srv.mux.HandleFunc("/test/sleep", func(w http.ResponseWriter, r *http.Request) {
		// Sleep briefly with a hard cap; if timeout=0 then no 503 should fire.
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "slept"})
	})

	rec := performRequest(t, srv, http.MethodGet, "/test/sleep", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("status with timeout disabled: want 200, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// /metrics carve-out
// ---------------------------------------------------------------------------

func TestSafety_MetricsRoute_NotTimedOut(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(15 * time.Millisecond)

	// Register a "metrics-like" handler at /metrics that takes ~50ms. With
	// the carve-out it should complete normally; without it, a 15ms
	// timeout would convert it into a 503.
	srv.mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(50 * time.Millisecond):
		case <-r.Context().Done():
		}
		_, _ = w.Write([]byte("# HELP fake metric\n"))
	})
	// Tell the server which path is the metrics path so safety knows to
	// carve it out.
	srv.metricsPath = "/metrics"

	rec := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	if rec.Code == http.StatusServiceUnavailable {
		t.Errorf("/metrics should not be subject to the per-handler timeout, got 503")
	}
	if !strings.Contains(rec.Body.String(), "fake metric") {
		t.Errorf("/metrics body: want fake metric, got %q", rec.Body.String())
	}
}

func TestSafety_MetricsRoute_PanicStillRecovered(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(2 * time.Second)
	srv.mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		panic("metrics handler panic")
	})
	srv.metricsPath = "/metrics"

	rec := performRequest(t, srv, http.MethodGet, "/metrics", nil)
	// Even though /metrics is exempt from timeout, panic recovery still
	// applies — operators should get a 500 JSON, not a closed connection.
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Errorf("body: want internal server error, got %q", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Request cancellation by the client
// ---------------------------------------------------------------------------

func TestSafety_ClientCancellation_NoStrayResponse(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).WithHandlerTimeout(2 * time.Second)
	installSlowRoute(srv)

	// Build a request with a context the test cancels mid-flight. The
	// handler waits on r.Context().Done(), so the cancel propagates and
	// the safety middleware sees ctx.Err() == context.Canceled (not
	// DeadlineExceeded), so it must not write a 503 response.
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/test/slow", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		srv.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good — server returned after cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return after client cancellation within 2s")
	}

	// Body should be empty (we don't write anything on client cancel) or
	// the late response from the handler. Either way, status must NOT be
	// 503 because the cause is client cancellation, not deadline.
	if rec.Code == http.StatusServiceUnavailable {
		t.Errorf("client cancellation should not produce 503, got 503")
	}
}
