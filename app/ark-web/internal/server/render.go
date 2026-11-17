package server

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed templates static
var embedded embed.FS

type renderer struct {
	mu        sync.Mutex
	templates map[string]*template.Template
}

func newRenderer() *renderer {
	return &renderer{templates: make(map[string]*template.Template)}
}

func (r *renderer) render(w http.ResponseWriter, page string, data any) {
	tmpl, err := r.load(page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (r *renderer) renderPartial(w io.Writer, tmplName string, partial string, data any) error {
	tmpl, err := r.load(tmplName)
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, partial, data)
}

func (r *renderer) load(page string) (*template.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tmpl, ok := r.templates[page]; ok {
		return tmpl, nil
	}

	tmpl := template.New("").Funcs(template.FuncMap{
		"len": func(v any) int {
			switch v := v.(type) {
			case []string:
				return len(v)
			case map[string]struct{}:
				return len(v)
			default:
				return 0
			}
		},
	})

	basePatterns := []string{"templates/base.html", "templates/partials/*.html"}
	pagePatterns := []string{"templates/pages/" + page + ".html"}

	for _, p := range basePatterns {
		matches, err := fs.Glob(embedded, p)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			b, err := embedded.ReadFile(m)
			if err != nil {
				return nil, err
			}
			_, parseErr := tmpl.New(filepath.Base(m)).Parse(string(b))
			if parseErr != nil {
				return nil, parseErr
			}
		}
	}

	for _, p := range pagePatterns {
		matches, err := fs.Glob(embedded, p)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			b, err := embedded.ReadFile(m)
			if err != nil {
				return nil, err
			}
			if _, parseErr := tmpl.Parse(string(b)); parseErr != nil {
				return nil, parseErr
			}
		}
	}

	r.templates[page] = tmpl
	return tmpl, nil
}

func serveStatic() http.Handler {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

func isHTMX(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("HX-Request")) == "true"
}
