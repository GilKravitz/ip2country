package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"

	"ip2country/internal/geoip"
	"ip2country/internal/metrics"
	"ip2country/internal/ratelimit"
)

type testStore struct {
	locations map[netip.Addr]geoip.Location
	err       error
}

func (s testStore) Lookup(addr netip.Addr) (geoip.Location, error) {
	if s.err != nil {
		return geoip.Location{}, s.err
	}
	if loc, ok := s.locations[addr]; ok {
		return loc, nil
	}

	return geoip.Location{}, geoip.ErrNotFound
}

func newTestServer(rps int) http.Handler {
	return Handler(newTestDeps(rps))
}

func newTestDeps(rps int) Deps {
	return Deps{
		Store:   newTestStore(),
		Limiter: ratelimit.New(rps),
		Metrics: metrics.New(),
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func newTestStore() testStore {
	return testStore{
		locations: map[netip.Addr]geoip.Location{
			netip.MustParseAddr("2.22.233.255"): {Country: "GB", City: "London"},
		},
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
		{
			name:       "success",
			method:     http.MethodGet,
			target:     "/v1/find-country?ip=2.22.233.255",
			wantStatus: http.StatusOK,
			wantBody:   map[string]string{"country": "GB", "city": "London"},
		},
		{
			name:       "missing ip",
			method:     http.MethodGet,
			target:     "/v1/find-country",
			wantStatus: http.StatusBadRequest,
			wantBody:   map[string]string{"error": "missing ip parameter"},
		},
		{
			name:       "invalid ip",
			method:     http.MethodGet,
			target:     "/v1/find-country?ip=not-an-ip",
			wantStatus: http.StatusBadRequest,
			wantBody:   map[string]string{"error": "invalid ip address"},
		},
		{
			name:       "not found",
			method:     http.MethodGet,
			target:     "/v1/find-country?ip=9.9.9.9",
			wantStatus: http.StatusNotFound,
			wantBody:   map[string]string{"error": "country not found"},
		},
		{
			name:       "wrong method",
			method:     http.MethodPost,
			target:     "/v1/find-country?ip=2.22.233.255",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   map[string]string{"error": "method not allowed"},
		},
		{
			name:       "unknown path",
			method:     http.MethodGet,
			target:     "/v1/nope",
			wantStatus: http.StatusNotFound,
			wantBody:   map[string]string{"error": "not found"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(1000)
			rec := performRequest(t, srv, tt.method, tt.target)

			assertJSONBody(t, rec, tt.wantStatus, tt.wantBody)
		})
	}
}

func TestFindCountryStoreFailureReturnsInternalError(t *testing.T) {
	d := newTestDeps(1000)
	d.Store = testStore{err: errors.New("backend unavailable")}
	srv := Handler(d)

	rec := performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")

	assertErrorBody(t, rec, http.StatusInternalServerError, "internal error")
}

func TestFindCountryRateLimitReturns429(t *testing.T) {
	srv := newTestServer(1) // one request per second, one-request burst

	first := performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")
	assertJSONBody(t, first, http.StatusOK, map[string]string{"country": "GB", "city": "London"})

	second := performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")
	assertErrorBody(t, second, http.StatusTooManyRequests, "rate limit exceeded")
}

func TestWrongMethodNotMaskedByRateLimit(t *testing.T) {
	srv := newTestServer(1) // one request per second, one-request burst
	performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")

	rec := performRequest(t, srv, http.MethodPost, "/v1/find-country?ip=2.22.233.255")
	assertErrorBody(t, rec, http.StatusMethodNotAllowed, "method not allowed")
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("Allow = %q, want %q", allow, http.MethodGet)
	}
}

func TestHealthzNotRateLimited(t *testing.T) {
	srv := newTestServer(1)
	performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")

	rec := performRequest(t, srv, http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200 (must bypass rate limit)", rec.Code)
	}
}

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	srv := newTestServer(1000)

	rec := performRequest(t, srv, http.MethodGet, "/healthz")
	if id := rec.Header().Get(requestIDHeader); id == "" {
		t.Error("expected generated X-Request-ID in response")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(requestIDHeader, "client-supplied-id")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if id := rec.Header().Get(requestIDHeader); id != "client-supplied-id" {
		t.Errorf("X-Request-ID = %q, want client-supplied-id", id)
	}
}

func TestRecoverPanic(t *testing.T) {
	d := newTestDeps(1000)
	mux := http.NewServeMux()
	mux.HandleFunc("/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })
	srv := recoverPanic(d.Log, mux)

	rec := performRequest(t, srv, http.MethodGet, "/boom")

	assertErrorBody(t, rec, http.StatusInternalServerError, "internal error")
}

func TestMetricsRecorded(t *testing.T) {
	d := newTestDeps(1000)
	srv := Handler(d)

	performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")
	performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=9.9.9.9")

	rec := performRequest(t, srv, http.MethodGet, "/metrics")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
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
	d := newTestDeps(1) // one request per second, one-request burst
	srv := Handler(d)

	performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")
	performRequest(t, srv, http.MethodGet, "/v1/find-country?ip=2.22.233.255")

	rec := performRequest(t, srv, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "rate_limit_rejections_total 1") {
		t.Errorf("expected rate_limit_rejections_total 1:\n%s", rec.Body.String())
	}
}

func performRequest(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantError string) {
	t.Helper()

	assertJSONBody(t, rec, wantStatus, map[string]string{"error": wantError})
}

func assertJSONBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, want map[string]string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rec.Code, wantStatus)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v (%q)", err, rec.Body.String())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body = %v, want %v", got, want)
	}
}
