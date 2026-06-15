// Package console serves the embedded React single-page console app.
//
// The built frontend lives in dist/ (produced by `npm run build` in this
// directory) and is embedded into the Go binary, so a single artifact serves
// both the data-plane API and the self-host console. Run the build before
// `go build`; CI should run `npm ci && npm run build` in console/ first.
package console

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the console SPA at the site root.
// Static assets are served from the embedded filesystem; any unmatched path falls
// back to index.html so client-side routing works on deep links and refreshes.
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}

	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" || p == "index.html" {
			serveIndex(w)
			return
		}
		if f, err := sub.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(w) // SPA fallback for client-side routes
	}), nil
}
