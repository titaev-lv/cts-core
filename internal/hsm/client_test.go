package hsm

import (
	"bytes"
	"context"
	"encoding/json"
"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
"testing"
"time"

	"github.com/titaev-lv/cts-core/internal/requestid"
)

func TestCalculateBackoff(t *testing.T) {
logger := slog.Default()

client := &Client{
logger: logger,
retryCfg: RetryConfig{
MaxAttempts: 5,
InitialWait: 100 * time.Millisecond,
MaxWait:     5 * time.Second,
Multiplier:  2.0,
},
}

tests := []struct {
attempt  int
expected time.Duration
}{
{1, 100 * time.Millisecond},
{2, 200 * time.Millisecond},
{3, 400 * time.Millisecond},
{4, 800 * time.Millisecond},
{5, 1600 * time.Millisecond},
{10, 5 * time.Second},
}

for _, tt := range tests {
result := client.calculateBackoff(tt.attempt)
if result != tt.expected {
t.Errorf("Attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
}
}
}

func TestBase64Encoding(t *testing.T) {
tests := []struct {
name  string
input []byte
want  string
}{
{"empty", []byte{}, ""},
{"simple", []byte("hello"), "aGVsbG8="},
{"binary", []byte{0x01, 0x02, 0x03}, "AQID"},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
encoded := EncodeBase64(tt.input)
if encoded != tt.want {
t.Errorf("EncodeBase64() = %v, want %v", encoded, tt.want)
}

decoded, err := DecodeBase64(encoded)
if err != nil {
t.Errorf("DecodeBase64() error = %v", err)
}
if string(decoded) != string(tt.input) {
t.Errorf("Round-trip failed: got %v, want %v", decoded, tt.input)
}
})
}
}

func TestRetryConfig(t *testing.T) {
cfg := RetryConfig{
MaxAttempts: 3,
InitialWait: 100 * time.Millisecond,
MaxWait:     1 * time.Second,
Multiplier:  2.0,
}

if cfg.MaxAttempts != 3 {
t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
}
if cfg.InitialWait != 100*time.Millisecond {
t.Errorf("InitialWait = %v, want 100ms", cfg.InitialWait)
}
}

func TestDoRequestGeneratesRequestIDWhenMissing(t *testing.T) {
	var capturedRequestID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = r.Header.Get(hsmRequestIDHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	var outBuf bytes.Buffer
	client := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		logger:     slog.Default(),
		outLogger:  slog.New(slog.NewJSONHandler(&outBuf, &slog.HandlerOptions{Level: slog.LevelInfo})),
		retryCfg: RetryConfig{
			MaxAttempts: 1,
			InitialWait: 10 * time.Millisecond,
			MaxWait:     10 * time.Millisecond,
			Multiplier:  2,
		},
	}

	var resp map[string]any
	if err := client.doRequest(context.Background(), "GET", "/health", nil, &resp); err != nil {
		t.Fatalf("doRequest failed: %v", err)
	}

	if strings.TrimSpace(capturedRequestID) == "" {
		t.Fatalf("expected generated request_id in outbound header")
	}

	logOutput := outBuf.String()
	if !strings.Contains(logOutput, `"request_id":"`) {
		t.Fatalf("expected request_id field in out_request log, got: %s", logOutput)
	}
	if strings.Contains(logOutput, `"request_id":""`) {
		t.Fatalf("expected non-empty request_id in out_request log, got: %s", logOutput)
	}
}

func TestDoRequestPreservesExistingRequestID(t *testing.T) {
	const expectedRequestID = "rid-hsm-123"
	var capturedRequestID string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequestID = r.Header.Get(hsmRequestIDHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	var outBuf bytes.Buffer
	client := &Client{
		baseURL:    server.URL,
		httpClient: server.Client(),
		logger:     slog.Default(),
		outLogger:  slog.New(slog.NewJSONHandler(&outBuf, &slog.HandlerOptions{Level: slog.LevelInfo})),
		retryCfg: RetryConfig{
			MaxAttempts: 1,
			InitialWait: 10 * time.Millisecond,
			MaxWait:     10 * time.Millisecond,
			Multiplier:  2,
		},
	}

	ctx := requestid.WithContext(context.Background(), expectedRequestID)
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	var resp map[string]any
	if err := client.doRequest(ctx, "GET", "/health", nil, &resp); err != nil {
		t.Fatalf("doRequest failed: %v", err)
	}

	if capturedRequestID != expectedRequestID {
		t.Fatalf("expected outbound request_id=%s, got=%s", expectedRequestID, capturedRequestID)
	}

	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	if len(lines) == 0 {
		t.Fatal("expected out_request logs")
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("failed to parse out_request log JSON: %v", err)
	}
	if got, _ := record["request_id"].(string); got != expectedRequestID {
		t.Fatalf("expected logged request_id=%s, got=%s", expectedRequestID, got)
	}
}
