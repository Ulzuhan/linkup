package services

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"strings"

	"github.com/Ulzuhan/linkup/internal/models"
)

// RouterEngine handles conditional routing (Device, Locale, A/B Testing) in-memory
type RouterEngine struct{}

func NewRouterEngine() *RouterEngine {
	return &RouterEngine{}
}

// ResolveDestination evaluates conditional rules in order and returns the winning target URL
func (e *RouterEngine) ResolveDestination(r *http.Request, link *models.Link) (finalURL string, variantName string) {
	// 1. A/B Testing Evaluation (Priority 1 if configured)
	if len(link.ABVariants) > 0 {
		varName, abURL, ok := e.EvaluateABTest(link)
		if ok && abURL != "" {
			return abURL, varName
		}
	}

	// 2. Device Routing Evaluation
	userAgent := r.Header.Get("User-Agent")
	if userAgent != "" {
		deviceURL, ok := e.EvaluateDevice(userAgent, link)
		if ok && deviceURL != "" {
			return deviceURL, ""
		}
	}

	// 3. Locale / Language Routing Evaluation
	acceptLang := r.Header.Get("Accept-Language")
	if acceptLang != "" && len(link.LocaleRouting) > 0 {
		localeURL, ok := e.EvaluateLocale(acceptLang, link)
		if ok && localeURL != "" {
			return localeURL, ""
		}
	}

	// 4. Default Fallback
	return link.TargetURL, ""
}

// EvaluateDevice checks User-Agent for iOS or Android
func (e *RouterEngine) EvaluateDevice(userAgent string, link *models.Link) (string, bool) {
	uaLower := strings.ToLower(userAgent)

	if strings.Contains(uaLower, "iphone") || strings.Contains(uaLower, "ipad") || strings.Contains(uaLower, "ipod") {
		if link.IOSURL != "" {
			return link.IOSURL, true
		}
	}

	if strings.Contains(uaLower, "android") {
		if link.AndroidURL != "" {
			return link.AndroidURL, true
		}
	}

	return "", false
}

// EvaluateLocale parses Accept-Language header to find matching language rule
func (e *RouterEngine) EvaluateLocale(acceptLanguage string, link *models.Link) (string, bool) {
	if len(link.LocaleRouting) == 0 || acceptLanguage == "" {
		return "", false
	}

	// Example: "es-ES,es;q=0.9,en;q=0.8,fr;q=0.5"
	parts := strings.Split(acceptLanguage, ",")
	for _, part := range parts {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		if tag == "" {
			continue
		}

		tagLower := strings.ToLower(tag)

		// 1. Exact match (e.g. "es-es")
		if dest, ok := link.LocaleRouting[tagLower]; ok && dest != "" {
			return dest, true
		}

		// 2. Base language match (e.g. "es" from "es-es")
		baseLang := strings.Split(tagLower, "-")[0]
		if dest, ok := link.LocaleRouting[baseLang]; ok && dest != "" {
			return dest, true
		}
	}

	return "", false
}

// EvaluateABTest selects a variant based on configured weights
func (e *RouterEngine) EvaluateABTest(link *models.Link) (string, string, bool) {
	if len(link.ABVariants) == 0 {
		return "", "", false
	}

	totalWeight := 0
	for _, v := range link.ABVariants {
		if v.Weight > 0 {
			totalWeight += v.Weight
		}
	}

	if totalWeight <= 0 {
		// Equal distribution fallback if weights are not set
		n := len(link.ABVariants)
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(n)))
		v := link.ABVariants[idx.Int64()]
		return v.Name, v.TargetURL, true
	}

	randValBig, err := rand.Int(rand.Reader, big.NewInt(int64(totalWeight)))
	if err != nil {
		return link.ABVariants[0].Name, link.ABVariants[0].TargetURL, true
	}

	randVal := int(randValBig.Int64())
	current := 0
	for _, v := range link.ABVariants {
		current += v.Weight
		if randVal < current {
			return v.Name, v.TargetURL, true
		}
	}

	last := link.ABVariants[len(link.ABVariants)-1]
	return last.Name, last.TargetURL, true
}
