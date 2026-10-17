package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/rasparac/rekreativko-api/shared/api"
)

const (
	// AppKeyHeader carries the key the mobile app is built with.
	AppKeyHeader = "X-App-Key"

	AppKeyMissing = "missing"
	AppKeyInvalid = "invalid"
)

// RequireAppKey checks that a request carries one of the app's keys.
//
// The key ships inside the app, so anyone can pull it out; it keeps out
// casual traffic (curl, scanners, bots), it does not authenticate anyone. Several
// keys are accepted at once so one can be rotated in before the old one goes.
//
// Without keys the check is off. With keys but enforce false a bad or missing
// key is only reported to onReject (to see who would be turned away before
// switching enforcement on); with enforce true the request is refused with 403
// app_key_invalid - not 401, which the app answers by refreshing its token.
//
// The header is removed before the request goes on: the backends have no use for it.
func RequireAppKey(keys []string, enforce bool, onReject func(reason string)) func(http.Handler) http.Handler {
	var valid [][]byte
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			valid = append(valid, []byte(key))
		}
	}

	if len(valid) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			given := r.Header.Get(AppKeyHeader)
			r.Header.Del(AppKeyHeader)

			reason := ""
			switch {
			case given == "":
				reason = AppKeyMissing
			case !matchesAny(valid, []byte(given)):
				reason = AppKeyInvalid
			}

			if reason == "" {
				next.ServeHTTP(w, r)
				return
			}

			if onReject != nil {
				onReject(reason)
			}

			if !enforce {
				next.ServeHTTP(w, r)
				return
			}

			writeAppKeyRefusal(w)
		})
	}
}

// matchesAny compares against every key, however early one matches, so the
// time taken does not say which of them it was.
func matchesAny(valid [][]byte, given []byte) bool {
	match := 0
	for _, key := range valid {
		match |= subtle.ConstantTimeCompare(key, given)
	}
	return match == 1
}

func writeAppKeyRefusal(w http.ResponseWriter) {
	// Nothing to do if the client has gone: the refusal is already decided.
	_ = api.WriteError(
		w,
		http.StatusForbidden,
		"app_key_invalid",
		"missing or invalid app key",
		nil,
	)
}
