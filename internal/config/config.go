package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                int
	Host                string
	PublicHost          string
	DefaultDomain       string
	SessionSecret       []byte
	OIDCClientID        string
	OIDCClientSecret    string
	OIDCIssuerURL       string
	OIDCInternalBase    string
	OIDCProviderName    string
	OIDCRedirectURI     string
	EnrollURL           string
	AccountURL          string
	DBPath              string
	QRForgeURL          string
	AdminUsers          map[string]bool
	AdminGroup          string
	DevMode             bool
	AllowPrivateTargets bool
	// Whether the footer links to the other tools of the house.
	FooterLinks bool
}

func Load() *Config {
	port := getEnvAsInt("LINKUP_PORT", 3464)
	host := getEnv("LINKUP_HOST", "0.0.0.0")
	publicHost := getEnv("LINKUP_PUBLIC_HOST", "localhost:3464")
	defaultDomain := getEnv("LINKUP_DEFAULT_DOMAIN", publicHost)
	dbPath := getEnv("LINKUP_DB_PATH", "./data/linkup.db")
	qrForgeURL := getEnv("LINKUP_QRFORGE_URL", "https://qr.kaicorplabs.com")
	devMode := getEnvAsBool("LINKUP_DEV_MODE", false)

	// Shortening an intranet URL is a legitimate thing for a self-hosted tool to
	// do — someone may well want link.example.com/wiki to point at 10.0.0.5. But
	// it is also how a shortener becomes a probe for other people's networks, so
	// the default is no and the exception is deliberate. Webhooks are NOT covered
	// by this: those are requests the server itself makes, and there the answer is
	// always no.
	allowPrivateTargets := getEnvAsBool("LINKUP_ALLOW_PRIVATE_TARGETS", false)

	// Session Secret
	secretStr := os.Getenv("LINKUP_SESSION_SECRET")
	var sessionSecret []byte
	if secretStr == "" {
		if !devMode {
			log.Println("[WARN] LINKUP_SESSION_SECRET is not set. Generating an ephemeral 32-byte secret (sessions will reset on restart).")
		}
		sessionSecret = make([]byte, 32)
		_, _ = rand.Read(sessionSecret)
	} else {
		sessionSecret = []byte(secretStr)
		if len(sessionSecret) < 32 {
			// Pad or hash to 32 bytes minimum for AES-GCM / HMAC
			padded := make([]byte, 32)
			copy(padded, sessionSecret)
			sessionSecret = padded
		}
	}

	// OIDC settings
	oidcClientID := getEnv("LINKUP_OIDC_CLIENT_ID", "")
	oidcClientSecret := getEnv("LINKUP_OIDC_CLIENT_SECRET", "")
	// The ISSUER, not the discovery document.
	//
	// The library appends /.well-known/openid-configuration itself, so handing
	// it the full discovery URL produces a request for
	// …/.well-known/openid-configuration/.well-known/openid-configuration and a
	// sign-in that fails at the first step. The old variable name said
	// DISCOVERY_URL and the examples showed exactly that, so both are accepted
	// and the suffix is trimmed rather than left to bite someone.
	oidcIssuerURL := getEnv("LINKUP_OIDC_ISSUER", getEnv("LINKUP_OIDC_DISCOVERY_URL", ""))
	oidcIssuerURL = recortarDiscovery(strings.TrimSpace(oidcIssuerURL))

	// How THIS server reaches the provider, when that is not the public address.
	// Two containers on the same host talking through the public hostname go out
	// to the internet and come back through the tunnel to reach a neighbour, and
	// stop working the moment that tunnel does. The browser still gets sent to
	// the public address; only the server-to-server calls take the short way.
	oidcInternalBase := strings.TrimRight(getEnv("LINKUP_OIDC_INTERNAL_BASE", ""), "/")

	// What the sign-in button calls the provider.
	//
	// It used to say "Login with Authentik" in the template, which is fine for
	// us and wrong for everyone else: this is an MIT product people run against
	// Keycloak, Zitadel or Entra, and a button naming somebody else's software
	// is the same mistake as a hardcoded provider URL. The default says nothing
	// about who.
	providerName := strings.TrimSpace(getEnv("LINKUP_OIDC_PROVIDER_NAME", "your provider"))
	oidcRedirectURI := getEnv("LINKUP_OIDC_REDIRECT_URI", "http://localhost:3464/auth/callback")
	enrollURL := getEnv("LINKUP_ENROLL_URL", "")
	accountURL := getEnv("LINKUP_ACCOUNT_URL", "")

	// Whether the footer links to the other KaiCorp Labs tools. Off unless the
	// operator asks for it: in somebody else's deployment those links are
	// advertising for services of ours. Same variable, same meaning, as in the
	// five sibling applications.
	footerLinks := strings.TrimSpace(getEnv("KAICORP_FOOTER_LINKS", "")) != ""

	// Who administers, decided by the identity provider.
	//
	// A group beats a list of usernames for one reason that matters in
	// practice: revoking becomes removing someone from a group, not editing a
	// file and restarting the service. LINKUP_ADMIN_USERS stays as a fallback
	// for deployments whose provider does not emit groups, and it is checked
	// only when no group is configured.
	adminGroup := getEnv("LINKUP_ADMIN_GROUP", "")

	// Admin users (fallback)
	adminUsersMap := make(map[string]bool)
	adminUsersStr := getEnv("LINKUP_ADMIN_USERS", "")
	if adminUsersStr != "" {
		for _, u := range strings.Split(adminUsersStr, ",") {
			cleaned := strings.TrimSpace(u)
			if cleaned != "" {
				adminUsersMap[cleaned] = true
			}
		}
	}

	cfg := &Config{
		Port:                port,
		Host:                host,
		PublicHost:          publicHost,
		DefaultDomain:       defaultDomain,
		SessionSecret:       sessionSecret,
		OIDCClientID:        oidcClientID,
		OIDCClientSecret:    oidcClientSecret,
		OIDCIssuerURL:       oidcIssuerURL,
		OIDCInternalBase:    oidcInternalBase,
		OIDCProviderName:    providerName,
		OIDCRedirectURI:     oidcRedirectURI,
		EnrollURL:           enrollURL,
		AccountURL:          accountURL,
		FooterLinks:         footerLinks,
		DBPath:              dbPath,
		QRForgeURL:          strings.TrimRight(qrForgeURL, "/"),
		AdminUsers:          adminUsersMap,
		AdminGroup:          adminGroup,
		DevMode:             devMode,
		AllowPrivateTargets: allowPrivateTargets,
	}

	cfg.refuseUnsafeDevMode()
	return cfg
}

// refuseUnsafeDevMode aborts startup rather than serving an open panel.
//
// WHY THIS IS FATAL AND NOT A WARNING. In development mode with OIDC absent,
// AuthService.GetSession hands a session with IsAdmin=true to anyone arriving
// without a cookie. That is fine on a laptop and catastrophic anywhere else,
// and the way it gets anywhere else is never a decision — it is a forgotten
// variable, or a secrets file that failed to load and took the OIDC settings
// with it. A warning in a log nobody reads does not stop that; refusing to
// start does.
//
// Better a door believed open that is shut, than the other way round.
func (c *Config) refuseUnsafeDevMode() {
	if !c.DevMode || c.IsOIDCConfigured() {
		return
	}
	if isLoopbackHost(c.Host) {
		return
	}
	log.Fatalf("[FATAL] LINKUP_DEV_MODE is on, OIDC is not configured and the server "+
		"would listen on %q. In that combination every request without a cookie is "+
		"treated as an administrator. Refusing to start. Bind to 127.0.0.1 for local "+
		"development, or configure OIDC.", c.Host)
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	}
	return false
}

// recortarDiscovery removes the well-known suffix if someone pasted the whole
// discovery URL, and touches nothing else.
//
// The trailing slash is left exactly as it was found, deliberately: the `iss`
// claim has to match what the provider publishes character for character, and
// Authentik publishes .../application/o/<app>/ with it. Tidying that away would
// turn a working configuration into "issuer did not match".
func recortarDiscovery(raw string) string {
	const sufijo = ".well-known/openid-configuration"
	recortado := strings.TrimSuffix(raw, "/")
	if strings.HasSuffix(recortado, sufijo) {
		return strings.TrimSuffix(recortado, sufijo)
	}
	return raw
}

func (c *Config) IsOIDCConfigured() bool {
	return c.OIDCIssuerURL != "" && c.OIDCClientID != ""
}

// IsAdmin resolves administration from the provider's groups when a group is
// configured, and only otherwise from the username fallback. The two are never
// combined: mixing them would mean removing someone from the group did not
// actually remove them, which is exactly the failure a group is meant to avoid.
func (c *Config) IsAdmin(username string, groups []string) bool {
	if c.AdminGroup != "" {
		for _, g := range groups {
			if strings.EqualFold(strings.TrimSpace(g), c.AdminGroup) {
				return true
			}
		}
		return false
	}
	if len(c.AdminUsers) == 0 {
		return false
	}
	return c.AdminUsers[username]
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}

func getEnvAsBool(key string, fallback bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return fallback
	}
	return val
}

// GenerateRandomKey generates a cryptographic hex string
func GenerateRandomKey(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
