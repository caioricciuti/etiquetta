package api

import (
	"log"
	"net/http"
	"strings"
	"time"
)

// staticExtensions lists file extensions to skip logging for.
var staticExtensions = []string{
	".js", ".css", ".png", ".jpg", ".jpeg", ".gif", ".svg",
	".ico", ".woff", ".woff2", ".ttf", ".eot", ".map",
}

// RequestLogger is a clean, minimal HTTP request logger middleware.
// It skips static assets and uses the [http] prefix convention.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip static assets
		path := r.URL.Path
		for _, ext := range staticExtensions {
			if strings.HasSuffix(path, ext) {
				next.ServeHTTP(w, r)
				return
			}
		}

		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		duration := time.Since(start)

		if ww.status >= 400 {
			log.Printf("[http] %d %s %s %s", ww.status, r.Method, path, duration.Round(time.Millisecond))
		} else {
			log.Printf("[http] %d %s %s %s", ww.status, r.Method, path, duration.Round(time.Millisecond))
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
