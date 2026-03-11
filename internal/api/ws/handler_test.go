package ws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
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

func dialTestWS(t *testing.T) *websocket.Conn {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	h := NewHandler()
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
