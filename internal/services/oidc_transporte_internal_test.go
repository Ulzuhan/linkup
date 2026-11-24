package services

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func pad32(b []byte) []byte {
	if len(b) >= 32 {
		return b[len(b)-32:]
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func signES256Test(t *testing.T, key *ecdsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	hdr, err := json.Marshal(map[string]any{"alg": "ES256", "kid": "k1", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	pay, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	input := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pay)
	sum := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := append(pad32(r.Bytes()), pad32(s.Bytes())...)
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestClienteInternoOIDCRoutesServerCallsThroughInternalBase(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const publicIssuer = "https://issuer.invalid/auth/v1"
	var hits []string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v1/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                publicIssuer,
				"authorization_endpoint":                publicIssuer + "/oauth/authorize",
				"token_endpoint":                        publicIssuer + "/oauth/token",
				"userinfo_endpoint":                     publicIssuer + "/oauth/userinfo",
				"jwks_uri":                              publicIssuer + "/.well-known/jwks.json",
				"id_token_signing_alg_values_supported": []string{"RS256", "ES256"},
			})
		case "/auth/v1/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{{
					"kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig", "kid": "k1",
					"x": base64.RawURLEncoding.EncodeToString(pad32(key.PublicKey.X.Bytes())),
					"y": base64.RawURLEncoding.EncodeToString(pad32(key.PublicKey.Y.Bytes())),
				}},
			})
		case "/auth/v1/oauth/token":
			now := time.Now().Unix()
			idTok := signES256Test(t, key, map[string]any{
				"iss": publicIssuer, "sub": "u-1", "aud": "cid-1", "exp": now + 300, "iat": now,
			})
			accTok := signES256Test(t, key, map[string]any{
				"iss": publicIssuer, "sub": "u-1", "aud": "authenticated", "groups": []string{"linkup"}, "exp": now + 300, "iat": now,
			})
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": accTok, "token_type": "Bearer", "expires_in": 300, "id_token": idTok,
			})
		case "/auth/v1/oauth/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "u-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := clienteInternoOIDC(publicIssuer, srv.URL)
	if err != nil || client == nil {
		t.Fatalf("clienteInternoOIDC failed: %v", err)
	}
	ctx := oidc.ClientContext(context.Background(), client)
	provider, err := descubrirProveedor(ctx, publicIssuer, srv.URL)
	if err != nil {
		t.Fatalf("descubrirProveedor: %v", err)
	}
	ep, err := endpointPublico(provider.Endpoint(), publicIssuer)
	if err != nil {
		t.Fatalf("endpointPublico: %v", err)
	}
	if ep.AuthURL != publicIssuer+"/oauth/authorize" {
		t.Fatalf("unexpected AuthURL: %q", ep.AuthURL)
	}
	ocfg := &oauth2.Config{ClientID: "cid-1", ClientSecret: "sec-1", RedirectURL: "https://link.invalid/auth/callback", Endpoint: ep}
	tok, err := ocfg.Exchange(ctx, "code-1")
	if err != nil {
		t.Fatalf("Exchange via internalBase failed: %v", err)
	}
	rawID, _ := tok.Extra("id_token").(string)
	verifier := provider.Verifier(&oidc.Config{ClientID: "cid-1", SkipIssuerCheck: true, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256}})
	if _, err := verifier.Verify(ctx, rawID); err != nil {
		t.Fatalf("Verify ES256 via internalBase failed: %v", err)
	}
	if _, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err != nil {
		t.Fatalf("UserInfo via internalBase failed: %v", err)
	}
	joined := strings.Join(hits, ",")
	for _, want := range []string{"openid-configuration", "/oauth/token", "jwks.json", "/oauth/userinfo"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing internal hit for %s; hits=%v", want, hits)
		}
	}
}
