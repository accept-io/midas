package main

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/quickstart"
)

// ---------------------------------------------------------------------------
// Helpers — stub apply and capExists functions for executeQuickstart tests.
// ---------------------------------------------------------------------------

// applyRecorder records the actor argument passed to a stubbed
// applyBundleFunc. The result it returns is constructed once at
// construction time. Calls counts ApplyBundle invocations; tests assert
// on this for re-run-refusal coverage.
type applyRecorder struct {
	actorSeen string
	calls     int
	result    *types.ApplyResult
	err       error
}

func (a *applyRecorder) apply(_ context.Context, _ []byte, actor string) (*types.ApplyResult, error) {
	a.calls++
	a.actorSeen = actor
	if a.err != nil {
		return nil, a.err
	}
	return a.result, nil
}

// fixedExists returns a capExists stub that always returns the same value.
func fixedExists(present bool) capabilityExistsFunc {
	return func(_ context.Context, _ string) (bool, error) {
		return present, nil
	}
}

// erroringExists returns a capExists stub that always returns the given error.
func erroringExists(err error) capabilityExistsFunc {
	return func(_ context.Context, _ string) (bool, error) {
		return false, err
	}
}

// successApplyResult constructs an ApplyResult shaped like a clean
// quickstart apply: 21 created entries (2 BS, 4 Cap, 5 BSC, 4 Proc, 6 Surf).
// Surface IDs match the bundle's actual IDs so tests can assert on them.
func successApplyResult() *types.ApplyResult {
	r := &types.ApplyResult{}
	r.AddCreated(types.KindBusinessService, "bs-consumer-lending")
	r.AddCreated(types.KindBusinessService, "bs-merchant-services")
	r.AddCreated(types.KindCapability, "cap-identity-verification")
	r.AddCreated(types.KindCapability, "cap-credit-scoring")
	r.AddCreated(types.KindCapability, "cap-fraud-detection")
	r.AddCreated(types.KindCapability, "cap-payment-authorization")
	r.AddCreated(types.KindBusinessServiceCapability, "bsc-consumer-lending-identity-verification")
	r.AddCreated(types.KindBusinessServiceCapability, "bsc-consumer-lending-credit-scoring")
	r.AddCreated(types.KindBusinessServiceCapability, "bsc-consumer-lending-fraud-detection")
	r.AddCreated(types.KindBusinessServiceCapability, "bsc-merchant-services-fraud-detection")
	r.AddCreated(types.KindBusinessServiceCapability, "bsc-merchant-services-payment-authorization")
	r.AddCreated(types.KindProcess, "proc-consumer-onboarding")
	r.AddCreated(types.KindProcess, "proc-credit-assessment")
	r.AddCreated(types.KindProcess, "proc-merchant-risk-screen")
	r.AddCreated(types.KindProcess, "proc-merchant-payment-auth")
	r.AddCreated(types.KindSurface, "surf-v2-id-verify")
	r.AddCreated(types.KindSurface, "surf-v2-consumer-fraud")
	r.AddCreated(types.KindSurface, "surf-v2-credit-assess")
	r.AddCreated(types.KindSurface, "surf-v2-merchant-risk")
	r.AddCreated(types.KindSurface, "surf-v2-merchant-payment")
	r.AddCreated(types.KindSurface, "surf-v2-merchant-hv-pay")
	return r
}

// ---------------------------------------------------------------------------
// Flag-parsing tests
// ---------------------------------------------------------------------------

// TestInitQuickstart_DefaultActorString asserts that without --actor
// the parsed actor is the package-level default constant.
func TestInitQuickstart_DefaultActorString(t *testing.T) {
	actor, helpRequested, err := parseInitQuickstartFlags(nil)
	if err != nil {
		t.Fatalf("parseInitQuickstartFlags(nil): %v", err)
	}
	if helpRequested {
		t.Fatal("help should not be requested by empty args")
	}
	if actor != defaultActor {
		t.Errorf("actor: want %q, got %q", defaultActor, actor)
	}
	if actor != "cli:init-quickstart" {
		t.Errorf("default actor literal drifted: want %q, got %q", "cli:init-quickstart", actor)
	}
}

// TestInitQuickstart_RejectsEmptyActor asserts that an explicit empty or
// whitespace-only --actor value is rejected. Falling back silently to a
// system fallback inside ApplyBundle would defeat the audit attribution
// the command exists to produce.
func TestInitQuickstart_RejectsEmptyActor(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "empty", args: []string{"--actor", ""}},
		{name: "whitespace", args: []string{"--actor", "   "}},
		{name: "tab", args: []string{"--actor", "\t"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseInitQuickstartFlags(tc.args)
			if err == nil {
				t.Fatal("want error: empty/whitespace --actor must be rejected")
			}
			if !strings.Contains(err.Error(), "--actor must not be empty") {
				t.Errorf("error did not mention empty actor: %v", err)
			}
		})
	}
}

// TestInitQuickstart_CustomActorString asserts --actor overrides the default.
func TestInitQuickstart_CustomActorString(t *testing.T) {
	actor, _, err := parseInitQuickstartFlags([]string{"--actor", "ops:operator@example.com"})
	if err != nil {
		t.Fatalf("parseInitQuickstartFlags: %v", err)
	}
	if actor != "ops:operator@example.com" {
		t.Errorf("actor: want %q, got %q", "ops:operator@example.com", actor)
	}
}

// TestInitQuickstart_ActorPassesThroughToApply asserts the actor returned
// by flag parsing is exactly the actor passed to applyBundle.
func TestInitQuickstart_ActorPassesThroughToApply(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default", args: nil, want: "cli:init-quickstart"},
		{name: "custom", args: []string{"--actor", "ops:alice"}, want: "ops:alice"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actor, _, err := parseInitQuickstartFlags(tc.args)
			if err != nil {
				t.Fatalf("parseInitQuickstartFlags: %v", err)
			}

			rec := &applyRecorder{result: successApplyResult()}
			var buf bytes.Buffer
			if err := executeQuickstart(
				context.Background(),
				&buf,
				rec.apply,
				fixedExists(false),
				quickstart.Bundle(),
				actor,
				quickstart.AnchorCapabilityID,
			); err != nil {
				t.Fatalf("executeQuickstart: %v", err)
			}
			if rec.actorSeen != tc.want {
				t.Errorf("actor passed to apply: want %q, got %q", tc.want, rec.actorSeen)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Memory-rejection
// ---------------------------------------------------------------------------

// TestInitQuickstart_RejectMemoryBackend asserts the documented memory
// backend rejection fires before any apply attempt.
func TestInitQuickstart_RejectMemoryBackend(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Store.Backend = "memory"

	var buf bytes.Buffer
	err := runQuickstartWithConfig(context.Background(), cfg, "cli:init-quickstart", &buf)
	if err == nil {
		t.Fatal("want error: memory backend not supported")
	}
	if !strings.Contains(err.Error(), "store.backend=memory is not supported") {
		t.Errorf("error message did not match documented text: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("memory rejection should print nothing to stdout, got %d bytes: %q",
			buf.Len(), buf.String())
	}
}

// ---------------------------------------------------------------------------
// Re-run preflight
// ---------------------------------------------------------------------------

// TestInitQuickstart_RefusesReRun asserts that when the preflight anchor
// is already present the command fails before invoking apply, naming the
// anchor capability ID in the error.
func TestInitQuickstart_RefusesReRun(t *testing.T) {
	rec := &applyRecorder{result: successApplyResult()}
	var buf bytes.Buffer
	err := executeQuickstart(
		context.Background(),
		&buf,
		rec.apply,
		fixedExists(true),
		quickstart.Bundle(),
		"cli:init-quickstart",
		quickstart.AnchorCapabilityID,
	)
	if err == nil {
		t.Fatal("want error: bundle already applied")
	}
	if !strings.Contains(err.Error(), "bundle already applied") {
		t.Errorf("error did not mention 'bundle already applied': %v", err)
	}
	if !strings.Contains(err.Error(), quickstart.AnchorCapabilityID) {
		t.Errorf("error did not name anchor capability %q: %v",
			quickstart.AnchorCapabilityID, err)
	}
	if rec.calls != 0 {
		t.Errorf("apply must NOT be invoked when preflight refuses; got %d call(s)", rec.calls)
	}
	if buf.Len() != 0 {
		t.Errorf("re-run rejection should print nothing to stdout, got: %q", buf.String())
	}
}

// TestInitQuickstart_PreflightErrorPropagates asserts that an error from
// the preflight check is reported and apply is not invoked.
func TestInitQuickstart_PreflightErrorPropagates(t *testing.T) {
	rec := &applyRecorder{result: successApplyResult()}
	preflightErr := errors.New("simulated repo failure")
	var buf bytes.Buffer
	err := executeQuickstart(
		context.Background(),
		&buf,
		rec.apply,
		erroringExists(preflightErr),
		quickstart.Bundle(),
		"cli:init-quickstart",
		quickstart.AnchorCapabilityID,
	)
	if err == nil {
		t.Fatal("want error: preflight failure must propagate")
	}
	if !errors.Is(err, preflightErr) {
		t.Errorf("returned error must wrap preflight error: %v", err)
	}
	if rec.calls != 0 {
		t.Errorf("apply must NOT be invoked when preflight errors; got %d call(s)", rec.calls)
	}
}

// ---------------------------------------------------------------------------
// Output content
// ---------------------------------------------------------------------------

// TestInitQuickstart_OutputContainsCreatedSurfaceIDs asserts that the
// success output names every created Surface ID, sorted lexicographically.
func TestInitQuickstart_OutputContainsCreatedSurfaceIDs(t *testing.T) {
	rec := &applyRecorder{result: successApplyResult()}
	var buf bytes.Buffer
	if err := executeQuickstart(
		context.Background(),
		&buf,
		rec.apply,
		fixedExists(false),
		quickstart.Bundle(),
		"cli:init-quickstart",
		quickstart.AnchorCapabilityID,
	); err != nil {
		t.Fatalf("executeQuickstart: %v", err)
	}

	out := buf.String()
	expected := []string{
		"surf-v2-consumer-fraud",
		"surf-v2-credit-assess",
		"surf-v2-id-verify",
		"surf-v2-merchant-hv-pay",
		"surf-v2-merchant-payment",
		"surf-v2-merchant-risk",
	}
	for _, id := range expected {
		if !strings.Contains(out, id) {
			t.Errorf("output missing Surface ID %q. Full output:\n%s", id, out)
		}
	}

	// Lexicographic sort assertion: walk the output and capture the
	// order in which surface IDs appear; compare against sort.Strings.
	var seen []string
	for _, id := range expected {
		idx := strings.Index(out, id)
		if idx < 0 {
			continue
		}
		seen = append(seen, id)
	}
	// Above append order matches `expected` order, not output order;
	// re-derive by index in the buffer.
	type pos struct {
		id  string
		idx int
	}
	positions := make([]pos, 0, len(expected))
	for _, id := range expected {
		positions = append(positions, pos{id: id, idx: strings.Index(out, id)})
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].idx < positions[j].idx })
	got := make([]string, len(positions))
	for i, p := range positions {
		got[i] = p.id
	}
	want := append([]string(nil), expected...)
	sort.Strings(want)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Surface ID order in output: want %v, got %v", want, got)
			break
		}
	}

	// Output must reference the surface-approval endpoint.
	if !strings.Contains(out, "/v1/controlplane/surfaces/") {
		t.Errorf("output missing surface-approval endpoint reference; output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

// TestInitQuickstart_HelpTextMentionsApprovalStep asserts the help text
// instructs the operator about the surface-approval step.
func TestInitQuickstart_HelpTextMentionsApprovalStep(t *testing.T) {
	var buf bytes.Buffer
	printInitQuickstartHelp(&buf)
	out := buf.String()

	if !strings.Contains(out, "approve") {
		t.Errorf("help text must mention 'approve'; got:\n%s", out)
	}
	if !strings.Contains(out, "/v1/controlplane/surfaces/") {
		t.Errorf("help text must mention /v1/controlplane/surfaces/; got:\n%s", out)
	}
}

// TestInitQuickstart_HelpFlagPrintsHelp asserts that --help (handled by
// flag.ErrHelp) sets helpRequested=true and returns no error.
func TestInitQuickstart_HelpFlagPrintsHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			actor, helpRequested, err := parseInitQuickstartFlags([]string{arg})
			if err != nil {
				t.Fatalf("parseInitQuickstartFlags(%q): %v", arg, err)
			}
			if !helpRequested {
				t.Errorf("helpRequested must be true for %q", arg)
			}
			// actor is irrelevant when help is requested; the caller
			// short-circuits before using it.
			_ = actor
		})
	}
}

// ---------------------------------------------------------------------------
// Static-source assertions: the implementation must not synthesise an
// IAM principal or contain forbidden literals.
// ---------------------------------------------------------------------------

// TestInitQuickstart_DoesNotConstructSyntheticPrincipal walks the AST of
// init_quickstart.go and asserts:
//   - no composite literal of identity.Principal is constructed
//   - the literal "system:quickstart" does not appear anywhere
//   - "cli:init-quickstart" does not appear inside any composite literal
//     (it must remain a free-text string passed to ApplyBundle, never
//     bound to a principal struct field)
func TestInitQuickstart_DoesNotConstructSyntheticPrincipal(t *testing.T) {
	const path = "init_quickstart.go"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if bytes.Contains(src, []byte("system:quickstart")) {
		t.Fatalf(`forbidden literal "system:quickstart" found in %s`, path)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typeName := compositeLiteralTypeName(cl)
		if typeName == "" {
			return true
		}
		// Reject identity.Principal{...} and Principal{...} specifically.
		if typeName == "identity.Principal" || typeName == "Principal" {
			t.Errorf(`forbidden composite literal %s{...} at %s — `+
				`the quickstart CLI must not construct an IAM principal`,
				typeName, fset.Position(cl.Pos()))
			return true
		}
		// Inspect every key:value or scalar element. If any element is
		// the literal string "cli:init-quickstart" inside a struct that
		// looks like an IAM type (Principal, User, Role, Grant), fail.
		// The literal is only allowed as a flag default or argument.
		for _, elt := range cl.Elts {
			if !containsStringLiteral(elt, "cli:init-quickstart") {
				continue
			}
			// Allow it inside non-IAM types; flag if it's inside any
			// type whose name looks IAM-shaped.
			if isIAMShapedTypeName(typeName) {
				t.Errorf(`literal "cli:init-quickstart" appears inside IAM-shaped type %s at %s`,
					typeName, fset.Position(cl.Pos()))
			}
		}
		return true
	})
}

// compositeLiteralTypeName returns the printable type name for a
// CompositeLit, e.g. "identity.Principal" or "applyRecorder", or ""
// when the type cannot be cheaply expressed.
func compositeLiteralTypeName(cl *ast.CompositeLit) string {
	switch t := cl.Type.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	}
	return ""
}

// containsStringLiteral reports whether the AST subtree rooted at n
// contains a basic string literal whose unquoted value equals want.
func containsStringLiteral(n ast.Node, want string) bool {
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		bl, ok := node.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		// bl.Value is the source-quoted form, including quotes.
		if len(bl.Value) >= 2 && bl.Value[1:len(bl.Value)-1] == want {
			found = true
			return false
		}
		return true
	})
	return found
}

// isIAMShapedTypeName returns true when the type name looks like an
// identity, principal, role, or grant construct that would represent
// IAM state. Used to flag misuse of the audit-attribution string.
func isIAMShapedTypeName(name string) bool {
	lc := strings.ToLower(name)
	for _, needle := range []string{"principal", "identity", "role", "grant", "user", "subject"} {
		if strings.Contains(lc, needle) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Sanity: we are intentionally not driving the discard path
// ---------------------------------------------------------------------------

// TestExecuteQuickstart_ApplyReturnsValidationErrors_PropagatesError
// asserts that validation errors from ApplyBundle become an error,
// preventing the success message from being printed.
func TestExecuteQuickstart_ApplyReturnsValidationErrors_PropagatesError(t *testing.T) {
	r := successApplyResult()
	r.AddValidationError(types.KindSurface, "surf-broken", "synthetic-failure")

	rec := &applyRecorder{result: r}
	var buf bytes.Buffer
	err := executeQuickstart(
		context.Background(),
		&buf,
		rec.apply,
		fixedExists(false),
		quickstart.Bundle(),
		"cli:init-quickstart",
		quickstart.AnchorCapabilityID,
	)
	if err == nil {
		t.Fatal("want error: validation errors must surface as command failure")
	}
	if !strings.Contains(err.Error(), "validation") {
		t.Errorf("error must mention validation: %v", err)
	}
	if strings.Contains(buf.String(), "successfully") {
		t.Errorf("must not print success when validation errors present; got:\n%s", buf.String())
	}
}
