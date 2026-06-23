package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveRequest(t *testing.T) {
	m := New()
	m.ObserveRequest("GET", "/v1/find-country", 200, 5*time.Millisecond)
	m.ObserveRequest("GET", "/v1/find-country", 200, 7*time.Millisecond)
	m.ObserveRequest("GET", "/v1/find-country", 404, time.Millisecond)

	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/v1/find-country", "200")); got != 2 {
		t.Errorf("200 count = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/v1/find-country", "404")); got != 1 {
		t.Errorf("404 count = %v, want 1", got)
	}
}

func TestIncRateLimited(t *testing.T) {
	m := New()
	m.IncRateLimited()
	m.IncRateLimited()
	if got := testutil.ToFloat64(m.rateLimited); got != 2 {
		t.Errorf("rate limited = %v, want 2", got)
	}
}

func TestScrapeHandler(t *testing.T) {
	m := New()
	m.ObserveRequest("GET", "/healthz", 200, time.Millisecond)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "http_requests_total") {
		t.Errorf("scrape output missing http_requests_total:\n%s", rec.Body.String())
	}
}
