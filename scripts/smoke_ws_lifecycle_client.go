package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type envelope struct {
	Type            string          `json:"type"`
	Action          string          `json:"action"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	RequestID       string          `json:"request_id,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	TS              int64           `json:"ts,omitempty"`
}

type errorPayload struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func main() {
	wsURL := os.Getenv("CTS_WS_URL")
	if wsURL == "" {
		wsURL = "ws://localhost:8081/ws"
	}
	if _, err := url.Parse(wsURL); err != nil {
		fail("invalid CTS_WS_URL: %v", err)
	}
	parsed, _ := url.Parse(wsURL)

	dialer := *websocket.DefaultDialer
	if parsed != nil && parsed.Scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		fail("dial ws: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, _, err = conn.ReadMessage() // connected event
	if err != nil {
		fail("read connected event: %v", err)
	}
	_ = conn.SetReadDeadline(time.Time{})

	traderID := os.Getenv("CTS_SMOKE_TRADER_ID")
	if traderID == "" {
		traderID = fmt.Sprintf("smoke-trader-%d", time.Now().UnixNano())
	}
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	registerRequestID := "smoke-reg-" + runID
	heartbeatRequestID := "smoke-hb-" + runID

	registerReq := envelope{
		Type:            "request",
		Action:          "trader.register",
		ProtocolVersion: "2",
		RequestID:       registerRequestID,
		Payload: mustJSON(map[string]interface{}{
			"trader_id": traderID,
			"version":   "1.0.0",
			"region":    "smoke",
		}),
	}
	writeEnvelope(conn, registerReq)

	regResp := readEnvelope(conn)
	if regResp.Action == "error" {
		printWSErrorAndFail(regResp, "register")
	}
	if regResp.Action != "trader.register_ack" {
		fail("unexpected register response action: %s", regResp.Action)
	}

	heartbeatReq := envelope{
		Type:            "request",
		Action:          "trader.heartbeat",
		ProtocolVersion: "2",
		RequestID:       heartbeatRequestID,
		Payload: mustJSON(map[string]interface{}{
			"trader_id": traderID,
			"status":    "active",
		}),
	}
	writeEnvelope(conn, heartbeatReq)

	hbResp := readEnvelope(conn)
	if hbResp.Action == "error" {
		printWSErrorAndFail(hbResp, "heartbeat")
	}
	if hbResp.Action != "trader.heartbeat_ack" {
		fail("unexpected heartbeat response action: %s", hbResp.Action)
	}

	if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "smoke-finish")); err != nil {
		fail("close ws: %v", err)
	}

	fmt.Printf("WS smoke lifecycle passed (run_id=%s register_request_id=%s heartbeat_request_id=%s)\n", runID, registerRequestID, heartbeatRequestID)
}

func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		fail("marshal payload: %v", err)
	}
	return b
}

func writeEnvelope(conn *websocket.Conn, msg envelope) {
	b, err := json.Marshal(msg)
	if err != nil {
		fail("marshal envelope: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		fail("write envelope: %v", err)
	}
}

func readEnvelope(conn *websocket.Conn) envelope {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, b, err := conn.ReadMessage()
	if err != nil {
		fail("read envelope: %v", err)
	}
	_ = conn.SetReadDeadline(time.Time{})

	var msg envelope
	if err := json.Unmarshal(b, &msg); err != nil {
		fail("unmarshal envelope: %v", err)
	}
	return msg
}

func printWSErrorAndFail(resp envelope, stage string) {
	var p errorPayload
	if err := json.Unmarshal(resp.Payload, &p); err != nil {
		fail("%s error response has invalid payload: %v", stage, err)
	}
	fail("%s failed with WS error code=%s message=%s details=%v", stage, p.Code, p.Message, p.Details)
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
