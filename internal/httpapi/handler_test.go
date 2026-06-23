package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"ip2country/internal/geoip"
	"ip2country/internal/metrics"
	"ip2country/internal/ratelimit"
)

// fakeStore returns a fixed location for one known IP, ErrNotFound otherwise.
type fakeStore struct{}

func (fakeStore) Lookup(addr netip.Addr) (geoip.Location, error) {
	if addr == netip.MustParseAddr("2.22.233.255") {
		return geoip.Location{Country: "GB", City: "London"}, nil
	}
	return geoip.Location{}, geoip.ErrNotFound
}

func newServer(rps float64) http.Handler {
	return Handler(deps(rps))
}

func deps(rps float64) Deps {
	return Deps{
		Store:   fakeStore{},
		Limiter: ratelimit.New(rps),
		Metrics: metrics.New(),
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestFindCountry(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
		wantBody   map[string]string
	}{
		{"success", "GET", "/v1/find-country?ip=2.22.233.255", 200, map[string]string{"country": "GB", "city": "London"}},
		{"missing ip", "GET", "/v1/find-country", 400, map[string]string{"error": "missing ip parameter"}},
		{"invalid ip", "GET", "/v1/find-country?ip=not-an-ip", 400, map[string]string{"error": "invalid ip address"}},
		{"not found", "GET", "/v1/find-country?ip=9.9.9.9", 404, map[string]string{"error": "country not found"}},
		{"wrong method", "POST", "/v1/find-country?ip=2.22.233.255", 405, map[string]string{"error": "method not allowed"}},
		{"unknown path", "GET", "/v1/nope", 404, map[string]string{"error": "not found"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(1000)
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody == nil {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("content-type = %q, want application/json", ct)
			}
			var got map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			for k, want := range tt.wantBody {
				if got[k] != want {
					t.Errorf("body[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestRateLimit429(t *testing.T) {
	srv := newServer(1) // burst of 1
	do := func() int {
		req := httptest.NewRequest("GET", "/v1/find-country?ip=2.22.233.255", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(); code != 200 {
		t.Fatalf("first request = %d, want 200", code)
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", code)
	}
}

func TestWrongMethodNotMaskedByRateLimit(t *testing.T) {
	srv := newServer(1) // burst of 1
	// Exhaust the limiter with a valid GET.
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/find-country?ip=2.22.233.255", nil))

	// A POST now must still get 405, not the limiter's 429.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/find-country?ip=2.22.233.255", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405 (method check must precede rate limit)", rec.Code)
	}
}

func TestHealthzNotRateLimited(t *testing.T) {
	srv := newServer(1)
	// Exhaust the find-country budget first.
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/find-country?ip=2.22.233.255", nil))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 {
		t.Fatalf("healthz = %d, want 200 (must bypass rate limit)", rec.Code)
	}
}

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	srv := newServer(1000)

	// No inbound id -> one is generated and returned.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if id := rec.Header().Get("X-Request-ID"); id == "" {
		t.Error("expected generated X-Request-ID in response")
	}

	// Inbound id -> echoed back unchanged.
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-ID", "client-supplied-id")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if id := rec.Header().Get("X-Request-ID"); id != "client-supplied-id" {
		t.Errorf("X-Request-ID = %q, want client-supplied-id", id)
	}
}

func TestRecoverPanic(t *testing.T) {
	d := deps(1000)
	mux := http.NewServeMux()
	mux.HandleFunc("/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	srv := recoverPanic(d.Log, mux)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body not JSON: %v (%q)", err, rec.Body.String())
	}
	if got["error"] != "internal error" {
		t.Errorf("error = %q, want internal error", got["error"])
	}
}

func TestMetricsRecorded(t *testing.T) {
	d := deps(1000)
	srv := Handler(d)

	// One success and one not-found.
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/find-country?ip=2.22.233.255", nil))
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/find-country?ip=9.9.9.9", nil))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	if !strings.Contains(body, `http_requests_total{method="GET",path="/v1/find-country",status="200"} 1`) {
		t.Errorf("missing 200 counter in scrape:\n%s", body)
	}
	if !strings.Contains(body, `http_requests_total{method="GET",path="/v1/find-country",status="404"} 1`) {
		t.Errorf("missing 404 counter in scrape:\n%s", body)
	}
}

func TestMetricsRateLimitRejection(t *testing.T) {
	d := deps(1) // burst of 1
	srv := Handler(d)

	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/find-country?ip=2.22.233.255", nil)) // 200
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/find-country?ip=2.22.233.255", nil)) // 429

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "rate_limit_rejections_total 1") {
		t.Errorf("expected rate_limit_rejections_total 1:\n%s", rec.Body.String())
	}
}
