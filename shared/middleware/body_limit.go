package middleware

import (
	"net/http"

	"github.com/rasparac/rekreativko-api/shared/api"
)

// LimitBodySize refuses a request whose body is bigger than maxBytes with 413.
// A client that declares the size (Content-Length) is turned away before a byte
// is read; one that does not (chunked) is cut off by http.MaxBytesReader once it
// sends more than maxBytes, so nobody can make a service buffer an arbitrary
// amount of memory. Whoever reads the body then gets an *http.MaxBytesError.
func LimitBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				WriteBodyTooLarge(w, maxBytes)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			next.ServeHTTP(w, r)
		})
	}
}

// WriteBodyTooLarge answers 413 with the usual error body.
func WriteBodyTooLarge(w http.ResponseWriter, maxBytes int64) {
	// the connection can't be reused after a body that was never read
	w.Header().Set("Connection", "close")
	api.WriteError(w, http.StatusRequestEntityTooLarge, "request_too_large",
		"Request body is too large", map[string]any{"max_bytes": maxBytes})
}
