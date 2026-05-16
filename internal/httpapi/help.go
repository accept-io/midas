// Package httpapi — D33x-help-1 — Embedded MIDAS User Guide.
//
// The MIDAS User Guide is authored as Markdown under `userguide/src/` at the
// repo root, compiled by Material for MkDocs into `internal/httpapi/help/
// static/`, embedded into the MIDAS binary via `//go:embed`, and served by
// the Go server at `/help/`. There is no runtime dependency on Python or
// MkDocs — the static site is committed alongside the source, so the
// runtime image is the same pure-Go distroless container.
//
// To regenerate the static site after editing the Markdown source, run
// `make help-build` (which shells out to `squidfunk/mkdocs-material` in
// Docker — no host install required).
//
// The route is mounted inside `WithExplorerEnabled(true)` because the
// `/help/` surface is the Explorer's in-app help; nothing else in the
// runtime opens it.

package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"sync"
)

// helpFS embeds the entire compiled MIDAS User Guide. The `all:` prefix is
// defensive: it tells `//go:embed` to include files whose names start with
// `.` or `_`, in case a future MkDocs theme generates one. Today's Material
// for MkDocs output contains neither, but the embed directive is fixed at
// compile time and forgetting `all:` later is a silent regression.
//
//go:embed all:help/static
var helpFS embed.FS

// helpStaticOnce / helpStaticFS / helpStaticErr cache the chrooted sub-FS.
// `fs.Sub` is O(1) but allocates; computing it once at first request avoids
// per-request allocation on the hot path.
var (
	helpStaticOnce sync.Once
	helpStaticFS   fs.FS
	helpStaticErr  error
)

// helpStaticFileSystem returns a sub-FS rooted at `help/static/` so request
// paths under `/help/` map directly to files. Without this chroot, a request
// for `/help/index.html` would be rewritten by StripPrefix to `/index.html`
// and then resolved against `help/static/index.html` in the embedded FS,
// which would 404.
func helpStaticFileSystem() (fs.FS, error) {
	helpStaticOnce.Do(func() {
		helpStaticFS, helpStaticErr = fs.Sub(helpFS, "help/static")
	})
	return helpStaticFS, helpStaticErr
}

// handleHelpAssets serves the compiled MIDAS User Guide at `/help/...`.
// The request path is stripped of its `/help` prefix and resolved against
// the chrooted sub-FS. `http.FileServer` handles the directory-index dance
// for clean URLs (`/help/graphs/authority-graph/` resolves to
// `graphs/authority-graph/index.html`) because MkDocs is configured with
// `use_directory_urls: true`.
//
// `GET /help` (without the trailing slash) is handled by Go's `ServeMux`
// subtree-redirect behaviour: registering this handler at `GET /help/`
// automatically redirects `/help` → `/help/` with a 301.
func (s *Server) handleHelpAssets(w http.ResponseWriter, r *http.Request) {
	sub, err := helpStaticFileSystem()
	if err != nil {
		http.Error(w, "help: static assets unavailable", http.StatusInternalServerError)
		return
	}
	http.StripPrefix("/help", http.FileServer(http.FS(sub))).ServeHTTP(w, r)
}
