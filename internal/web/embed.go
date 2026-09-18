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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Ulzuhan/linkup/internal/models"
)

//go:embed static/* templates/*
var EmbeddedFS embed.FS

type Renderer struct {
	templates map[string]*template.Template
	// comunes se mezcla en cada render. Sin esto, un dato que aparece en el
	// layout —y por tanto en las seis páginas— habría que pasarlo en las seis
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
		// The template is named after the layout so that Execute renders the
		// layout, which is what pulls the page's "content" block in.
		tmpl, err := template.New("layout.html").Funcs(funcs()).ParseFS(EmbeddedFS, "templates/layout.html", "templates/"+page)
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

// funcs is what the templates can call beyond the language itself. Kept
// deliberately small: formatting, a few readings of a link, and the pointer
// dereference the models force on us. Anything that decides something
// belongs in a handler or a service, not here.
func funcs() template.FuncMap {
	return template.FuncMap{
		"date":     fmtDate,
		"datetime": fmtDateTime,
		"iso":      fmtISO,
		"num":      fmtNum,
		"val":      asInt64,
		"pct":      percent,
		"hostOf":   hostOf,
		"restOf":   restOf,
		"state":    linkState,
		"initial":  initial,
		"join":     func(sep string, xs []string) string { return strings.Join(xs, sep) },
		// The rings on the cards: how much of a budget or a lifetime is left,
		// as a percentage, and the dash offset that draws it on a circle of
		// circumference 100.
		"budgetLeft": budgetLeft,
		"lifeLeft":   lifeLeft,
		"dash":       func(pct int) int { return 100 - pct },
		"remaining":  remaining,
	}
}

// budgetLeft is the share of a click budget still unspent, 0–100. A link
// without a budget has nothing to draw and answers 0.
func budgetLeft(l models.Link) int {
	if l.MaxClicks == nil || *l.MaxClicks <= 0 {
		return 0
	}
	left := 100 - percent(l.ClickCount, *l.MaxClicks)
	if left < 0 {
		return 0
	}
	return left
}

// lifeLeft is the share of a link's lifetime still ahead, 0–100, measured
// from when it was created to when it expires. Without an expiry, 0.
func lifeLeft(l models.Link) int {
	if l.ExpiresAt == nil || *l.ExpiresAt <= 0 || *l.ExpiresAt <= l.CreatedAt {
		return 0
	}
	now := time.Now().Unix()
	if now >= *l.ExpiresAt {
		return 0
	}
	left := int(float64(*l.ExpiresAt-now) / float64(*l.ExpiresAt-l.CreatedAt) * 100)
	if left > 100 {
		return 100
	}
	if left < 0 {
		return 0
	}
	return left
}

// remaining is the time until a moment, in the coarsest unit that is still
// honest and short enough for a ring label: "3d", "22h", "41m". A moment
// already past is "0m".
func remaining(v interface{}) string {
	n := asInt64(v)
	d := time.Until(time.Unix(n, 0))
	switch {
	case n <= 0 || d <= 0:
		return "0m"
	case d >= 48*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	case d >= time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Minutes())+1) + "m"
	}
}

// asInt64 reads an int, an int64 or a pointer to either; nil is zero. The
// models keep optional timestamps and budgets as pointers, and a template can
// print a pointer but cannot compare one.
func asInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case *int:
		if n != nil {
			return int64(*n)
		}
	case *int64:
		if n != nil {
			return *n
		}
	}
	return 0
}

func fmtDate(v interface{}) string {
	n := asInt64(v)
	if n <= 0 {
		return ""
	}
	return time.Unix(n, 0).Format("2 Jan 2006")
}

func fmtDateTime(v interface{}) string {
	n := asInt64(v)
	if n <= 0 {
		return ""
	}
	return time.Unix(n, 0).Format("2 Jan 2006, 15:04")
}

// fmtISO is for <time datetime="…">, which the script turns into "in 3 days".
func fmtISO(v interface{}) string {
	n := asInt64(v)
	if n <= 0 {
		return ""
	}
	return time.Unix(n, 0).Format(time.RFC3339)
}

// fmtNum groups thousands: 12480 → 12,480.
func fmtNum(v interface{}) string {
	n := asInt64(v)
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// percent of a over b, clamped to 0–100. Zero when there is no budget.
func percent(a, b interface{}) int {
	x, y := asInt64(a), asInt64(b)
	if y <= 0 {
		return 0
	}
	p := int(x * 100 / y)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// hostOf and restOf split a destination for display: the host in one weight,
// the path and query in another. A URL that does not parse is shown whole.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

func restOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	rest := u.RequestURI()
	if u.Fragment != "" {
		rest += "#" + u.Fragment
	}
	if rest == "/" {
		return ""
	}
	return rest
}

// linkState names what a link is doing right now: "active", "paused" (its
// owner switched it off), "expired" (its date passed) or "spent" (its click
// budget is used up). The pill in the list reads this; the redirect itself
// only asks IsExpired, which is true for the last three.
func linkState(l models.Link) string {
	if !l.IsActive {
		return "paused"
	}
	if l.ExpiresAt != nil && *l.ExpiresAt > 0 && time.Now().Unix() >= *l.ExpiresAt {
		return "expired"
	}
	if l.MaxClicks != nil && *l.MaxClicks > 0 && l.ClickCount >= *l.MaxClicks {
		return "spent"
	}
	return "active"
}

// initial is the letter in the avatar: the first character of the name,
// upper-cased, or a question mark for a name that has none.
func initial(s string) string {
	for _, r := range strings.TrimSpace(s) {
		return string(unicode.ToUpper(r))
	}
	return "?"
}

var (
	assetVersionOnce sync.Once
	assetVersion     string
)

// AssetVersion is a short digest of everything under static/, computed once
// per process. It goes into the URLs of the stylesheet and the scripts so
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
