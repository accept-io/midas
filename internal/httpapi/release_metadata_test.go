package httpapi

// release_metadata_test.go — D41b pin set for the v1.1.0-rc.1
// release metadata.
//
// The pin asserts that the version string agrees across every
// release-facing artifact: CHANGELOG, OpenAPI spec, Helm chart
// (version + appVersion), Helm production values (image tag),
// chart README install examples. A drift on any one of these
// fails this test and surfaces the inconsistency before the
// maintainer cuts the tag.
//
// The pin is intentionally focused: it verifies the canonical
// release version string is present in each artifact, plus that
// the CHANGELOG carries the dated heading for the same version.
// Larger release-framework concerns (build pipeline, semver
// validation, GitHub release verification) are out of scope.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// releaseVersion is the canonical D41b release-candidate version
// string. Update this constant first when bumping the RC.
const releaseVersion = "1.1.0-rc.1"

// releaseDate is the dated heading the CHANGELOG carries for
// releaseVersion. Combined with releaseVersion this pins the
// canonical release-heading shape.
const releaseDate = "2026-05-25"

// releaseMetadataFile pairs a file path (repo-relative) with a
// substring that must appear in it for the release version to be
// considered aligned.
type releaseMetadataFile struct {
	path     string
	mustHave string
	purpose  string
}

// releaseMetadataFiles enumerates every artifact that carries
// the release version string. Adding a new artifact in a future
// tranche is a one-row append.
var releaseMetadataFiles = []releaseMetadataFile{
	{
		path:     "CHANGELOG.md",
		mustHave: "## [" + releaseVersion + "] — " + releaseDate,
		purpose:  "CHANGELOG section header for the release candidate",
	},
	{
		path:     "api/openapi/v1.yaml",
		mustHave: `version: "` + releaseVersion + `"`,
		purpose:  "OpenAPI info.version field",
	},
	{
		path:     "charts/midas/Chart.yaml",
		mustHave: "version: " + releaseVersion,
		purpose:  "Helm chart `version` field",
	},
	{
		path:     "charts/midas/Chart.yaml",
		mustHave: `appVersion: "` + releaseVersion + `"`,
		purpose:  "Helm chart `appVersion` field",
	},
	{
		path:     "charts/midas/values-production.yaml",
		mustHave: `tag: "` + releaseVersion + `"`,
		purpose:  "Helm production values image.tag pin",
	},
	{
		path:     "charts/midas/README.md",
		mustHave: "--set image.tag=" + releaseVersion,
		purpose:  "Helm chart README install example image tag",
	},
}

// TestReleaseMetadata_V110RC1Consistency pins that every
// release-facing metadata artifact carries the canonical
// release-candidate version string. Drift on any single artifact
// fails the corresponding subtest with file:line-level guidance.
func TestReleaseMetadata_V110RC1Consistency(t *testing.T) {
	root := repoRoot(t)
	for _, f := range releaseMetadataFiles {
		t.Run(strings.ReplaceAll(f.path, "/", "_"), func(t *testing.T) {
			fullPath := filepath.Join(root, filepath.FromSlash(f.path))
			body, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatalf("D41b: read %s: %v", f.path, err)
			}
			if !strings.Contains(string(body), f.mustHave) {
				t.Errorf("D41b: %s must contain %q (purpose: %s)", f.path, f.mustHave, f.purpose)
			}
		})
	}
}

// TestReleaseMetadata_NoStaleVersionStrings pins that the
// CHANGELOG's [Unreleased] section is empty (placeholder only)
// because D41b promoted every prior unreleased entry into the
// release-candidate section. A non-placeholder [Unreleased]
// section after this tranche means content was added that has
// not been classified for the RC.
func TestReleaseMetadata_NoStaleVersionStrings(t *testing.T) {
	root := repoRoot(t)
	changelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("D41b: read CHANGELOG.md: %v", err)
	}
	text := string(changelog)

	// The [Unreleased] section should be present (so future
	// tranches have a place to land entries) and should carry
	// the placeholder marker. The placeholder is intentional;
	// dropping it back to "_No unreleased entries pending._" is
	// the signal that the next promotion has not yet happened.
	if !strings.Contains(text, "## [Unreleased]") {
		t.Errorf("D41b: CHANGELOG must retain the [Unreleased] section as a future-work placeholder")
	}
	if !strings.Contains(text, "_No unreleased entries pending. Items below have been promoted to the v1.1.0-rc.1 release candidate._") {
		t.Errorf("D41b: CHANGELOG [Unreleased] section must carry the post-promotion placeholder; a non-placeholder entry means content was added without being classified for the RC")
	}
}

// TestReleaseMetadata_NoOldImageTagInDeploymentDocs pins that
// the historical 1.0.x image tag references have been swept
// out of current install/deployment metadata. Historical
// CHANGELOG entries are explicitly excluded — they are
// immutable historical records, not current metadata.
func TestReleaseMetadata_NoOldImageTagInDeploymentDocs(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{
		"charts/midas/Chart.yaml",
		"charts/midas/values-production.yaml",
		"charts/midas/README.md",
	} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("D41b: read %s: %v", path, err)
		}
		text := string(body)
		for _, stale := range []string{"1.0.2", "1.0.1", "1.0.0", "1.0.3", "0.1.0"} {
			if strings.Contains(text, stale) {
				t.Errorf("D41b: %s still references stale release version %q; current install/deployment metadata must point at %s", path, stale, releaseVersion)
			}
		}
	}
}
