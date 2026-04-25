package httpapi

// openapi_contract_test.go — asserts symmetry between paths declared in
// api/openapi/v1.yaml and routes registered with the HTTP server.
//
// Scope per the contract-test brief: paths only, not request/response
// schemas. A spec path with no matching registered route, or a registered
// API contract route with no matching spec path, fails the test.
//
// Route enumeration is by static parsing of the route-registration source
// (server.go, auth.go, and any other non-test file in this package), via
// regex over `s.mux.HandleFunc("…",`. The Server type intentionally exposes
// no Routes() method — the source itself is the canonical registry.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// repoRoot resolves the repository root from this test file's location
// (internal/httpapi/) so the test does not depend on the working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Tests run with cwd = internal/httpapi; root is two levels up.
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// loadSpecPaths reads api/openapi/v1.yaml and returns the keys of its
// `paths:` map. Uses gopkg.in/yaml.v3 (already vendored, see go.mod).
func loadSpecPaths(t *testing.T) []string {
	t.Helper()
	specPath := filepath.Join(repoRoot(t), "api", "openapi", "v1.yaml")
	b, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}
	var doc struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}
	out := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// handleFuncCall and handleFuncLiteralRE capture the first argument to
// s.mux.HandleFunc(...). The literal form is ALWAYS a double-quoted string
// in this codebase; if anyone introduces a non-literal first argument
// (constant, variable, function call), the sanity test below will fail.
//
// The literal may include a Go-1.22-style method prefix, e.g. "GET /explorer".
var (
	handleFuncCallRE    = regexp.MustCompile(`\.HandleFunc\(`)
	handleFuncLiteralRE = regexp.MustCompile(`\.HandleFunc\("([^"]+)"`)
)

// registeredRoute couples the raw literal (for error reporting) and its
// path component (with any method prefix stripped).
type registeredRoute struct {
	literal  string // exact first-arg literal, e.g. "GET /explorer"
	path     string // path only, e.g. "/explorer"
	file     string // file relative to repo root
	line     int    // 1-based line number
	hasSlash bool   // path ends with "/" — prefix route in net/http ServeMux
}

// loadRegisteredRoutes walks internal/httpapi/*.go (excluding _test.go) and
// extracts every HandleFunc registration. The two regexes also serve as a
// loud-failure sanity check: if the literal regex matches fewer times than
// the call regex, at least one HandleFunc call has a non-literal first
// argument, and the test FAILS naming the offending file:line.
func loadRegisteredRoutes(t *testing.T) []registeredRoute {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "httpapi")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}

	var (
		routes      []registeredRoute
		callTotal   int
		matchTotal  int
		unmatchedAt []string
	)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		lines := strings.Split(string(body), "\n")
		for i, line := range lines {
			callsHere := len(handleFuncCallRE.FindAllString(line, -1))
			matchesHere := handleFuncLiteralRE.FindAllStringSubmatch(line, -1)
			callTotal += callsHere
			matchTotal += len(matchesHere)

			if callsHere != len(matchesHere) {
				// Loud failure precondition: a HandleFunc( call exists on this
				// line but the literal-string regex did not capture it.
				unmatchedAt = append(unmatchedAt, name+":"+itoa(i+1)+": "+strings.TrimSpace(line))
			}

			for _, m := range matchesHere {
				literal := m[1]
				p := stripMethodPrefix(literal)
				routes = append(routes, registeredRoute{
					literal:  literal,
					path:     p,
					file:     name,
					line:     i + 1,
					hasSlash: strings.HasSuffix(p, "/") && p != "/",
				})
			}
		}
	}

	if callTotal != matchTotal || len(unmatchedAt) > 0 {
		// Per the brief: silent misses defeat the purpose. Name the offenders.
		t.Fatalf("HandleFunc literal regex did not match every HandleFunc( call (%d calls, %d literals captured). Unparseable sites:\n  %s",
			callTotal, matchTotal, strings.Join(unmatchedAt, "\n  "))
	}
	return routes
}

// stripMethodPrefix removes a leading "METHOD " from a Go-1.22-style
// HandleFunc literal, leaving just the path component.
func stripMethodPrefix(literal string) string {
	if i := strings.IndexByte(literal, ' '); i > 0 {
		method := literal[:i]
		if isAllUpper(method) {
			return literal[i+1:]
		}
	}
	return literal
}

func isAllUpper(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// itoa is a tiny stdlib-free int-to-string. Avoids pulling strconv just for
// error formatting in this single test file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// nonContractAllowlist enumerates registered route paths that intentionally
// sit outside the OpenAPI contract. Each entry MUST trace to a category
// documented in the Step 0.5b inventory.
//
// Matching rule: a registered route is excluded if its path equals one of
// the prefix entries OR begins with one of the prefix entries followed by
// any character (covering both "/auth/login" and "/explorer/" against
// "/auth/" / "/explorer" prefixes).
//
// Note: /healthz and /readyz are intentionally NOT excluded — the existing
// OpenAPI spec documents them (v1.yaml lines 878 and 903), so they are
// part of the contract surface despite being probes.
var nonContractAllowlist = []struct {
	prefix string
	reason string
}{
	{"/auth/", "local IAM and OIDC mechanics; not part of API contract"},
	{"/explorer", "developer sandbox per ADR memory mode; not part of API contract"},
}

func isNonContract(path string) bool {
	for _, e := range nonContractAllowlist {
		if path == e.prefix || strings.HasPrefix(path, e.prefix) {
			return true
		}
		// Bare-stem match: "/explorer" should also exclude "/explorer/"
		// even though the entry has no trailing slash.
		if !strings.HasSuffix(e.prefix, "/") && path == e.prefix+"/" {
			return true
		}
	}
	return false
}

// expectedRegisteredPathsForSpec returns the registered-route paths that
// would satisfy a given spec path. A spec path with `{placeholder}`
// segments collapses to its first parent prefix ending in `/` (matching
// net/http ServeMux subtree-routing semantics). A spec path without
// placeholders maps to itself exactly.
//
// Examples:
//
//	/v1/businessservices             → ["/v1/businessservices"]
//	/v1/businessservices/{id}        → ["/v1/businessservices/"]
//	/v1/capabilities/{id}/processes  → ["/v1/capabilities/"]
//	/v1/controlplane/surfaces/{id}/approve → ["/v1/controlplane/surfaces/"]
//
// This is the dedicated, commented prefix-matching function called out in
// the Q2 constraint reminder. Future readers: bare path comparison fails
// for these endpoints because net/http ServeMux uses subtree prefix
// routing (registered "/v1/foo/" handles every "/v1/foo/<anything>"),
// while the OpenAPI spec documents the fully-qualified action paths.
func expectedRegisteredPathsForSpec(specPath string) []string {
	parts := strings.Split(specPath, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			return []string{strings.Join(parts[:i], "/") + "/"}
		}
	}
	return []string{specPath}
}

// PathDiff reports the result of comparing a spec path set against a
// registered route set. Exposed (lowercase, package-internal) for use by
// the negative-fixture tests below.
type pathDiff struct {
	// SpecPathsWithoutHandler — spec declares the path but no registered
	// route satisfies it.
	SpecPathsWithoutHandler []string

	// RegisteredRoutesWithoutSpec — registered route is contract-classified
	// but no spec path is satisfied by it.
	RegisteredRoutesWithoutSpec []string

	// PrefixRoutesWithNoMatchingSpec — Q2 tightening: a registered prefix
	// route (trailing slash) must have at least one spec path beginning
	// with it. If none, this prevents a regression where all spec
	// sub-paths are removed while the prefix registration remains.
	PrefixRoutesWithNoMatchingSpec []string
}

// comparePaths is the symmetry check, extracted into a pure function so the
// negative-fixture tests can drive it directly with synthetic inputs.
//
// `registeredPaths` must already be filtered to contract routes only
// (non-contract allowlist applied). `specPaths` is the raw set from
// api/openapi/v1.yaml.
func comparePaths(specPaths, registeredPaths []string) pathDiff {
	specSet := make(map[string]struct{}, len(specPaths))
	for _, p := range specPaths {
		specSet[p] = struct{}{}
	}
	regSet := make(map[string]struct{}, len(registeredPaths))
	for _, p := range registeredPaths {
		regSet[p] = struct{}{}
	}

	var diff pathDiff

	// Spec → registered: every spec path must have a registered route that
	// would handle it.
	for _, sp := range specPaths {
		expected := expectedRegisteredPathsForSpec(sp)
		matched := false
		for _, exp := range expected {
			if _, ok := regSet[exp]; ok {
				matched = true
				break
			}
		}
		if !matched {
			diff.SpecPathsWithoutHandler = append(diff.SpecPathsWithoutHandler, sp)
		}
	}

	// Registered → spec: every registered route must match at least one
	// spec path. For prefix routes (trailing slash), at least one spec
	// path must begin with the prefix (Q2 tightening).
	for _, rp := range registeredPaths {
		if strings.HasSuffix(rp, "/") && rp != "/" {
			// Prefix route. Match if any spec path's
			// expectedRegisteredPathsForSpec yields this prefix.
			matched := false
			for _, sp := range specPaths {
				for _, exp := range expectedRegisteredPathsForSpec(sp) {
					if exp == rp {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				diff.PrefixRoutesWithNoMatchingSpec = append(diff.PrefixRoutesWithNoMatchingSpec, rp)
			}
		} else {
			// Exact-match route. Spec must contain it verbatim.
			if _, ok := specSet[rp]; !ok {
				diff.RegisteredRoutesWithoutSpec = append(diff.RegisteredRoutesWithoutSpec, rp)
			}
		}
	}

	sort.Strings(diff.SpecPathsWithoutHandler)
	sort.Strings(diff.RegisteredRoutesWithoutSpec)
	sort.Strings(diff.PrefixRoutesWithNoMatchingSpec)
	return diff
}

// TestOpenAPIContract_PathSymmetry is the headline test. It asserts that
// every path in api/openapi/v1.yaml is registered as a handler, and every
// registered API contract route appears in the spec.
func TestOpenAPIContract_PathSymmetry(t *testing.T) {
	specPaths := loadSpecPaths(t)
	registered := loadRegisteredRoutes(t)

	var contractPaths []string
	for _, r := range registered {
		if isNonContract(r.path) {
			continue
		}
		contractPaths = append(contractPaths, r.path)
	}

	diff := comparePaths(specPaths, contractPaths)

	if len(diff.SpecPathsWithoutHandler) > 0 {
		t.Errorf("OpenAPI spec declares paths with no registered handler:\n  %s",
			strings.Join(diff.SpecPathsWithoutHandler, "\n  "))
	}
	if len(diff.RegisteredRoutesWithoutSpec) > 0 {
		t.Errorf("Registered API contract routes are missing from OpenAPI spec:\n  %s",
			strings.Join(diff.RegisteredRoutesWithoutSpec, "\n  "))
	}
	if len(diff.PrefixRoutesWithNoMatchingSpec) > 0 {
		t.Errorf("Registered prefix routes have no matching spec sub-paths (Q2 tightening):\n  %s",
			strings.Join(diff.PrefixRoutesWithNoMatchingSpec, "\n  "))
	}
}

// TestOpenAPIContract_NegativeFixture_MissingSpecPath proves that
// comparePaths flags a registered exact-match route when its spec entry
// is absent. This is the Step-1.5 verification that the test catches the
// kind of drift that originally hid B-2 and B-3.
func TestOpenAPIContract_NegativeFixture_MissingSpecPath(t *testing.T) {
	registered := []string{"/v1/businessservices", "/v1/platform/admin-audit"}
	spec := []string{} // simulate pre-1.3 state

	diff := comparePaths(spec, registered)

	if len(diff.RegisteredRoutesWithoutSpec) != 2 {
		t.Fatalf("want 2 registered-without-spec entries, got %d: %v",
			len(diff.RegisteredRoutesWithoutSpec), diff.RegisteredRoutesWithoutSpec)
	}
	for _, want := range []string{"/v1/businessservices", "/v1/platform/admin-audit"} {
		found := false
		for _, got := range diff.RegisteredRoutesWithoutSpec {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in RegisteredRoutesWithoutSpec; got %v",
				want, diff.RegisteredRoutesWithoutSpec)
		}
	}
}

// TestOpenAPIContract_NegativeFixture_MissingPrefixRoute proves that
// comparePaths flags a registered prefix route when no spec sub-path
// begins with it. This guards against the regression Q2 calls out: all
// spec sub-paths removed while the prefix registration remains.
func TestOpenAPIContract_NegativeFixture_MissingPrefixRoute(t *testing.T) {
	registered := []string{"/v1/businessservices/"}
	spec := []string{} // no /v1/businessservices/{id} in spec

	diff := comparePaths(spec, registered)

	if len(diff.PrefixRoutesWithNoMatchingSpec) != 1 ||
		diff.PrefixRoutesWithNoMatchingSpec[0] != "/v1/businessservices/" {
		t.Fatalf("want PrefixRoutesWithNoMatchingSpec=[/v1/businessservices/], got %v",
			diff.PrefixRoutesWithNoMatchingSpec)
	}
}

// TestOpenAPIContract_NegativeFixture_MissingHandlerForSpec proves the
// reverse direction: a spec path with no registered handler is reported.
func TestOpenAPIContract_NegativeFixture_MissingHandlerForSpec(t *testing.T) {
	registered := []string{}
	spec := []string{"/v1/businessservices", "/v1/businessservices/{id}"}

	diff := comparePaths(spec, registered)

	if len(diff.SpecPathsWithoutHandler) != 2 {
		t.Fatalf("want 2 spec-without-handler entries, got %d: %v",
			len(diff.SpecPathsWithoutHandler), diff.SpecPathsWithoutHandler)
	}
}

// TestOpenAPIContract_PrefixMatching_PositiveCases sanity-checks that the
// prefix-matching helper correctly maps placeholder spec paths to their
// expected registered prefix.
func TestOpenAPIContract_PrefixMatching_PositiveCases(t *testing.T) {
	cases := []struct {
		spec string
		want string
	}{
		{"/v1/businessservices", "/v1/businessservices"},
		{"/v1/businessservices/{id}", "/v1/businessservices/"},
		{"/v1/capabilities/{id}/processes", "/v1/capabilities/"},
		{"/v1/controlplane/surfaces/{id}/approve", "/v1/controlplane/surfaces/"},
		{"/v1/decisions/request/{requestId}", "/v1/decisions/request/"},
	}
	for _, c := range cases {
		got := expectedRegisteredPathsForSpec(c.spec)
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("expectedRegisteredPathsForSpec(%q) = %v, want [%q]", c.spec, got, c.want)
		}
	}
}
