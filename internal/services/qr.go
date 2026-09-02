package services

import (
	"bytes"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// El código QR, dibujado aquí.
//
// POR QUÉ EN CASA. La vista previa del panel lo pedía a api.qrserver.com con la
// URL corta dentro de la query: un tercero enterándose de qué enlaces existen y
// a dónde apuntan, en un producto cuyo argumento entero es que no habla con
// nadie. Se quitó, y esto es lo que ocupa su sitio.
//
// La dependencia es Go puro y sin CGO, así que el binario sigue siendo estático.
// Sólo se le pide la matriz de módulos; el SVG lo construimos nosotros porque el
// suyo es una malla de <rect>, uno por módulo, y eso son miles de nodos para lo
// que cabe en un solo <path>.

// Cuatro módulos de zona de silencio: es lo que exige la especificación para
// que un lector encuentre el código, y omitirla es la causa habitual de un QR
// que no escanea pegado a un borde.
const zonaDeSilencio = 4

// nivelCorreccion medio: aguanta un 15 % de daño. Alto engorda la matriz y
// bajo no sobrevive a una impresión mediocre.
const nivelCorreccion = qrcode.Medium

// SVGDeQR devuelve el código como SVG, en módulos y sin unidades: quien lo
// pinta decide el tamaño, y no hay pérdida al ampliarlo.
func SVGDeQR(contenido string) ([]byte, error) {
	if contenido == "" {
		return nil, fmt.Errorf("nothing to encode")
	}
	codigo, err := qrcode.New(contenido, nivelCorreccion)
	if err != nil {
		return nil, fmt.Errorf("could not build the QR: %w", err)
	}

	matriz := codigo.Bitmap()
	// Bitmap ya viene con su propio borde; se recorta para poner el nuestro y
	// que el tamaño no dependa de lo que decida la librería.
	borde := 4
	lado := len(matriz) - 2*borde
	if lado < 1 {
		return nil, fmt.Errorf("unexpected QR matrix")
	}
	total := lado + 2*zonaDeSilencio

	// Un solo <path>: cada módulo es un "M x y h1 v1 h-1 z". Con <rect> por
	// módulo un QR corriente pasa de mil nodos y el navegador lo nota.
	var d bytes.Buffer
	for y := 0; y < lado; y++ {
		for x := 0; x < lado; x++ {
			if matriz[y+borde][x+borde] {
				fmt.Fprintf(&d, "M%d %dh1v1h-1z", x+zonaDeSilencio, y+zonaDeSilencio)
			}
		}
	}

	var svg bytes.Buffer
	fmt.Fprintf(&svg,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="QR code">`,
		total, total)
	// El fondo blanco va dentro del propio SVG: un QR sobre fondo oscuro no lo
	// lee ningún teléfono, y el panel es oscuro.
	fmt.Fprintf(&svg, `<rect width="%d" height="%d" fill="#ffffff"/>`, total, total)
	fmt.Fprintf(&svg, `<path d="%s" fill="#000000"/></svg>`, d.String())
	return svg.Bytes(), nil
}

// PNGDeQR devuelve el código como PNG del ancho pedido, para descargar.
func PNGDeQR(contenido string, px int) ([]byte, error) {
	if contenido == "" {
		return nil, fmt.Errorf("nothing to encode")
	}
	if px < 64 {
		px = 64
	}
	if px > 2048 {
		px = 2048
	}
	return qrcode.Encode(contenido, nivelCorreccion, px)
}
