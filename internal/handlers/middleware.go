package handlers

import (
	"log"
	"net/http"
)

// WithRecover wraps an http.Handler and recovers from panics,
// returning HTTP 500 instead of crashing the server.
func WithRecover(next http.Handler, tplPath string) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Printf("[recover] %v (%s %s)", rec, r.Method, r.URL.Path)
                http.ServeFile(w, r, tplPath)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
