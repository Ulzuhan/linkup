package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"sync"
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

var (
	assetVersionOnce sync.Once
	assetVersion     string
)

// AssetVersion is a short digest of everything under static/, computed once
// per process. It goes into the URLs of the stylesheets and the script so
// that a new build is a new URL.
//
// Without it, a deploy changed the file behind /static/css/app.css and left
// the address the same, and with no Cache-Control from here a CDN in front
// kept the old stylesheet for its default four hours while the new pages
// asked for classes it did not have. Production looked unstyled for everyone
// who had not seen the site before — and fine for whoever checked with curl,
// because the file answered 200 either way.
func AssetVersion() string {
	assetVersionOnce.Do(func() {
		h := sha256.New()
		var paths []string
		_ = fs.WalkDir(EmbeddedFS, "static", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				paths = append(paths, p)
			}
			return nil
		})
		sort.Strings(paths)
		for _, p := range paths {
			b, _ := EmbeddedFS.ReadFile(p)
			h.Write([]byte(p))
			h.Write(b)
		}
		assetVersion = hex.EncodeToString(h.Sum(nil))[:12]
	})
	return assetVersion
}

// StaticFS serves the embedded assets with cache headers that match how they
// are addressed: a URL that carries the current version can be kept for a
// year, because a change is a new URL; anything else — fonts referenced from
// the stylesheet, the favicon, an old page still asking — is kept a day.
func StaticFS() http.Handler {
	staticSubFS, err := fs.Sub(EmbeddedFS, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(staticSubFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") == AssetVersion() {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		files.ServeHTTP(w, r)
	})
}
