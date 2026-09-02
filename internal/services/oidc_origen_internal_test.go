package services

import "testing"

// The regression test for the sign-in that broke on 2026-09-02:
//
//	oidc: id token issued by a different provider,
//	expected "https://auth.kaicorplabs.com/application/o/linkup/"
//	got "http://authentik-server:9000/application/o/linkup/"
//
// The provider derives the issuer from the Host it was asked on. The browser
// goes to the public address and the code exchange goes over the internal one,
// so both values are legitimate and both must be accepted — while everything
// else must still be refused, because skipping the library's check without
// replacing it would accept any issuer at all.

const (
	emisorPublico = "https://auth.example.com/application/o/linkup/"
	baseInterna   = "http://provider-internal:9000"
	emisorInterno = "http://provider-internal:9000/application/o/linkup/"
)

func TestAmbosEmisoresLegitimosSeAceptan(t *testing.T) {
	for _, recibido := range []string{emisorPublico, emisorInterno} {
		if !emisorValido(recibido, emisorPublico, baseInterna) {
			t.Errorf("%q debería aceptarse: es el mismo proveedor por otra puerta", recibido)
		}
	}
	// La barra final varía entre proveedores; el resto no.
	if !emisorValido("https://auth.example.com/application/o/linkup", emisorPublico, baseInterna) {
		t.Error("una barra final de más o de menos no debería rechazar el token")
	}
}

func TestCualquierOtroEmisorSeRechaza(t *testing.T) {
	ajenos := []string{
		"https://malo.example.com/application/o/linkup/",
		"http://provider-internal:9001/application/o/linkup/",
		"https://auth.example.com/application/o/otra-app/",
		"",
	}
	for _, recibido := range ajenos {
		if emisorValido(recibido, emisorPublico, baseInterna) {
			t.Errorf("%q NO debería aceptarse", recibido)
		}
	}
}

// Sin vía interna hay un solo emisor válido, y el interno deja de serlo.
func TestSinViaInternaSoloValeElPublico(t *testing.T) {
	if emisorValido(emisorInterno, emisorPublico, "") {
		t.Error("sin LINKUP_OIDC_INTERNAL_BASE el emisor interno no es legítimo")
	}
	if !emisorValido(emisorPublico, emisorPublico, "") {
		t.Error("el público siempre vale")
	}
}
