package services

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Known tracking and surveillance query parameters
var knownTrackerParams = map[string]bool{
	// Google Analytics / Urchin
	"utm_source":           true,
	"utm_medium":           true,
	"utm_campaign":         true,
	"utm_term":             true,
	"utm_content":          true,
	"utm_id":               true,
	"utm_source_platform":  true,
	"utm_creative_format":  true,
	"utm_marketing_tactic": true,
	"utm_reader":           true,
	"utm_name":             true,
	"utm_cid":              true,
	"utm_viz_id":           true,
	"utm_pubreferrer":      true,
	"utm_swu":              true,

	// Google Ads / DoubleClick
	"gclid":      true,
	"gclsrc":     true,
	"dclid":      true,
	"gad_source": true,
	"gbraid":     true,
	"wbraid":     true,
	"_ga":        true,
	"_gl":        true,

	// Meta / Facebook / Instagram
	"fbclid":            true,
	"fbadid":            true,
	"fb_action_ids":     true,
	"fb_action_types":   true,
	"fb_source":         true,
	"fb_ref":            true,
	"action_object_map": true,
	"igshid":            true,

	// Microsoft / Bing
	"msclkid": true,

	// Twitter / X
	"twclid": true,

	// TikTok
	"ttclid": true,

	// LinkedIn
	"li_fat_id": true,

	// Pinterest
	"epik": true,

	// Yandex
	"yclid":     true,
	"ym_debug":  true,
	"_openstat": true,

	// Mailchimp
	"mc_cid": true,
	"mc_eid": true,

	// HubSpot
	"_hsenc":  true,
	"_hsmi":   true,
	"hsa_cam": true,
	"hsa_grp": true,
	"hsa_mt":  true,
	"hsa_src": true,
	"hsa_ad":  true,
	"hsa_acc": true,
	"hsa_net": true,
	"hsa_kw":  true,
	"hsa_tgt": true,
	"hsa_ol":  true,

	// Marketo / Pardot / Vero / Klaviyo
	"mkt_tok":   true,
	"vero_id":   true,
	"vero_conv": true,
	"_kx":       true,
	"wickedid":  true,

	// Share Telemetry (Spotify, YouTube, etc.)
	"si":      true, // Spotify/YouTube share identifier
	"feature": true, // YouTube feature share tracker
}

// CleanURL sanitizes an input URL by removing tracking query parameters and validating protocols.
func CleanURL(rawURL string, ownHost string) (cleanURL string, strippedParams []string, err error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", nil, fmt.Errorf("URL cannot be empty")
	}

	// Auto-prepend https:// if protocol is missing
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		if strings.Contains(trimmed, "://") {
			return "", nil, fmt.Errorf("invalid URL protocol: only http and https are permitted")
		}
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", nil, fmt.Errorf("invalid URL format: %w", err)
	}

	// Validate scheme
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", nil, fmt.Errorf("disallowed scheme '%s': only http and https are allowed", parsed.Scheme)
	}

	// Validate host
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", nil, fmt.Errorf("URL must include a valid hostname")
	}

	// Prevent loop back to our own public host
	if ownHost != "" {
		cleanOwnHost := strings.ToLower(strings.Split(ownHost, ":")[0])
		if host == cleanOwnHost || host == "127.0.0.1" || host == "localhost" {
			// In dev mode allow testing, but warn or prevent self-referencing redirect loops
			if host == cleanOwnHost {
				return "", nil, fmt.Errorf("destination cannot point to LinkUp itself (prevents redirection loops)")
			}
		}
	}

	// Parse query parameters
	queryParams := parsed.Query()
	var removed []string

	for param := range queryParams {
		paramLower := strings.ToLower(param)
		if isTrackerParam(paramLower) {
			removed = append(removed, param)
			queryParams.Del(param)
		}
	}

	// Sort removed params for deterministic output
	sort.Strings(removed)

	// Reconstruct query string
	parsed.RawQuery = queryParams.Encode()

	return parsed.String(), removed, nil
}

func isTrackerParam(key string) bool {
	if knownTrackerParams[key] {
		return true
	}
	// Prefix checks for utm_, hsa_, etc.
	if strings.HasPrefix(key, "utm_") || strings.HasPrefix(key, "hsa_") {
		return true
	}
	// Amazon / e-commerce click telemetry
	if strings.HasPrefix(key, "ref_") || strings.HasPrefix(key, "pf_rd_") || strings.HasPrefix(key, "pd_rd_") {
		return true
	}
	return false
}
