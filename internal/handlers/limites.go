package handlers

import (
	"net/http"

	"github.com/Ulzuhan/linkup/internal/services"
)

// WriteRateLimit throttles everything that writes, keyed by the caller's
// identity.
//
// Keyed by identity and not by address on purpose: LinkUp promises never to
// look at a visitor's IP, and a limiter that quietly breaks that promise would
// make the promise worthless. Every write already requires a session or an API
// key, so the identity is there for the taking without profiling anyone.
//
// Reads pass through untouched, and so do unauthenticated requests — those are
// about to be rejected by the handler anyway, and spending a token on them
// would let an anonymous caller drain someone else's budget.
func WriteRateLimit(
	bucket *services.TokenBucket,
	authService *services.AuthService,
	apiKeyService *services.APIKeyService,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			session := getAuthSession(r, authService, apiKeyService)
			if session == nil {
				next.ServeHTTP(w, r)
				return
			}

			if !bucket.Allow(session.Username) {
				w.Header().Set("Retry-After", "60")
				sendJSON(w, http.StatusTooManyRequests, map[string]string{
					"error": "Too many writes. Slow down and try again shortly.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
