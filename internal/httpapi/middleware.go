package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"ip2country/internal/metrics"
	"ip2country/internal/ratelimit"
)

// requestIDHeader carries the correlation id in and out of the service.
const requestIDHeader = "X-Request-ID"

type ctxKey int

const requestIDKey ctxKey = iota

// withRequestID assigns a correlation id to each request: it reuses an inbound
// X-Request-ID when present, otherwise generates one. The id is stored in the
// context (for logs) and echoed in the response header.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown" // crypto/rand failure is effectively impossible; degrade gracefully
	}
	return hex.EncodeToString(b[:])
}

func requestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// recoverPanic turns a handler panic into a 500 JSON response instead of a
// dropped connection, logging the stack with the request id.
func recoverPanic(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("panic recovered",
					"request_id", requestIDFrom(r.Context()),
					"panic", v,
					"stack", string(debug.Stack()),
				)
				writeError(w, log, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// rateLimit rejects requests with 429 when the global limiter is exhausted.
func rateLimit(limiter *ratelimit.Limiter, m *metrics.Metrics, log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			m.IncRateLimited()
			writeError(w, log, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response status for logging and metrics.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withMetrics records request count and latency, labelled by the matched route
// pattern (not the raw path) to keep label cardinality bounded.
func withMetrics(m *metrics.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		m.ObserveRequest(r.Method, routePattern(r), rec.status, time.Since(start))
	})
}

// logRequests emits one structured log line per request.
func logRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		log.Info("request",
			"request_id", requestIDFrom(r.Context()),
			"method", r.Method,
			"url", r.URL.RequestURI(),
			"status", rec.status,
			"duration", time.Since(start),
		)
	})
}

// routePattern returns the matched route path from the ServeMux pattern (Go
// 1.23+), without the optional leading method (method is its own metric label).
// Unmatched requests fall back to "other" so the label set stays bounded.
func routePattern(r *http.Request) string {
	p := r.Pattern
	if p == "" {
		return "other"
	}
	if i := strings.IndexByte(p, ' '); i >= 0 {
		p = p[i+1:] // drop "GET " prefix -> "/healthz"
	}
	return p
}
