//go:build integration
// +build integration

package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/titaev-lv/cts-core/internal/state"
)

type wsEnvelope struct {
	Type            string          `json:"type"`
	Action          string          `json:"action"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	RequestID       string          `json:"request_id,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

func TestRouterIntegration_HealthMetricsAndWSLifecycle(t *testing.T) {
	stateManager, err := state.NewManager(state.ManagerConfig{
		StateFile:    t.TempDir() + "/daemon.state",
		SyncInterval: time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("state manager init: %v", err)
	}

	router, _ := NewRouter(nil, Options{
		RESTRequestsPerSecond: 1000,
		RESTBurst:             1000,
		WSRequestsPerSecond:   1000,
		WSBurst:               1000,
		WSHeartbeatInterval:   5 * time.Second,
		WSHeartbeatTimeout:    15 * time.Second,
		WSMaxPayloadBytes:     64 * 1024,
		WSMaxMessagesPerSec:   200,
		WSMaxUnknownActions:   5,
		WSUnknownActionWindow: 10 * time.Second,
		WSRequestDedupWindow:  60 * time.Second,
		StateManager:          stateManager,
		MetricsEnabled:        true,
		MetricsPath:           "/metrics",
		StartedAt:             time.Now().Add(-5 * time.Second),
		ServiceVersion:        "integration-test",
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	wsURL := toWSURL(ts.URL + "/ws")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	_, connectedRaw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read connected: %v", err)
	}
	if !strings.Contains(string(connectedRaw), "connected") {
		t.Fatalf("expected connected event, got %s", string(connectedRaw))
	}

	register := wsEnvelope{
		Type:            "request",
		Action:          "trader.register",
		ProtocolVersion: "1",
		RequestID:       "it-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "it-trader-1",
			"version":   "1.0.0",
			"region":    "it",
		}),
	}
	writeWS(t, conn, register)
	regResp := readWS(t, conn)
	if regResp.Action != "trader.register_ack" {
		t.Fatalf("expected trader.register_ack, got %q", regResp.Action)
	}

	heartbeat := wsEnvelope{
		Type:            "request",
		Action:          "trader.heartbeat",
		ProtocolVersion: "1",
		RequestID:       "it-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "it-trader-1",
			"status":    "active",
		}),
	}
	writeWS(t, conn, heartbeat)
	hbResp := readWS(t, conn)
	if hbResp.Action != "trader.heartbeat_ack" {
		t.Fatalf("expected trader.heartbeat_ack, got %q", hbResp.Action)
	}

	healthResp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected /health status 503 (dependencies not configured), got %d", healthResp.StatusCode)
	}

	var healthBody map[string]interface{}
	if err := json.NewDecoder(healthResp.Body).Decode(&healthBody); err != nil {
		t.Fatalf("decode /health: %v", err)
	}
	components, ok := healthBody["components"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing components in health response")
	}
	websocketComp, ok := components["websocket"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing websocket component in health response")
	}
	if websocketComp["active_connections"].(float64) < 1 {
		t.Fatalf("expected at least one active ws connection in health response")
	}

	metricsResp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer metricsResp.Body.Close()
	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected /metrics status 200, got %d", metricsResp.StatusCode)
	}

	metricsRaw, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	metricsText := string(metricsRaw)
	if !strings.Contains(metricsText, "cts_core_ws_active_connections") {
		t.Fatalf("expected ws active metrics, got: %s", metricsText)
	}
	if !strings.Contains(metricsText, "cts_core_runtime_ws_timeout_count") {
		t.Fatalf("expected runtime ws timeout metric")
	}
}

func toWSURL(httpURL string) string {
	u, _ := url.Parse(httpURL)
	u.Scheme = "ws"
	return u.String()
}

func writeWS(t *testing.T, conn *websocket.Conn, msg wsEnvelope) {
	t.Helper()
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("write ws: %v", err)
	}
}

func readWS(t *testing.T, conn *websocket.Conn) wsEnvelope {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{})

	var msg wsEnvelope
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read ws: %v", err)
	}
	return msg
}

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}
