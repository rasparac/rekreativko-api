package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

func Recover(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// http.ErrAbortHandler is a sentinel panic value used (e.g. by
					// httputil.ReverseProxy) to silently abort a response whose
					// connection is already broken - the client disconnected, or the
					// request context was cancelled (timeout). net/http's own server
					// recognizes this value and aborts without logging; re-panic so
					// it can do the same, instead of treating it as an application error.
					if err == http.ErrAbortHandler {
						panic(err)
					}

					// Use request context to preserve tracing, request ID, etc.
					ctx := r.Context()
					log.Error(
						ctx,
						"recovered from panic",
						"error", err,
						"stack", string(debug.Stack()),
						"path", r.URL.Path,
						"http_method", r.Method,
					)
					api.WriteInternalServerErrorResponse(w)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
