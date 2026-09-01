package services

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Outbound URL validation.
//
// WHY THIS EXISTS. Webhook targets are chosen by users and delivered by the
// server, which makes them a server-side request forgery surface: without this,
// a webhook pointing at http://127.0.0.1:9100/ or at the cloud metadata address
// makes LinkUp itself issue that request from inside the private network. HMAC
// signing does not help — it authenticates the receiver, it does not control
// where we are willing to talk to.
//
// TWO PLACES, NOT ONE. Validation runs when the URL is stored and again right
// before the request is made. A hostname that resolved to a public address at
// creation time can resolve to a private one later; checking only once leaves
// DNS rebinding wide open.

var (
	// ErrInvalidOutboundURL is returned for anything we refuse to call.
	ErrInvalidOutboundURL = errors.New("invalid outbound URL")

	// Reserved ranges that must never be reachable from a user-supplied URL.
	// 169.254.0.0/16 covers cloud metadata endpoints, which is the single most
	// valuable target an SSRF can reach.
	blockedCIDRs = []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	blockedNets []*net.IPNet
)

func init() {
	for _, cidr := range blockedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("egress: bad CIDR in blocklist: " + cidr)
		}
		blockedNets = append(blockedNets, network)
	}
}

// resolveHost is swappable so tests can exercise the rebinding case without
// depending on real DNS.
var resolveHost = net.LookupIP

// IsBlockedIP reports whether an address belongs to a range we refuse to call.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, network := range blockedNets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateOutboundURL parses a user-supplied URL and refuses anything we are
// not willing to send a request to. It resolves the hostname and checks every
// address it answers with: a name is not a destination, an address is.
func ValidateOutboundURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty URL", ErrInvalidOutboundURL)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidOutboundURL, err)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, fmt.Errorf("%w: scheme %q is not allowed, use http or https",
			ErrInvalidOutboundURL, parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: missing hostname", ErrInvalidOutboundURL)
	}
	if strings.EqualFold(host, "localhost") {
		return nil, fmt.Errorf("%w: localhost is not a valid destination", ErrInvalidOutboundURL)
	}

	// A literal address needs no lookup; a name does. Either way the decision
	// is made on addresses, never on the string.
	if literal := net.ParseIP(host); literal != nil {
		if IsBlockedIP(literal) {
			return nil, fmt.Errorf("%w: address %s is in a reserved range",
				ErrInvalidOutboundURL, literal)
		}
		return parsed, nil
	}

	addrs, err := resolveHost(host)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot resolve %q", ErrInvalidOutboundURL, host)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: %q resolves to nothing", ErrInvalidOutboundURL, host)
	}
	// Every address must be acceptable. One private answer is enough to refuse:
	// we cannot choose which one the dialer will pick.
	for _, ip := range addrs {
		if IsBlockedIP(ip) {
			return nil, fmt.Errorf("%w: %q resolves to %s, which is in a reserved range",
				ErrInvalidOutboundURL, host, ip)
		}
	}
	return parsed, nil
}

// NewOutboundClient builds the only HTTP client allowed to call user-supplied
// URLs.
//
// Redirects are NOT followed. The default client follows them, and that undoes
// the validation above in one hop: a public host answering 302 to
// http://169.254.169.254/ would take us exactly where we refused to go.
func NewOutboundClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
