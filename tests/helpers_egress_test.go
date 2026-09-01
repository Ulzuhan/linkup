package tests

import "net/http"

// nethttpHandler wraps a bare callback as an http.Handler, so the egress test
// can tell whether a request ever arrived without pulling in a mock framework.
func nethttpHandler(onRequest func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		onRequest()
		w.WriteHeader(http.StatusOK)
	})
}
