package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/accept-io/midas/internal/identity"
)

// recordingAuthenticator is a test double that returns a pre-configured
// (principal, error) tuple and counts how many times it was invoked. It is
// the cheapest way to assert chain short-circuit behaviour without spinning
// up real authenticators.
type recordingAuthenticator struct {
	principal *identity.Principal
	err       error
	calls     int
}

func (a *recordingAuthenticator) Authenticate(*http.Request) (*identity.Principal, error) {
	a.calls++
	return a.principal, a.err
}

func newRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/", nil)
}

// TestChain_NoCredsThenSuccess pins point 1+3: ErrNoCredentials makes the
// chain advance to the next member, and the first success is returned.
func TestChain_NoCredsThenSuccess(t *testing.T) {
	want := &identity.Principal{ID: "user:bob", Provider: "test"}
	first := &recordingAuthenticator{err: ErrNoCredentials}
	second := &recordingAuthenticator{principal: want}
	chain := Chain(first, second)

	got, err := chain.Authenticate(newRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("want principal %p, got %p", want, got)
	}
	if first.calls != 1 {
		t.Errorf("first authenticator: want 1 call, got %d", first.calls)
	}
	if second.calls != 1 {
		t.Errorf("second authenticator: want 1 call, got %d", second.calls)
	}
}

// TestChain_InvalidShortCircuits pins point 4: a non-ErrNoCredentials error
// must stop the chain immediately and must NOT call later members. This is
// the security invariant against confused-deputy fall-through.
func TestChain_InvalidShortCircuits(t *testing.T) {
	invalid := errors.New("auth: unknown token")
	first := &recordingAuthenticator{err: invalid}
	second := &recordingAuthenticator{principal: &identity.Principal{ID: "user:should-not-be-returned"}}
	chain := Chain(first, second)

	got, err := chain.Authenticate(newRequest())
	if err == nil {
		t.Fatal("want error from invalid-credentials short-circuit")
	}
	if !errors.Is(err, invalid) {
		t.Errorf("want returned error to wrap %v, got %v", invalid, err)
	}
	if got != nil {
		t.Errorf("want nil principal on error, got %+v", got)
	}
	if first.calls != 1 {
		t.Errorf("first authenticator: want 1 call, got %d", first.calls)
	}
	if second.calls != 0 {
		t.Errorf("second authenticator must NOT be called when the first returns a non-ErrNoCredentials error; got %d calls", second.calls)
	}
}

// TestChain_Empty pins point 6: an empty chain returns ErrNoCredentials.
func TestChain_Empty(t *testing.T) {
	chain := Chain()

	_, err := chain.Authenticate(newRequest())
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("want ErrNoCredentials, got %v", err)
	}
}

// TestChain_AllNoCreds pins point 5: when every member returns
// ErrNoCredentials, the chain itself returns ErrNoCredentials.
func TestChain_AllNoCreds(t *testing.T) {
	first := &recordingAuthenticator{err: ErrNoCredentials}
	second := &recordingAuthenticator{err: ErrNoCredentials}
	chain := Chain(first, second)

	_, err := chain.Authenticate(newRequest())
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("want ErrNoCredentials, got %v", err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Errorf("both members should be tried; got first=%d second=%d", first.calls, second.calls)
	}
}

// TestChain_FirstSuccessShortCircuits pins point 2: a successful first
// authenticator wins and later members are not consulted (avoids redundant
// session lookup on token-bearing service calls).
func TestChain_FirstSuccessShortCircuits(t *testing.T) {
	first := &recordingAuthenticator{principal: &identity.Principal{ID: "user:first"}}
	second := &recordingAuthenticator{principal: &identity.Principal{ID: "user:second"}}
	chain := Chain(first, second)

	got, err := chain.Authenticate(newRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != "user:first" {
		t.Errorf("want first authenticator's principal user:first, got %+v", got)
	}
	if second.calls != 0 {
		t.Errorf("second authenticator must NOT be called after first success; got %d calls", second.calls)
	}
}

// TestChain_NilEntriesSkipped pins point 7: nil entries are filtered out
// rather than panicking. The chain is operationally identical to one
// constructed without the nil entries.
func TestChain_NilEntriesSkipped(t *testing.T) {
	wantPrincipal := &identity.Principal{ID: "user:c"}
	mid := &recordingAuthenticator{err: ErrNoCredentials}
	last := &recordingAuthenticator{principal: wantPrincipal}
	// nil first, then a real ErrNoCredentials, then a real success.
	chain := Chain(nil, mid, last)

	got, err := chain.Authenticate(newRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantPrincipal {
		t.Errorf("want principal %p, got %p", wantPrincipal, got)
	}
	if mid.calls != 1 || last.calls != 1 {
		t.Errorf("both real members should be tried; mid=%d last=%d", mid.calls, last.calls)
	}
}

// TestChain_NilEntriesAroundNoCreds pins the corner case where nil entries
// are interleaved with real ErrNoCredentials returns. The result is still
// ErrNoCredentials — nils contribute nothing, do not panic, and do not
// short-circuit.
func TestChain_NilEntriesAroundNoCreds(t *testing.T) {
	first := &recordingAuthenticator{err: ErrNoCredentials}
	last := &recordingAuthenticator{err: ErrNoCredentials}
	chain := Chain(first, nil, last)

	_, err := chain.Authenticate(newRequest())
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("want ErrNoCredentials, got %v", err)
	}
	if first.calls != 1 || last.calls != 1 {
		t.Errorf("both real members should be tried; first=%d last=%d", first.calls, last.calls)
	}
}

// TestChain_OnlyNilsBehavesAsEmpty pins the all-nil corner case: the chain
// is operationally identical to Chain() and returns ErrNoCredentials.
func TestChain_OnlyNilsBehavesAsEmpty(t *testing.T) {
	chain := Chain(nil, nil, nil)

	_, err := chain.Authenticate(newRequest())
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("want ErrNoCredentials, got %v", err)
	}
}
