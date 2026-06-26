package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// SPA returns the built single-page-app filesystem (the Vite output embedded at
// build time). It is served by the HTTP layer with an index.html fallback.
func SPA() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
