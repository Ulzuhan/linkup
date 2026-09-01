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
	Port            int
	Host            string
	PublicHost      string
	DefaultDomain   string
	SessionSecret   []byte
	OIDCClientID     string
	OIDCClientSecret string
	OIDCIssuerURL   string
	OIDCRedirectURI string
	EnrollURL       string
	AccountURL      string
	DBPath          string
	QRForgeURL      string
	AdminUsers      map[string]bool
	DevMode         bool
}

func Load() *Config {
	port := getEnvAsInt("LINKUP_PORT", 3464)
	host := getEnv("LINKUP_HOST", "0.0.0.0")
	publicHost := getEnv("LINKUP_PUBLIC_HOST", "localhost:3464")
	defaultDomain := getEnv("LINKUP_DEFAULT_DOMAIN", publicHost)
	dbPath := getEnv("LINKUP_DB_PATH", "./data/linkup.db")
	qrForgeURL := getEnv("LINKUP_QRFORGE_URL", "https://qr.kaicorplabs.com")
	devMode := getEnvAsBool("LINKUP_DEV_MODE", false)

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
	oidcIssuerURL := getEnv("LINKUP_OIDC_DISCOVERY_URL", "")
	oidcRedirectURI := getEnv("LINKUP_OIDC_REDIRECT_URI", "http://localhost:3464/auth/callback")
	enrollURL := getEnv("LINKUP_ENROLL_URL", "")
	accountURL := getEnv("LINKUP_ACCOUNT_URL", "")

	// Admin users
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

	return &Config{
		Port:             port,
		Host:             host,
		PublicHost:       publicHost,
		DefaultDomain:    defaultDomain,
		SessionSecret:    sessionSecret,
		OIDCClientID:     oidcClientID,
		OIDCClientSecret: oidcClientSecret,
		OIDCIssuerURL:    oidcIssuerURL,
		OIDCRedirectURI:  oidcRedirectURI,
		EnrollURL:        enrollURL,
		AccountURL:       accountURL,
		DBPath:           dbPath,
		QRForgeURL:       strings.TrimRight(qrForgeURL, "/"),
		AdminUsers:       adminUsersMap,
		DevMode:          devMode,
	}
}

func (c *Config) IsOIDCConfigured() bool {
	return c.OIDCIssuerURL != "" && c.OIDCClientID != ""
}

func (c *Config) IsAdmin(username string) bool {
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
