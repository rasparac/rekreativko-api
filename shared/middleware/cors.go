package middleware

import (
	"net/http"
	"strings"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedHeaders   []string
	AllowedMethods   []string
	ExposedHeaders   []string
	MaxAge           int
	AllowCredentials bool
}

func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-Request-ID", "Authorization"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		ExposedHeaders:   []string{"X-Request-ID"},
		MaxAge:           200,
		AllowCredentials: true,
	}
}

func CORS(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && isOriginAllowed(origin, config.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ","))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ","))
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ","))

			if config.AllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if config.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", string(rune(config.MaxAge)))
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isOriginAllowed(origin string, allowedOrigins []string) bool {
	for _, allowedOrigin := range allowedOrigins {
		if origin == allowedOrigin {
			return true
		}
	}
	return false
}
