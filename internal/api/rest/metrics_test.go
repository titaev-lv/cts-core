package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeMetricsPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "/metrics"},
		{name: "spaces", in: "  ", want: "/metrics"},
		{name: "with slash", in: "/internal/metrics", want: "/internal/metrics"},
		{name: "without slash", in: "internal/metrics", want: "/internal/metrics"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeMetricsPath(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeMetricsPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMetricsEndpointEnabled(t *testing.T) {
	router, _ := NewRouter(nil, Options{
		RESTRequestsPerSecond: 1000,
		RESTBurst:             1000,
		WSRequestsPerSecond:   1000,
		WSBurst:               1000,
		MetricsEnabled:        true,
		MetricsPath:           "/metrics",
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "go_goroutines") {
		t.Fatalf("expected go_goroutines metric, got body: %s", body)
	}
	if !strings.Contains(body, "cts_core_ws_active_connections") {
		t.Fatalf("expected cts_core_ws_active_connections metric")
	}
	if !strings.Contains(body, "cts_core_runtime_scheduler_cycle_count") {
		t.Fatalf("expected cts_core_runtime_scheduler_cycle_count metric")
	}
}

func TestMetricsEndpointDisabled(t *testing.T) {
	router, _ := NewRouter(nil, Options{
		RESTRequestsPerSecond: 1000,
		RESTBurst:             1000,
		WSRequestsPerSecond:   1000,
		WSBurst:               1000,
		MetricsEnabled:        false,
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}
