package tests

import (
	"bytes"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ulzuhan/linkup/internal/models"
)

// El QR se dibuja en este servidor. Lo que estas pruebas fijan no es que la
// imagen sea bonita —eso lo cubre el test unitario del servicio— sino que la
// ruta existe, que devuelve lo que dice devolver, y que no se le da a quien no
// es el dueño: un QR es una foto de un destino privado, y regalarlo es regalar
// el destino.

func TestQRDeUnEnlace(t *testing.T) {
	router, links, _, cleanup := setupTestServer(t)
	defer cleanup()

	creado, _, err := links.Create(models.CreateLinkRequest{
		URL:        "https://example.com/destino",
		CustomSlug: "qr-prueba",
	}, "dev-user")
	if err != nil {
		t.Fatalf("no se pudo crear el enlace: %v", err)
	}

	t.Run("svg", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/links/"+creado.ID+"/qr.svg", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Errorf("Content-Type %q", ct)
		}
		if !strings.Contains(rec.Body.String(), "<svg") {
			t.Error("el cuerpo no parece un SVG")
		}
		// Privado, no público: es el destino de alguien.
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "private") {
			t.Errorf("Cache-Control %q debería ser privado", cc)
		}
	})

	t.Run("png", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/links/"+creado.ID+"/qr.png", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("Content-Type %q", ct)
		}
		if !bytes.HasPrefix(rec.Body.Bytes(), []byte{0x89, 'P', 'N', 'G'}) {
			t.Error("no es un PNG: falta la firma")
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "qr-prueba.png") {
			t.Errorf("debería descargarse con el nombre del slug, dio %q", cd)
		}
	})

	t.Run("un id que no existe", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/links/no-existe/qr.svg", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("got %d, want 404", rec.Code)
		}
	})
}

// La vista previa manda a QR-Forge con la intención, no con un parámetro suelto.
// Y con la URL CORTA: un QR del destino se saltaría a LinkUp, no contaría el
// clic y dejaría de poder cambiarse — que es justo lo que el enlace ofrece.
func TestLaVistaPreviaLlevaLaIntencion(t *testing.T) {
	router, links, _, cleanup := setupTestServer(t)
	defer cleanup()

	if _, _, err := links.Create(models.CreateLinkRequest{
		URL:        "https://example.com/destino",
		CustomSlug: "intencion",
		Title:      "Un título",
	}, "dev-user"); err != nil {
		t.Fatalf("no se pudo crear el enlace: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/preview/intencion", nil))
	cuerpo := rec.Body.String()

	// Sobre el HTML DESESCAPADO, que es lo que el navegador va a seguir: la
	// plantilla escapa `&` como `&amp;` y `+` como `&#43;`, y comprobarlo sobre
	// el texto crudo mediría el escapado en vez del enlace.
	enlace := html.UnescapeString(cuerpo)

	for _, trozo := range []string{"/new?url=", "from=linkup", "title=Un+t%C3%ADtulo"} {
		if !strings.Contains(enlace, trozo) {
			t.Errorf("falta %q en la vista previa", trozo)
		}
	}
	if strings.Contains(enlace, "?text=") {
		t.Error("sigue el formato viejo del enlace a QR-Forge")
	}
	// El destino NO debe ir codificado en el QR de QR-Forge.
	if strings.Contains(enlace, "url=https%3A%2F%2Fexample.com%2Fdestino") {
		t.Error("manda el destino en vez del enlace corto: el QR se saltaría LinkUp")
	}
}

func TestElPanelPideSuPropioQR(t *testing.T) {
	router, links, _, cleanup := setupTestServer(t)
	defer cleanup()

	if _, _, err := links.Create(models.CreateLinkRequest{
		URL:        "https://example.com/x",
		CustomSlug: "panel-qr",
	}, "dev-user"); err != nil {
		t.Fatalf("no se pudo crear el enlace: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	cuerpo := rec.Body.String()

	for _, trozo := range []string{`id="qr-image"`, `id="qr-png"`, "data-id="} {
		if !strings.Contains(cuerpo, trozo) {
			t.Errorf("falta %q en el panel", trozo)
		}
	}
}
