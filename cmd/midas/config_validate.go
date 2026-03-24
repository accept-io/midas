package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/accept-io/midas/internal/config"
)

// runConfigValidate implements `midas config validate`.
// It loads and runs all three validation stages, printing results to stdout.
// Returns a non-nil error if any stage fails; the process should exit non-zero.
func runConfigValidate(args []string) error {
	opts := config.LoadOptions{}

	for i, a := range args {
		switch a {
		case "--file", "-f":
			if i+1 < len(args) {
				opts.ConfigFile = args[i+1]
			}
		}
	}

	// --- Stage 1: Load (structural parse + placeholder expansion) ---
	fmt.Fprintln(os.Stdout, "Loading configuration...")
	result, err := config.Load(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL load: %v\n", err)
		return err
	}

	src := result.File
	if src == "" {
		src = "<built-in defaults>"
	}
	fmt.Fprintf(os.Stdout, "  File:    %s\n", src)
	fmt.Fprintf(os.Stdout, "  Profile: %s\n", result.Config.Profile)
	fmt.Fprintf(os.Stdout, "  Store:   %s\n", result.Config.Store.Backend)
	fmt.Fprintf(os.Stdout, "  Auth:    %s\n", result.Config.Auth.Mode)
	fmt.Fprintln(os.Stdout, "  [OK] load")

	// --- Stage 2: Structural validation ---
	fmt.Fprintln(os.Stdout, "Validating structure...")
	if err := config.ValidateStructural(result.Config); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL structural: %v\n", err)
		return err
	}
	fmt.Fprintln(os.Stdout, "  [OK] structural")

	// --- Stage 3: Semantic validation ---
	fmt.Fprintln(os.Stdout, "Validating semantics...")
	if err := config.ValidateSemantic(result.Config); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL semantic: %v\n", err)
		return err
	}
	fmt.Fprintln(os.Stdout, "  [OK] semantic")

	// --- Stage 4: Operational validation (connectivity) ---
	if result.Config.Store.Backend == "postgres" {
		fmt.Fprintln(os.Stdout, "Validating connectivity (postgres)...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := config.ValidateOperational(ctx, result.Config); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL operational: %v\n", err)
			return err
		}
		fmt.Fprintln(os.Stdout, "  [OK] operational")
	} else {
		fmt.Fprintln(os.Stdout, "Skipping connectivity check (store.backend=memory).")
	}

	fmt.Fprintln(os.Stdout, "\nConfiguration is valid.")
	return nil
}
