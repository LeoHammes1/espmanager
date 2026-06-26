package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandler(t *testing.T) {
	sub := fstest.MapFS{
		"index.html":    {Data: []byte(`<div id="root"></div>`)},
		"assets/app.js": {Data: []byte(`console.log(1)`)},
	}
	h := spaHandler(sub)

	t.Run("client route falls back to index", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/deploys/abc", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Fatalf("client route should serve the SPA shell, got %q", rec.Body.String())
		}
		if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("shell should be HTML, got %q", rec.Header().Get("Content-Type"))
		}
	})

	t.Run("root serves index", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Fatalf("root should serve the shell, got %d %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("real asset is served", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log") {
			t.Fatalf("asset should be served, got %d", rec.Code)
		}
		if rec.Header().Get("Cache-Control") == "" {
			t.Error("assets should carry a cache header")
		}
	})

	t.Run("non-GET is rejected", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/deploys", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("want 404 for non-GET, got %d", rec.Code)
		}
	})
}
