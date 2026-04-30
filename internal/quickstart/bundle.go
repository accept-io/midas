// Package quickstart embeds the static MIDAS quickstart bundle and exposes
// the small constants the CLI subcommand needs to apply it.
//
// The bundle is a structural skeleton: BusinessServices, Capabilities,
// BusinessServiceCapability links, Processes, and Surfaces. It is applied
// through the standard control-plane apply path; Surfaces persist in
// review status, matching the apply path's existing review-forcing
// behaviour. The CLI subcommand prints follow-up instructions pointing at
// the surface-approval endpoints.
//
// The bundle file is reviewed in source control. Changes to bundle.yaml
// are governed change.
package quickstart

import _ "embed"

// AnchorCapabilityID is the Capability ID used by the CLI subcommand's
// re-run preflight check. If a record with this ID already exists, the
// command refuses to apply the bundle a second time.
//
// The anchor must be the first Capability declared in bundle.yaml. If the
// bundle is reordered, update this constant accordingly.
const AnchorCapabilityID = "cap-identity-verification"

//go:embed bundle.yaml
var bundleYAML []byte

// Bundle returns the embedded quickstart bundle bytes verbatim. The bytes
// are the same content as bundle.yaml at build time.
func Bundle() []byte {
	return bundleYAML
}
