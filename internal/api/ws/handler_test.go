package ws

import (
	"context"
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

func TestRuntimeLifecycleTelemetrySync(t *testing.T) {
	sink := &testRuntimeStateSink{}
	h := NewHandlerWithOptions(HandlerOptions{
		HeartbeatTimeout: 80 * time.Millisecond,
		StateManager:     sink,
	})

	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)
	registerTrader(t, conn, "trader-life-1", "reg-life-1")

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "hb-life-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-life-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)
	_ = readEnvelope(t, conn)

	waitForCondition(t, 300*time.Millisecond, func() bool {
		_, _, lastHB, _, _ := sink.lifecycleSnapshot()
		return lastHB > 0
	})

	// Let read deadline expire to trigger timeout path.
	time.Sleep(160 * time.Millisecond)
	_, _, _ = readMessageWithTimeout(conn, 200*time.Millisecond)

	waitForCondition(t, 300*time.Millisecond, func() bool {
		_, _, _, lastTimeout, timeoutCount := sink.lifecycleSnapshot()
		return lastTimeout > 0 && timeoutCount > 0
	})
}

func TestSessionPersistenceLifecycle(t *testing.T) {
	persistence := &testSessionPersistence{resolvedTraderID: 101}
	h := NewHandlerWithOptions(HandlerOptions{
		HeartbeatTimeout: 3 * time.Second,
		Persistence:      persistence,
	})

	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	registerReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "persist-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, registerReq)

	regResp := readEnvelope(t, conn)
	if regResp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, regResp.Action)
	}

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "persist-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)

	hbResp := readEnvelope(t, conn)
	if hbResp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, hbResp.Action)
	}

	_ = conn.Close()

	waitForCondition(t, 300*time.Millisecond, func() bool {
		return persistence.finalizeCalls() > 0
	})

	if persistence.resolveCalls() == 0 {
		t.Fatalf("expected ResolveTraderID to be called")
	}
	if persistence.createCalls() == 0 {
		t.Fatalf("expected CreateSession to be called")
	}
	if persistence.heartbeatCalls() == 0 {
		t.Fatalf("expected UpdateHeartbeat to be called")
	}
}

func TestRegisterFailsWhenTraderResolveFails(t *testing.T) {
	persistence := &testSessionPersistence{resolveErr: context.DeadlineExceeded}
	h := NewHandlerWithOptions(HandlerOptions{
		Persistence: persistence,
	})

	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "resolve-fail-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-unknown",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errInternalError)
}

func TestUnsupportedProtocolVersion(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:            msgTypeRequest,
		Action:          actionTraderRegister,
		ProtocolVersion: "999",
		RequestID:       "proto-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-eu-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errUnsupportedVersion)
}

func TestDuplicateRequestID(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	registerTrader(t, conn, "trader-eu-1", "dup-req-1")

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "dup-req-2",
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

	writeJSON(t, conn, hbReq)
	dupResp := readEnvelope(t, conn)
	assertErrorCode(t, dupResp, errDuplicateRequest)
}

func TestInboundRateLimit(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{MaxMessagesPerSec: 2})
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	registerTrader(t, conn, "trader-rl-1", "rl-reg-1")

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "rl-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-rl-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)
	_ = readEnvelope(t, conn)

	hbReq.RequestID = "rl-hb-2"
	writeJSON(t, conn, hbReq)
	limitedResp := readEnvelope(t, conn)
	assertErrorCode(t, limitedResp, errRateLimited)
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
	mu                sync.Mutex
	active            int64
	lastConnectUnix   int64
	lastHeartbeatUnix int64
	lastTimeoutUnix   int64
	timeoutCount      int64
}

type testSessionPersistence struct {
	mu               sync.Mutex
	resolvedTraderID int
	resolveErr       error
	createErr        error
	heartbeatErr     error
	finalizeErr      error
	resolveCount     int
	createCount      int
	heartbeatCount   int
	finalizeCount    int
}

func (p *testSessionPersistence) ResolveTraderID(_ context.Context, _ string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resolveCount++
	if p.resolveErr != nil {
		return 0, p.resolveErr
	}
	return p.resolvedTraderID, nil
}

func (p *testSessionPersistence) CreateSession(_ context.Context, _ SessionCreateInput) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createCount++
	return p.createErr
}

func (p *testSessionPersistence) UpdateHeartbeat(_ context.Context, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.heartbeatCount++
	return p.heartbeatErr
}

func (p *testSessionPersistence) FinalizeSession(_ context.Context, _ string, _ string, _ *string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.finalizeCount++
	return p.finalizeErr
}

func (p *testSessionPersistence) resolveCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resolveCount
}

func (p *testSessionPersistence) createCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createCount
}

func (p *testSessionPersistence) heartbeatCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.heartbeatCount
}

func (p *testSessionPersistence) finalizeCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.finalizeCount
}

func (s *testRuntimeStateSink) SetRuntimeWS(active int64, lastConnectUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = active
	s.lastConnectUnix = lastConnectUnix
}

func (s *testRuntimeStateSink) SetRuntimeWSHeartbeat(lastHeartbeatUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastHeartbeatUnix = lastHeartbeatUnix
}

func (s *testRuntimeStateSink) IncrementRuntimeWSTimeout(lastTimeoutUnix int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastTimeoutUnix = lastTimeoutUnix
	s.timeoutCount++
}

func (s *testRuntimeStateSink) snapshot() (int64, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active, s.lastConnectUnix
}

func (s *testRuntimeStateSink) lifecycleSnapshot() (int64, int64, int64, int64, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active, s.lastConnectUnix, s.lastHeartbeatUnix, s.lastTimeoutUnix, s.timeoutCount
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
