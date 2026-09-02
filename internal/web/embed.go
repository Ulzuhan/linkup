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
	// comunes se mezcla en cada render. Sin esto, un dato que aparece en el
	// layout —y por tanto en las cinco páginas— habría que pasarlo en las cinco
	// llamadas, y bastaría olvidarse en una para que la plantilla lo pintara
	// vacío sin avisar.
	comunes map[string]interface{}
}

// SetCommon fija los valores que toda plantilla recibe. Se llama una vez, al
// arrancar, antes de servir.
func (r *Renderer) SetCommon(valores map[string]interface{}) {
	r.comunes = valores
}

func NewRenderer() (*Renderer, error) {
	templates := make(map[string]*template.Template)

	pages := []string{"landing.html", "dashboard.html", "preview.html", "pin.html", "error.html", "settings.html"}
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
	// Lo común no pisa lo que la página trae: si una quiere su propio valor,
	// gana el suyo.
	if mapa, ok := data.(map[string]interface{}); ok && len(r.comunes) > 0 {
		mezcla := make(map[string]interface{}, len(mapa)+len(r.comunes))
		for k, v := range r.comunes {
			mezcla[k] = v
		}
		for k, v := range mapa {
			mezcla[k] = v
		}
		data = mezcla
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
