package httpapi

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/LeoHammes1/espmanager/internal/web"
)

// ParseTemplates builds the shared template set. partial lets the shell render an
// arbitrary content template by name (the one composition html/template can't do
// natively), so the shell chrome lives in exactly one place; dict lets a caller
// pass an ad-hoc struct to a component partial.
func ParseTemplates() (*template.Template, error) {
	var root *template.Template
	root = template.New("").Funcs(template.FuncMap{
		"partial": func(name string, data any) (template.HTML, error) {
			var b strings.Builder
			if err := root.ExecuteTemplate(&b, name, data); err != nil {
				return "", err
			}
			return template.HTML(b.String()), nil
		},
		"reltime": Reltime,
		"dict":    dict,
	})
	return root.ParseFS(web.FS, "templates/*.html")
}

func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			return nil, errors.New("dict: keys must be strings")
		}
		m[key] = kv[i+1]
	}
	return m, nil
}

// pageView wraps a content template + its data inside the app shell so the shell
// chrome (sidebar, header, drawer, toasts) is defined exactly once.
type pageView struct {
	Title   string
	Nav     string
	User    string
	Content string
	Data    any
}

func renderShell(w http.ResponseWriter, tmpl *template.Template, pv pageView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", pv); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func render(w http.ResponseWriter, tmpl *template.Template, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Reltime renders a compact, human relative time for last-seen / updated values.
func Reltime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
