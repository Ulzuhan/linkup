package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
)

//go:embed static/* templates/*
var EmbeddedFS embed.FS

type Renderer struct {
	templates map[string]*template.Template
}

func NewRenderer() (*Renderer, error) {
	templates := make(map[string]*template.Template)

	pages := []string{"dashboard.html", "preview.html", "pin.html", "error.html", "settings.html"}
	for _, page := range pages {
		tmpl, err := template.ParseFS(EmbeddedFS, "templates/layout.html", "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", page, err)
		}
		templates[page] = tmpl
	}

	return &Renderer{templates: templates}, nil
}

func (r *Renderer) Render(w io.Writer, name string, data interface{}) error {
	tmpl, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	return tmpl.Execute(w, data)
}

func StaticFS() http.Handler {
	staticSubFS, err := fs.Sub(EmbeddedFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(staticSubFS))
}
