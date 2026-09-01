package services

import (
	"net"
	"strings"
	"testing"
)

// This test lives inside the package, unlike the rest of the suite in tests/.
// That is deliberate: the DNS rebinding case can only be exercised by swapping
// the resolver, and exporting a resolver hook just to satisfy a convention
// would widen the surface for no gain.

func TestValidateOutboundURLRejectsReservedRanges(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1:9100/",
		"http://localhost/hook",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://10.0.0.5/",
		"http://192.168.1.10/",
		"http://172.16.4.4/",
		"http://[::1]:8080/",
		"http://0.0.0.0/",
		"ftp://example.com/",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"",
	}
	for _, raw := range blocked {
		if _, err := ValidateOutboundURL(raw); err == nil {
			t.Errorf("expected %q to be refused, it was accepted", raw)
		}
	}
}

func TestValidateOutboundURLAcceptsPublicDestinations(t *testing.T) {
	original := resolveHost
	resolveHost = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	defer func() { resolveHost = original }()

	for _, raw := range []string{"https://example.com/hook", "http://example.com:8080/x?y=1"} {
		if _, err := ValidateOutboundURL(raw); err != nil {
			t.Errorf("expected %q to be accepted, got %v", raw, err)
		}
	}
}

// A name that answers with a private address is the rebinding case: the string
// looks harmless and the destination is not.
func TestValidateOutboundURLRejectsNamesResolvingToPrivate(t *testing.T) {
	original := resolveHost
	resolveHost = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}
	defer func() { resolveHost = original }()

	_, err := ValidateOutboundURL("https://parece-publico.example/hook")
	if err == nil {
		t.Fatal("a name resolving to a private address must be refused")
	}
	if !strings.Contains(err.Error(), "reserved range") {
		t.Errorf("the error should say why it was refused, got: %v", err)
	}
}

// One private answer among several is enough: we do not get to choose which
// address the dialer will pick.
func TestValidateOutboundURLRejectsMixedAnswers(t *testing.T) {
	original := resolveHost
	resolveHost = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("127.0.0.1")}, nil
	}
	defer func() { resolveHost = original }()

	if _, err := ValidateOutboundURL("https://mixto.example/hook"); err == nil {
		t.Fatal("a mixed answer containing a reserved address must be refused")
	}
}

func TestOutboundClientDoesNotFollowRedirects(t *testing.T) {
	client := NewOutboundClient(0)
	if client.CheckRedirect == nil {
		t.Fatal("the outbound client must refuse redirects; a 302 undoes the destination check")
	}
}
