# MIDAS User Guide source

This directory holds the source for the in-app **MIDAS User Guide** served
at `/help/`. It is the user-facing help surface for the application
experience — distinct from `docs/`, which holds engineering / design /
operational documentation.

## Layout

```
userguide/
  mkdocs.yml          # Material for MkDocs config
  src/
    index.md          # /help/
    explorer/         # /help/explorer/
    graphs/           # /help/graphs/
    governance/       # /help/governance/
    evidence/         # /help/evidence/
    operations/       # /help/operations/
    assets/screenshots/  # placeholder dirs; commit PNGs here later
```

Generated output goes to **`internal/httpapi/help/static/`**, which is
committed and embedded into the MIDAS binary via `//go:embed help/static`
in `internal/httpapi/help.go`.

## Building locally

```
make help-build
```

The target shells out to `docker run squidfunk/mkdocs-material:latest
build` so no Python or MkDocs install is required on the host.

To wipe generated output:

```
make help-clean
```

## Adding a new page

1. Create `userguide/src/<area>/<page>.md` (kebab-case filename).
2. Add the page to `nav:` in `userguide/mkdocs.yml`.
3. Add anchor IDs only when the heading slug would otherwise drift. Use
   `## Heading {#explicit-anchor}` — `attr_list` is enabled.
4. `make help-build` to regenerate.
5. If the new page is a likely target for context-help, add a mapping in
   `internal/httpapi/explorer/assets/js/help/help-links.js`.

## Adding a screenshot

1. Drop the PNG under
   `userguide/src/assets/screenshots/<area>/<name>.png`.
2. Reference it from Markdown with a relative path:

   ```
   ![Authority Graph diagnostics](../assets/screenshots/graphs/authority-graph-diagnostics.png)
   ```

3. `make help-build`.

## Adding a context mapping

Help button context resolution lives in
`internal/httpapi/explorer/assets/js/help/help-context.js`. The mapping
keys are in `help-links.js`. Add the new key + URL to the `CONTEXT_MAP`
and (if needed) extend the resolver to surface it.

Keep URLs in the canonical shape `/help/<area>/<page>/[#anchor]`. Do not
introduce `/help/help/<page>/` shapes — the route is mounted at `/help/`
so the source files live directly under `userguide/src/`, never under
`userguide/src/help/`.

## Build-time vs runtime

- **MkDocs is build-time only.** The runtime image (`Dockerfile`) is the
  same single Go binary — no Python, no MkDocs, no second container.
- **MIDAS remains a single application/container.** No second runtime
  process, no second port, no proxy.
- **React and Docusaurus are intentionally not introduced** for this
  tranche. The User Guide is plain Markdown compiled by MkDocs into
  static HTML.
