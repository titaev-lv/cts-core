package ws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestRegisterSuccess(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	if resp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, resp.Action)
	}
	if resp.RequestID != "req-1" {
		t.Fatalf("expected request_id=req-1, got %q", resp.RequestID)
	}

	var ack registerAck
	if err := json.Unmarshal(resp.Payload, &ack); err != nil {
		t.Fatalf("unmarshal ack: %v", err)
	}
	if ack.Status != "ok" || ack.TraderID != "trader-eu-1" || ack.SessionID == "" {
		t.Fatalf("unexpected ack payload: %+v", ack)
	}
}

func TestRegisterMissingTraderID(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-2",
		Payload: mustJSON(t, map[string]interface{}{
			"version": "1.0.0",
			"region":  "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errInvalidPayload)
}

func TestUnknownAction(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    "trader.unknown",
		RequestID: "req-3",
		Payload:   mustJSON(t, map[string]interface{}{}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errUnknownAction)
}

func TestDuplicateRegister(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-4",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	first := readEnvelope(t, conn)
	if first.Action != actionRegisterAck {
		t.Fatalf("expected first response ack, got %q", first.Action)
	}

	req.RequestID = "req-5"
	writeJSON(t, conn, req)
	second := readEnvelope(t, conn)
	assertErrorCode(t, second, errDuplicateConnection)
}

func TestRegisterWithoutRequestIDGetsServerRequestID(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:   msgTypeRequest,
		Action: actionTraderRegister,
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-2",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	if resp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, resp.Action)
	}
	if resp.RequestID == "" {
		t.Fatalf("expected non-empty generated request_id")
	}
	if !strings.HasPrefix(resp.RequestID, "srv-") {
		t.Fatalf("expected generated request_id prefix srv-, got %q", resp.RequestID)
	}
}

func TestHeartbeatRequestAfterRegisterReturnsAck(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	registerTrader(t, conn, "trader-eu-1", "reg-1")

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)

	resp := readEnvelope(t, conn)
	if resp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, resp.Action)
	}
	if resp.RequestID != "hb-1" {
		t.Fatalf("expected request_id hb-1, got %q", resp.RequestID)
	}

	var ack heartbeatAck
	if err := json.Unmarshal(resp.Payload, &ack); err != nil {
		t.Fatalf("unmarshal heartbeat ack: %v", err)
	}
	if ack.Status != "ok" || ack.TraderID != "trader-eu-1" || ack.SessionID == "" {
		t.Fatalf("unexpected heartbeat ack payload: %+v", ack)
	}
}

func TestHeartbeatBeforeRegisterReturnsError(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "hb-2",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
		}),
	}
	writeJSON(t, conn, hbReq)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errInvalidMessage)
}

func TestHeartbeatEventAfterRegisterHasNoResponse(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	registerTrader(t, conn, "trader-eu-1", "reg-2")

	hbEvent := envelope{
		Type:   msgTypeEvent,
		Action: actionTraderHeartbeat,
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbEvent)

	if _, _, err := readMessageWithTimeout(conn, 150*time.Millisecond); err == nil {
		t.Fatalf("expected no response for heartbeat event")
	}
}

func TestHeartbeatTimeoutClosesConnection(t *testing.T) {
	h := NewHandler()
	h.heartbeatTimeout = 60 * time.Millisecond

	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)
	registerTrader(t, conn, "trader-timeout-1", "reg-timeout")

	// Keep socket idle past heartbeat timeout; server should close the connection.
	time.Sleep(140 * time.Millisecond)

	if _, _, err := readMessageWithTimeout(conn, 200*time.Millisecond); err == nil {
		t.Fatalf("expected read error after heartbeat timeout disconnect")
	}
}

func TestRuntimeStateSyncAndConfiguredSessionTimeoutSec(t *testing.T) {
	sink := &testRuntimeStateSink{}
	h := NewHandlerWithOptions(HandlerOptions{
		HeartbeatTimeout: 3 * time.Second,
		StateManager:     sink,
	})

	conn := dialTestWSWithHandler(t, h)
	consumeConnected(t, conn)

	// Wait until connect side updates runtime counters.
	waitForCondition(t, 200*time.Millisecond, func() bool {
		active, _ := sink.snapshot()
		return active == 1
	})

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "cfg-timeout-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-cfg-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	if resp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, resp.Action)
	}
	var ack registerAck
	if err := json.Unmarshal(resp.Payload, &ack); err != nil {
		t.Fatalf("unmarshal ack: %v", err)
	}
	if ack.SessionTimeoutSec != 3 {
		t.Fatalf("expected session_timeout_sec=3, got %d", ack.SessionTimeoutSec)
	}

	_ = conn.Close()

	waitForCondition(t, 200*time.Millisecond, func() bool {
		active, _ := sink.snapshot()
		return active == 0
	})
}

func dialTestWS(t *testing.T) *websocket.Conn {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/ws", NewHandler().Serve)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return conn
}

func dialTestWSWithHandler(t *testing.T, h *Handler) *websocket.Conn {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/ws", h.Serve)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return conn
}

func consumeConnected(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_, _, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read connected: %v", err)
	}
}

func writeJSON(t *testing.T, conn *websocket.Conn, msg envelope) {
	t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write ws message: %v", err)
	}
}

func readEnvelope(t *testing.T, conn *websocket.Conn) envelope {
	t.Helper()
	_, b, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	var msg envelope
	if err := json.Unmarshal(b, &msg); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return msg
}

func assertErrorCode(t *testing.T, resp envelope, code string) {
	t.Helper()
	if resp.Action != actionError {
		t.Fatalf("expected action %q, got %q", actionError, resp.Action)
	}
	var p errorPayload
	if err := json.Unmarshal(resp.Payload, &p); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if p.Code != code {
		t.Fatalf("expected error code %q, got %q", code, p.Code)
	}
}

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}

func registerTrader(t *testing.T, conn *websocket.Conn, traderID, requestID string) {
	t.Helper()

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: requestID,
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": traderID,
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	if resp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, resp.Action)
	}
}

func readMessageWithTimeout(conn *websocket.Conn, timeout time.Duration) (int, []byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	msgType, b, err := conn.ReadMessage()
	_ = conn.SetReadDeadline(time.Time{})
	return msgType, b, err
}

type testRuntimeStateSink struct {
	mu              sync.Mutex
	active          int64
	lastConnectUnix int64
}

func (s *testRuntimeStateSink) SetRuntimeWS(active int64, lastConnectUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = active
	s.lastConnectUnix = lastConnectUnix
}

func (s *testRuntimeStateSink) snapshot() (int64, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active, s.lastConnectUnix
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition was not met in %s", timeout)
}
