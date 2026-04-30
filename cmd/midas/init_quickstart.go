package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/quickstart"
	"github.com/accept-io/midas/internal/store/postgres"
)

// defaultActor is the audit attribution string passed to ApplyBundle when
// the operator does not override it via --actor. It is a free-text audit
// label only — not an IAM principal, not a role, and not a permission.
//
// This constant lives in the cmd/midas package because it is a CLI policy
// (which actor string this command attributes its writes to), not a
// property of the embedded bundle.
const defaultActor = "cli:init-quickstart"

// memoryRejectionMessage is the user-facing error returned when
// `midas init quickstart` is invoked against a memory-backed store.
// Memory state is per-process and there is no transaction primitive,
// so the bundle would not survive the command's exit.
const memoryRejectionMessage = `midas init quickstart: store.backend=memory is not supported.
Memory backend has no transaction primitive, and memory state is per-process.
The quickstart bundle would not survive this command's exit. Use the postgres backend.`

// alreadyAppliedMessage is the user-facing error returned when the
// preflight detects the anchor capability already exists. Re-running
// would create new pending-review versions of every bundle Surface.
const alreadyAppliedMessage = `midas init quickstart: bundle already applied (preflight detected anchor capability %q).
Re-running would create new pending-review versions of all bundle Surfaces. Use the
apply path to evolve the platform from here.`

// applyBundleFunc is the apply-bundle entry signature used by the
// testable core. It exactly matches apply.Service.ApplyBundle so the
// production path is a method-value pass-through.
type applyBundleFunc func(ctx context.Context, bundle []byte, actor string) (*types.ApplyResult, error)

// capabilityExistsFunc is the preflight check signature used by the
// testable core. It matches the Capability repository's Exists method.
type capabilityExistsFunc func(ctx context.Context, id string) (bool, error)

// runInitQuickstart is the CLI entry point dispatched from cmd/midas/main.go
// for `midas init quickstart`.
func runInitQuickstart(args []string) error {
	actor, helpRequested, err := parseInitQuickstartFlags(args)
	if err != nil {
		return err
	}
	if helpRequested {
		printInitQuickstartHelp(os.Stdout)
		return nil
	}

	cfgResult, err := config.Load(config.LoadOptions{})
	if err != nil {
		return fmt.Errorf("midas init quickstart: load config: %w", err)
	}
	if err := config.ValidateStructural(cfgResult.Config); err != nil {
		return fmt.Errorf("midas init quickstart: invalid config: %w", err)
	}
	if err := config.ValidateSemantic(cfgResult.Config); err != nil {
		return fmt.Errorf("midas init quickstart: invalid config: %w", err)
	}

	return runQuickstartWithConfig(context.Background(), cfgResult.Config, actor, os.Stdout)
}

// parseInitQuickstartFlags parses the subcommand's flag arguments.
//
// Returns (actor, helpRequested, error). When helpRequested is true the
// caller should print help text and exit zero without further work.
//
// The default actor is the package-level constant defaultActor
// ("cli:init-quickstart"). It is only an audit attribution string; it is
// not modelled as an IAM principal anywhere.
//
// An explicit empty or whitespace-only --actor value is rejected: the
// quickstart command requires a recognisable audit label so per-resource
// controlaudit rows are unambiguously attributable.
func parseInitQuickstartFlags(args []string) (string, bool, error) {
	fs := flag.NewFlagSet("init quickstart", flag.ContinueOnError)
	// Suppress flag's automatic error/usage output; runInitQuickstart owns
	// help-text printing via printInitQuickstartHelp.
	fs.SetOutput(io.Discard)

	actor := fs.String("actor", defaultActor,
		`audit attribution string passed to apply.Service.ApplyBundle (free-text)`)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", true, nil
		}
		return "", false, fmt.Errorf("midas init quickstart: %w", err)
	}
	if fs.NArg() > 0 {
		return "", false, fmt.Errorf("midas init quickstart: unexpected positional arguments: %v", fs.Args())
	}
	trimmed := strings.TrimSpace(*actor)
	if trimmed == "" {
		return "", false, errors.New("midas init quickstart: --actor must not be empty")
	}
	return trimmed, false, nil
}

// printInitQuickstartHelp writes the subcommand help text. The text names
// the default actor, prerequisites, what the bundle creates, the review
// status of Surfaces, and the surface-approval endpoint operators must
// call to bring Surfaces from review to active.
func printInitQuickstartHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage: midas init quickstart [--actor <id>]

Apply the embedded structural quickstart bundle through the standard
control-plane apply path.

The bundle creates 2 BusinessServices, 4 Capabilities, 5 BusinessServiceCapability
links, 4 Processes, and 6 Surfaces — a structural skeleton demonstrating
Capability ↔ BusinessService → Process → Surface.

Surfaces persist in review status (the apply path's standard behaviour).
Approve them through /v1/controlplane/surfaces/{id}/approve before
submitting /v1/evaluate calls; until they are approved, evaluation will
fail with SURFACE_INACTIVE.

Agent, Profile, and Grant documents are not created by this command.
Author them through the normal apply path. Once your Profile is approved
via /v1/controlplane/profiles/{id}/approve, /v1/evaluate against your
Surface will succeed.

Prerequisites:
  - a valid midas config (run 'midas config validate' to check)
  - the postgres store backend (memory is not supported)

Flags:
  --actor <id>    Free-text audit attribution string passed to ApplyBundle
                  (default: "cli:init-quickstart"). This is only an audit
                  label written to controlaudit rows; it is not an IAM
                  principal, role, or permission.
  -h, --help      Show this help.

This command does not auto-approve any resource and does not create any
synthetic actor or principal.`)
}

// runQuickstartWithConfig is the configuration-aware layer split out
// from runInitQuickstart so memory-rejection and Postgres wiring can be
// exercised in tests without driving config-file IO.
func runQuickstartWithConfig(ctx context.Context, cfg config.Config, actor string, w io.Writer) error {
	if cfg.Store.Backend == "memory" {
		return errors.New(memoryRejectionMessage)
	}

	repos, repoStore, _, cleanup, _, err := buildRepositories(ctx, cfg.Store)
	if err != nil {
		return fmt.Errorf("midas init quickstart: build repositories: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	pgStore, ok := repoStore.(*postgres.Store)
	if !ok {
		return fmt.Errorf("midas init quickstart: expected *postgres.Store, got %T", repoStore)
	}

	applySvc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		ControlAudit:                repos.ControlAudit,
		Processes:                   repos.Processes,
		Capabilities:                repos.Capabilities,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
		Tx:                          postgres.NewApplyTxRunner(pgStore),
	})

	return executeQuickstart(
		ctx,
		w,
		applySvc.ApplyBundle,
		repos.Capabilities.Exists,
		quickstart.Bundle(),
		actor,
		quickstart.AnchorCapabilityID,
	)
}

// executeQuickstart is the testable core: preflight, apply, and output.
//
// It takes injected applyBundle and capExists functions so tests can drive
// the success path, the re-run-refusal path, and the actor-passthrough
// path without standing up real repositories or apply services.
func executeQuickstart(
	ctx context.Context,
	w io.Writer,
	applyBundle applyBundleFunc,
	capExists capabilityExistsFunc,
	bundle []byte,
	actor, anchorCapID string,
) error {
	exists, err := capExists(ctx, anchorCapID)
	if err != nil {
		return fmt.Errorf("midas init quickstart: preflight check failed: %w", err)
	}
	if exists {
		return fmt.Errorf(alreadyAppliedMessage, anchorCapID)
	}

	result, err := applyBundle(ctx, bundle, actor)
	if err != nil {
		return fmt.Errorf("midas init quickstart: apply failed: %w", err)
	}
	if result == nil {
		return errors.New("midas init quickstart: apply returned nil result")
	}
	if result.HasValidationErrors() {
		return fmt.Errorf("midas init quickstart: bundle failed validation: %s",
			summariseValidationErrors(result))
	}
	if n := result.ApplyErrorCount(); n > 0 {
		return fmt.Errorf("midas init quickstart: %d resource(s) failed to apply: %s",
			n, summariseApplyErrors(result))
	}

	writeQuickstartSuccess(w, result)
	return nil
}

// summariseValidationErrors renders the validation errors on an
// ApplyResult into a single-line, human-legible string for inclusion in
// a returned error.
func summariseValidationErrors(r *types.ApplyResult) string {
	parts := make([]string, 0, len(r.ValidationErrors))
	for _, ve := range r.ValidationErrors {
		parts = append(parts, fmt.Sprintf("%s/%s field=%s msg=%s",
			ve.Kind, ve.ID, ve.Field, ve.Message))
	}
	return strings.Join(parts, "; ")
}

// summariseApplyErrors renders the per-resource apply errors on an
// ApplyResult into a single-line string.
func summariseApplyErrors(r *types.ApplyResult) string {
	parts := []string{}
	for _, res := range r.Results {
		if res.Status == types.ResourceStatusError {
			parts = append(parts, fmt.Sprintf("%s/%s: %s", res.Kind, res.ID, res.Message))
		}
	}
	return strings.Join(parts, "; ")
}

// writeQuickstartSuccess prints the structured success message to w. It
// names every created Surface ID and points at the surface-approval
// endpoint the operator must call to bring Surfaces from review to active.
func writeQuickstartSuccess(w io.Writer, r *types.ApplyResult) {
	counts := map[string]int{}
	var surfaceIDs []string
	for _, res := range r.Results {
		if res.Status != types.ResourceStatusCreated {
			continue
		}
		counts[res.Kind]++
		if res.Kind == types.KindSurface {
			surfaceIDs = append(surfaceIDs, res.ID)
		}
	}
	sort.Strings(surfaceIDs)

	fmt.Fprintln(w, "MIDAS quickstart bundle applied successfully.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Created:")
	fmt.Fprintf(w, "  - %d BusinessServices\n", counts[types.KindBusinessService])
	fmt.Fprintf(w, "  - %d Capabilities\n", counts[types.KindCapability])
	fmt.Fprintf(w, "  - %d BusinessServiceCapability links\n", counts[types.KindBusinessServiceCapability])
	fmt.Fprintf(w, "  - %d Processes\n", counts[types.KindProcess])
	fmt.Fprintf(w, "  - %d Surfaces (in review status)\n", counts[types.KindSurface])
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  1. Approve the bundle's Surfaces. They are in review status and")
	fmt.Fprintln(w, "     /v1/evaluate calls will fail with SURFACE_INACTIVE until they")
	fmt.Fprintln(w, "     are approved.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "     Surfaces created by this bundle:")
	for _, id := range surfaceIDs {
		fmt.Fprintf(w, "       - %s\n", id)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "     Approve each via:")
	fmt.Fprintln(w, "       POST /v1/controlplane/surfaces/<id>/approve")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  2. Author your first Agent, Profile, and Grant through the apply")
	fmt.Fprintln(w, "     path. The quickstart bundle establishes the structural skeleton")
	fmt.Fprintln(w, "     (Capability ↔ BusinessService → Process → Surface); completing")
	fmt.Fprintln(w, "     the authority chain is your first governance authoring exercise.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  3. Once your Profile is approved (POST /v1/controlplane/profiles/<id>/approve),")
	fmt.Fprintln(w, "     /v1/evaluate calls against your Surface using the new Agent and")
	fmt.Fprintln(w, "     Grant will succeed.")
}
