package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// safetyMiddleware composes panic recovery and per-handler timeout around
// the HTTP mux. The middleware is invoked from Server.ServeHTTP so it
// covers every registered route, including ones registered after Server
// construction via With* builders.
//
// Order from outside in: recovery → timeout (or pass-through for /metrics)
// → mux → existing requireAuth / requireRole → handler. A panic anywhere
// inside this chain is recovered and translated into a JSON 500 response,
// regardless of whether it originated in the timeout goroutine or in the
// handler.

// withSafety wraps next with panic recovery and (when timeout > 0 and the
// path is not the metrics path) a per-handler timeout. The metrics path is
// exempt because Prometheus scrapers manage their own timeout and may rely
// on response streaming / flushing semantics that the timeout buffering
// would interfere with.
func (s *Server) withSafety(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /metrics: panic recovery only, no timeout wrapping.
		if s.metricsPath != "" && r.URL.Path == s.metricsPath {
			withPanicRecovery(next).ServeHTTP(w, r)
			return
		}
		// Everything else: recovery + timeout (when configured).
		h := next
		if s.handlerTimeout > 0 {
			h = withTimeout(h, s.handlerTimeout)
		}
		withPanicRecovery(h).ServeHTTP(w, r)
	})
}

// withPanicRecovery converts any panic in next into a structured-log event
// and a JSON 500 response. http.ErrAbortHandler is re-panicked so net/http
// can perform its intended connection-abort behaviour.
func withPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			slog.Error("midas_http_panic_recovered",
				"method", r.Method,
				"path", r.URL.Path,
				"panic", fmt.Sprintf("%v", rec),
				"stack", string(debug.Stack()),
				"remote_addr", r.RemoteAddr,
				"request_id", r.Header.Get("X-Request-ID"),
			)
			// If the handler already started writing a response there is
			// nothing useful we can do here — the outer http server will
			// eventually close the connection. Attempt the JSON write
			// regardless; the http.ResponseWriter contract makes it safe
			// to call WriteHeader/Write even after a partial write (extra
			// headers are dropped by net/http).
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}()
		next.ServeHTTP(w, r)
	})
}

// withTimeout enforces a per-request wall-clock deadline by:
//  1. attaching context.WithTimeout to r.Context() so downstream
//     dependencies (database, orchestrator, audit) can react to
//     cancellation
//  2. running the handler on a goroutine with a buffering response writer
//  3. on timeout, writing 503 + {"error":"request timed out"} to the real
//     writer
//
// Panics inside the handler goroutine are caught and translated into a 500
// response on the buffer, which is then flushed by the main goroutine —
// the outer panic-recovery middleware never sees them, but the structured
// log is emitted in the same format.
func withTimeout(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)

		bw := newBufferingWriter()
		done := make(chan struct{})

		go func() {
			defer close(done)
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					// Re-panic on the goroutine — net/http reads only the
					// outer goroutine, so this won't propagate. Best-effort
					// log and translate into a 500.
					slog.Error("midas_http_panic_recovered",
						"method", r.Method,
						"path", r.URL.Path,
						"panic", "abort handler",
						"remote_addr", r.RemoteAddr,
					)
					bw.reset()
					bw.WriteHeader(http.StatusInternalServerError)
					_, _ = bw.Write([]byte(`{"error":"internal server error"}` + "\n"))
					return
				}
				slog.Error("midas_http_panic_recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
					"remote_addr", r.RemoteAddr,
					"request_id", r.Header.Get("X-Request-ID"),
				)
				bw.reset()
				bw.Header().Set("Content-Type", "application/json; charset=utf-8")
				bw.WriteHeader(http.StatusInternalServerError)
				_, _ = bw.Write([]byte(`{"error":"internal server error"}` + "\n"))
			}()
			next.ServeHTTP(bw, r)
		}()

		select {
		case <-done:
			bw.flushTo(w)
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				slog.Warn("midas_http_timeout",
					"method", r.Method,
					"path", r.URL.Path,
					"timeout_ms", timeout.Milliseconds(),
					"remote_addr", r.RemoteAddr,
					"request_id", r.Header.Get("X-Request-ID"),
				)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"error": "request timed out",
				})
				return
			}
			// Client cancellation: connection is going away anyway. Don't
			// attempt to write a response — the goroutine may still be
			// running and may try to write through the buffering writer.
			// Discard.
		}
	})
}

// bufferingWriter is a minimal http.ResponseWriter implementation used by
// the timeout middleware. It captures status, headers, and body in memory
// so the outer goroutine can decide whether to flush them or write a
// timeout response instead.
//
// Concurrent access is guarded by mu so the timeout-side reset (in the
// panic-recovery defer) is race-free with the handler's writes.
type bufferingWriter struct {
	mu      sync.Mutex
	header  http.Header
	body    bytes.Buffer
	status  int
	written bool
}

func newBufferingWriter() *bufferingWriter {
	return &bufferingWriter{header: http.Header{}}
}

func (b *bufferingWriter) Header() http.Header {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.header
}

func (b *bufferingWriter) WriteHeader(status int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.written {
		return
	}
	b.status = status
	b.written = true
}

func (b *bufferingWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.written {
		b.status = http.StatusOK
		b.written = true
	}
	return b.body.Write(p)
}

// reset clears any captured state so a panic-recovery path can write a
// fresh 500 response over the top of a partial handler write.
func (b *bufferingWriter) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.header = http.Header{}
	b.body.Reset()
	b.status = 0
	b.written = false
}

// flushTo copies the buffered status, headers, and body to dst. Called
// only after the handler goroutine has finished, so no further writes are
// possible and locking is just a defensive guard.
func (b *bufferingWriter) flushTo(dst http.ResponseWriter) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, vs := range b.header {
		for _, v := range vs {
			dst.Header().Add(k, v)
		}
	}
	if b.status != 0 {
		dst.WriteHeader(b.status)
	}
	if b.body.Len() > 0 {
		_, _ = dst.Write(b.body.Bytes())
	}
}
