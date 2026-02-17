// Package middleware provides HTTP middleware functions for the REST API.
package middleware

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Context key type for request-scoped values.
type contextKey string

// RequestIDKey is the context key for request ID.
const RequestIDKey contextKey = "request_id"

// RequestIDHeader is the HTTP header name for request ID.
const RequestIDHeader = "X-Request-ID"

// Prometheus metrics.
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
	)
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// newResponseWriter creates a new responseWriter.
func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code and writes the header.
func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// Write writes the response body.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Hijack implements http.Hijacker interface to support WebSocket connections.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Flush implements http.Flusher interface.
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain creates a single middleware from multiple middlewares.
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// healthPaths contains paths that should be logged at Debug level
// to reduce log noise from frequent health/readiness probes.
var healthPaths = map[string]bool{
	"/health":  true,
	"/ready":   true,
	"/metrics": true,
}

// Logging returns a middleware that logs HTTP requests.
// Health, readiness, and metrics endpoints are logged at Debug level
// to reduce noise from frequent probe requests.
func Logging(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := newResponseWriter(w)

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.statusCode),
				zap.Duration("duration", duration),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
				zap.String("request_id", getRequestID(r)),
			}

			if healthPaths[r.URL.Path] {
				logger.Debug("http request", fields...)
			} else {
				logger.Info("http request", fields...)
			}
		})
	}
}

// Recovery returns a middleware that recovers from panics.
func Recovery(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						zap.Any("error", err),
						zap.String("stack", string(debug.Stack())),
						zap.String("path", r.URL.Path),
						zap.String("method", r.Method),
						zap.String("request_id", getRequestID(r)),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID returns a middleware that adds a unique request ID to each request.
// The ID is stored in the response header, request header, and request context.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(RequestIDHeader)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			w.Header().Set(RequestIDHeader, requestID)
			r.Header.Set(RequestIDHeader, requestID)

			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Metrics returns a middleware that records Prometheus metrics.
func Metrics() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := newResponseWriter(w)

			httpRequestsInFlight.Inc()
			defer httpRequestsInFlight.Dec()

			next.ServeHTTP(rw, r)

			duration := time.Since(start).Seconds()
			path := normalizeRequestPath(r)
			status := strconv.Itoa(rw.statusCode)

			httpRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
		})
	}
}

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
func CORS(allowedOrigins []string, allowedMethods []string, allowedHeaders []string) Middleware {
	originsMap := make(map[string]bool)
	for _, origin := range allowedOrigins {
		originsMap[origin] = true
	}

	methodsStr := strings.Join(allowedMethods, ", ")
	headersStr := strings.Join(allowedHeaders, ", ")

	hasWildcard := originsMap["*"]

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if hasWildcard {
				// When wildcard is configured, allow all origins but do not
				// set credentials header as browsers reject this combination.
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else if originsMap[origin] {
				// Specific origin matched — safe to allow credentials.
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			w.Header().Set("Access-Control-Allow-Methods", methodsStr)
			w.Header().Set("Access-Control-Allow-Headers", headersStr)
			w.Header().Set("Access-Control-Max-Age", "86400")

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getRequestID extracts the request ID from the request header.
func getRequestID(r *http.Request) string {
	return r.Header.Get(RequestIDHeader)
}

// joinStrings joins strings with comma separator.
// Retained for test compatibility; production code uses strings.Join directly.
//
//nolint:unused // used by unit tests in the same package
func joinStrings(strs []string) string {
	return strings.Join(strs, ", ")
}

// normalizePath truncates long paths for display purposes.
// Retained for test compatibility; production code uses normalizeRequestPath.
//
//nolint:unused // used by unit tests in the same package
func normalizePath(path string) string {
	if len(path) > 50 {
		return path[:50]
	}
	return path
}

// normalizeRequestPath extracts the route template from the matched mux route
// to avoid high-cardinality metric labels caused by dynamic path segments.
// Falls back to the raw URL path if no route template is available.
func normalizeRequestPath(r *http.Request) string {
	if route := mux.CurrentRoute(r); route != nil {
		if tmpl, err := route.GetPathTemplate(); err == nil {
			return tmpl
		}
	}
	return r.URL.Path
}
