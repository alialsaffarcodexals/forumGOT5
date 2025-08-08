package handlers

import (
	"log"
	"net/http"
)

// WithRecover wraps an http.Handler and recovers from panics,
// returning HTTP 500 instead of crashing the server.
// WithRecover wraps an http.Handler and recovers from panics.
//
// If a panic occurs during request processing, the server will not crash.  The
// panic value is logged and the client receives a generic 500 Internal Server
// Error response.  A custom error page could be served from the handler
// itself if desired.  Passing only the next handler makes the signature
// consistent with other middlewares and simplifies usage.
func WithRecover(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Printf("[recover] %v (%s %s)", rec, r.Method, r.URL.Path)
                // Respond with a plain 500.  Handlers may choose to render a
                // template-based error page instead of relying on this default.
                http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}
