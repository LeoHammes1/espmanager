package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const spaUnbuilt = `<!doctype html><html><head><meta charset="utf-8"><title>ESPManager</title></head>` +
	`<body style="font-family:system-ui;background:#0f1115;color:#e6e8eb;padding:2rem">` +
	`<h1>ESPManager</h1><p>The web UI has not been built. Run ` +
	`<code>npm --prefix web ci &amp;&amp; npm --prefix web run build</code> and rebuild the binary.</p></body></html>`

// spaHandler serves the embedded single-page app: real assets get long-lived
// immutable caching, every other GET falls back to index.html (served directly,
// since http.FileServer would 301 /index.html to ./). It runs as chi's NotFound,
// after the real API/static routes are resolved.
func spaHandler(sub fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(sub))
	index, indexErr := fs.ReadFile(sub, "index.html")

	serveIndex := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if indexErr != nil {
			_, _ = w.Write([]byte(spaUnbuilt))
			return
		}
		_, _ = w.Write(index)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" && p != "index.html" {
			if f, err := sub.Open(p); err == nil {
				_ = f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			// A missing file request (asset path or anything with an extension) is a
			// real 404 — only route-like paths fall back to the SPA shell.
			if strings.HasPrefix(p, "assets/") || strings.Contains(path.Base(p), ".") {
				http.NotFound(w, r)
				return
			}
		}
		serveIndex(w)
	}
}
