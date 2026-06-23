// Package httpapi exposes the geoip lookup over HTTP.
//
// It owns the transport concerns only — request parsing, status-code mapping,
// and JSON encoding — and delegates resolution to a geoip.Store. Routing uses
// the stdlib net/http ServeMux (method-aware patterns, Go 1.22+); no router
// dependency is needed.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"

	"ip2country/internal/geoip"
	"ip2country/internal/metrics"
	"ip2country/internal/ratelimit"
)

type errorResponse struct {
	Error string `json:"error"`
}

// Deps are the collaborators the HTTP layer needs. Grouping them keeps the
// constructor readable as the surface grows.
type Deps struct {
	Store   geoip.Store
	Limiter *ratelimit.Limiter
	Metrics *metrics.Metrics
	Log     *slog.Logger
}

// Handler builds the fully-wired HTTP handler: routes plus the middleware chain
// (recover → request id → logging → metrics → mux).
func Handler(d Deps) http.Handler {
	mux := http.NewServeMux()

	// Operational endpoints are intentionally outside the rate-limited route so
	// probes and scrapes never trip the limiter.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /metrics", d.Metrics.Handler())

	// Method-specific routes are dispatched natively by the mux; add new verbs
	// (e.g. POST) by registering them here. The method-less pattern on the same
	// path is the fallback for any unregistered method: it emits a JSON 405
	// (instead of ServeMux's plain-text default) before the limiter runs, so an
	// invalid method never consumes rate-limit budget nor gets masked by a 429.
	mux.Handle("GET /v1/find-country", rateLimit(d.Limiter, d.Metrics, d.Log, findCountry(d.Store, d.Log)))
	mux.HandleFunc("/v1/find-country", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, d.Log, http.StatusMethodNotAllowed, "method not allowed")
	})

	// JSON catch-all so unknown paths also honour the error contract.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, d.Log, http.StatusNotFound, "not found")
	})

	// Chain, inner → outer. recoverPanic sits just above the mux so a recovered
	// panic's 500 still flows back through the metrics and logging recorders;
	// withRequestID is outermost so even the panic log carries the request id.
	h := recoverPanic(d.Log, mux)
	h = withMetrics(d.Metrics, h)
	h = logRequests(d.Log, h)
	h = withRequestID(h)
	return h
}

func findCountry(store geoip.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("ip")
		if raw == "" {
			writeError(w, log, http.StatusBadRequest, "missing ip parameter")
			return
		}
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			writeError(w, log, http.StatusBadRequest, "invalid ip address")
			return
		}

		loc, err := store.Lookup(addr)
		switch {
		case errors.Is(err, geoip.ErrNotFound):
			writeError(w, log, http.StatusNotFound, "country not found")
			return
		case err != nil:
			log.Error("lookup failed", "request_id", requestIDFrom(r.Context()), "ip", raw, "err", err)
			writeError(w, log, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, log, http.StatusOK, loc)
	}
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error("encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, log *slog.Logger, status int, msg string) {
	writeJSON(w, log, status, errorResponse{Error: msg})
}
