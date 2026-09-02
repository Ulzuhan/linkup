package services

import (
	"bytes"
	"strings"
	"testing"
)

func TestSVGDeQR(t *testing.T) {
	svg, err := SVGDeQR("https://link.example/abc")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	texto := string(svg)
	for _, trozo := range []string{"<svg", "viewBox=", "<path", "shape-rendering"} {
		if !strings.Contains(texto, trozo) {
			t.Errorf("falta %q en el SVG", trozo)
		}
	}
	// Un solo path, no un rect por módulo: es la razón de generarlo en casa.
	if n := strings.Count(texto, "<path"); n != 1 {
		t.Errorf("se esperaba un único <path>, hay %d", n)
	}
	// Fondo blanco explícito: el panel es oscuro y un QR sin fondo no se lee.
	if !strings.Contains(texto, `fill="#ffffff"`) {
		t.Error("el SVG debe traer su propio fondo blanco")
	}
}

func TestPNGDeQR(t *testing.T) {
	png, err := PNGDeQR("https://link.example/abc", 256)
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if !bytes.HasPrefix(png, []byte{0x89, 'P', 'N', 'G'}) {
		t.Error("no es un PNG: falta la firma")
	}
}

func TestQRRechazaContenidoVacio(t *testing.T) {
	if _, err := SVGDeQR(""); err == nil {
		t.Error("un QR sin contenido no es un QR")
	}
	if _, err := PNGDeQR("", 256); err == nil {
		t.Error("ídem en PNG")
	}
}

// El tamaño se acota en los dos extremos: un PNG de 4 px no sirve y uno de
// 20 000 es una forma barata de pedirle mucha memoria al servidor.
func TestPNGAcotaElTamano(t *testing.T) {
	pequeno, err := PNGDeQR("https://x.example/", 1)
	if err != nil {
		t.Fatalf("debería acotar, no fallar: %v", err)
	}
	if len(pequeno) == 0 {
		t.Error("debería haber devuelto una imagen")
	}
	if _, err := PNGDeQR("https://x.example/", 99999); err != nil {
		t.Errorf("debería acotar por arriba, no fallar: %v", err)
	}
}
