package services

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Talking to the identity provider by the short way.
//
// THE PROBLEM. Two containers on the same host that speak to each other through
// the public hostname send their traffic out to the internet and back in through
// the tunnel to reach a neighbour. It works, it is slow, and it stops working
// the moment the tunnel does — taking sign-in down for a reason that has nothing
// to do with either service.
//
// THE SHAPE OF THE FIX. Only the server-to-server calls take the short way. The
// browser is still sent to the public address, because that is the only one it
// can reach and because the token's `iss` claim must match what the provider
// publishes. So discovery is fetched over the internal address while the issuer
// is still validated as the public one, and the authorization endpoint is
// rewritten back to the public origin afterwards.

// descubrirProveedor resolves the provider, over internalBase when one is given.
func descubrirProveedor(ctx context.Context, issuer, internalBase string) (*oidc.Provider, error) {
	if internalBase == "" {
		return oidc.NewProvider(ctx, issuer)
	}

	interno, err := enOrigen(issuer, internalBase)
	if err != nil {
		return nil, fmt.Errorf("LINKUP_OIDC_INTERNAL_BASE is not a usable URL: %w", err)
	}
	// Fetch from the internal address, keep validating `iss` as the public one.
	// The token is signed for the public issuer and must stay that way.
	return oidc.NewProvider(oidc.InsecureIssuerURLContext(ctx, issuer), interno)
}

// emisoresAceptados lists the issuer values a token may legitimately carry.
//
// WHY THERE IS MORE THAN ONE, AND WHY IT IS NOT LAXITY. The provider derives the
// issuer from the Host it was asked on. The browser goes to the public address,
// so a token minted there says the public issuer; the code exchange is made by
// this server over the internal address, so that token says the internal one.
// Both are the same provider and both are legitimate — insisting on the public
// value alone rejects the very token we just asked for, which is exactly how
// sign-in broke on 2026-09-02:
//
//	oidc: id token issued by a different provider,
//	expected "https://auth.example.com/application/o/linkup/"
//	got "http://provider-internal:9000/application/o/linkup/"
//
// The alternative — sending the exchange out to the public address — trades a
// correct check for a round trip through the internet to reach a neighbour.
// This keeps the check explicit and the traffic local.
func emisoresAceptados(issuer, internalBase string) []string {
	aceptados := []string{issuer}
	if internalBase == "" {
		return aceptados
	}
	if interno, err := enOrigen(issuer, internalBase); err == nil && interno != issuer {
		aceptados = append(aceptados, interno)
	}
	return aceptados
}

// emisorValido reports whether a token's issuer is one we accept.
func emisorValido(recibido, issuer, internalBase string) bool {
	for _, esperado := range emisoresAceptados(issuer, internalBase) {
		// Providers are inconsistent about the trailing slash; the rest must
		// match exactly.
		if strings.TrimRight(recibido, "/") == strings.TrimRight(esperado, "/") {
			return true
		}
	}
	return false
}

// endpointPublico rewrites the authorization URL back to the public origin.
//
// Discovery answered with internal addresses because that is where it was asked,
// and the browser cannot reach those. The token endpoint is left internal on
// purpose: that call is made by this server, which can.
func endpointPublico(endpoint oauth2.Endpoint, issuer string) (oauth2.Endpoint, error) {
	publico, err := url.Parse(issuer)
	if err != nil {
		return endpoint, err
	}
	reescrito, err := enOrigen(endpoint.AuthURL, publico.Scheme+"://"+publico.Host)
	if err != nil {
		return endpoint, err
	}
	endpoint.AuthURL = reescrito
	return endpoint, nil
}

// enOrigen swaps the origin of a URL while keeping its path.
//
// Host and port are set SEPARATELY, never through Host: the setter for Host
// keeps the existing port when the new value carries none, which turns
// http://provider-internal:9000/x into https://public:9000/x — the internal
// port smuggled into the address the browser is sent to.
func enOrigen(destino, origen string) (string, error) {
	u, err := url.Parse(destino)
	if err != nil {
		return "", err
	}
	o, err := url.Parse(origen)
	if err != nil {
		return "", err
	}
	if o.Host == "" {
		return "", fmt.Errorf("origin %q has no host", origen)
	}
	u.Scheme = o.Scheme
	u.Host = o.Hostname()
	if o.Port() != "" {
		u.Host = o.Hostname() + ":" + o.Port()
	}
	return u.String(), nil
}
