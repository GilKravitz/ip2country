//go:build e2e

// Package e2e contains black-box tests that exercise a *running* server over
// HTTP. They are tagged `e2e` so the normal unit suite (`go test ./...`) skips
// them.
//
// Run against a server started with RATE_LIMIT_RPS=1 and the sample dataset:
//
//	RATE_LIMIT_RPS=1 IP2COUNTRY_CSV_PATH=testdata/sample.csv go run ./cmd/server &
//	go test -tags e2e ./test/e2e/
//
// Override the target with BASE_URL (default http://localhost:8080).
package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var baseURL = getenv("BASE_URL", "http://localhost:8080")

// rateWindow lets the RATE_LIMIT_RPS=1 bucket refill before each functional
// test, so it sees its real status code rather than a 429 left over from a
// previous test.
const rateWindow = 1100 * time.Millisecond

func TestMain(m *testing.M) {
	// Fail fast with a clear message if nothing is listening.
	if _, err := http.Get(baseURL + "/healthz"); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: server not reachable at %s: %v\n", baseURL, err)
		fmt.Fprintln(os.Stderr, "start it with: RATE_LIMIT_RPS=1 IP2COUNTRY_CSV_PATH=testdata/sample.csv go run ./cmd/server")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestFindCountry(t *testing.T) {
	refillRateLimit(t)
	status, body := get(t, "/v1/find-country?ip=2.22.233.255")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["country"] != "GB" || body["city"] != "London" {
		t.Errorf("body = %v, want GB/London", body)
	}
}

func TestNotFound(t *testing.T) {
	refillRateLimit(t)
	assertError(t, "/v1/find-country?ip=9.9.9.9", http.StatusNotFound, "country not found")
}

func TestInvalidIP(t *testing.T) {
	refillRateLimit(t)
	assertError(t, "/v1/find-country?ip=999", http.StatusBadRequest, "invalid ip address")
}

func TestMissingIP(t *testing.T) {
	refillRateLimit(t)
	assertError(t, "/v1/find-country", http.StatusBadRequest, "missing ip parameter")
}

func TestMethodNotAllowed(t *testing.T) {
	refillRateLimit(t)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/find-country?ip=8.8.8.8", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestRateLimitReturns429(t *testing.T) {
	refillRateLimit(t)

	if status, _ := get(t, "/v1/find-country?ip=8.8.8.8"); status != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", status)
	}
	// Immediately again: the only available token was spent.
	if status, body := get(t, "/v1/find-country?ip=8.8.8.8"); status != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", status)
	} else if body["error"] != "rate limit exceeded" {
		t.Errorf("body = %v, want rate limit error", body)
	}
}

func TestMetricsExposed(t *testing.T) {
	get(t, "/v1/find-country?ip=8.8.8.8") // generate at least one observation
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", resp.StatusCode)
	}
	out, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(out), "http_requests_total") {
		t.Error("/metrics output missing http_requests_total")
	}
}

func TestHealthzBypassesRateLimit(t *testing.T) {
	// /healthz is exempt, so it answers 200 even when the API bucket is empty.
	get(t, "/v1/find-country?ip=8.8.8.8")
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", resp.StatusCode)
	}
}

// --- helpers ---

func refillRateLimit(t *testing.T) {
	t.Helper()
	time.Sleep(rateWindow)
}

func get(t *testing.T, path string) (int, map[string]string) {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body) // empty body (e.g. healthz) -> nil map, fine
	return resp.StatusCode, body
}

func assertError(t *testing.T, path string, wantStatus int, wantMsg string) {
	t.Helper()
	status, body := get(t, path)
	if status != wantStatus {
		t.Fatalf("status = %d, want %d", status, wantStatus)
	}
	if body["error"] != wantMsg {
		t.Errorf("error = %q, want %q", body["error"], wantMsg)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
