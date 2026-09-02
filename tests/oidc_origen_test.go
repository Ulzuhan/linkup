package tests

import (
	"os"
	"testing"

	"github.com/Ulzuhan/linkup/internal/config"
)

// The discovery URL is the issuer, not the document. go-oidc appends
// /.well-known/openid-configuration itself, so handing it the full URL asks the
// provider for that path twice and sign-in fails at the first step. Both names
// are accepted and the suffix is trimmed, because the old variable was called
// DISCOVERY_URL and every example showed exactly the wrong value.

func TestIssuerIsNormalisedWhateverTheVariableIsCalled(t *testing.T) {
	const want = "https://auth.example.com/application/o/linkup/"

	cases := []struct {
		name, variable, value string
	}{
		{"issuer, new name", "LINKUP_OIDC_ISSUER", want},
		{"full discovery URL, new name", "LINKUP_OIDC_ISSUER", want + ".well-known/openid-configuration"},
		{"full discovery URL, old name", "LINKUP_OIDC_DISCOVERY_URL", want + ".well-known/openid-configuration"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Unsetenv("LINKUP_OIDC_ISSUER")
			os.Unsetenv("LINKUP_OIDC_DISCOVERY_URL")
			t.Setenv(tc.variable, tc.value)
			t.Setenv("LINKUP_HOST", "127.0.0.1")

			if got := config.Load().OIDCIssuerURL; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestInternalBaseIsOptionalAndTrimmed(t *testing.T) {
	t.Setenv("LINKUP_HOST", "127.0.0.1")

	os.Unsetenv("LINKUP_OIDC_INTERNAL_BASE")
	if got := config.Load().OIDCInternalBase; got != "" {
		t.Errorf("without the variable it must be empty, got %q", got)
	}

	t.Setenv("LINKUP_OIDC_INTERNAL_BASE", "http://authentik-server:9000/")
	if got := config.Load().OIDCInternalBase; got != "http://authentik-server:9000" {
		t.Errorf("the trailing slash should be trimmed, got %q", got)
	}
}
