package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
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

type registerAckPayload struct {
	Status    string `json:"status"`
	TraderID  string `json:"trader_id"`
	SessionID string `json:"session_id"`
}

type heartbeatAckPayload struct {
	Status    string `json:"status"`
	TraderID  string `json:"trader_id"`
	SessionID string `json:"session_id"`
}

func main() {
	wsURL := os.Getenv("CTS_WS_URL")
	if wsURL == "" {
		wsURL = "wss://localhost:8080/ws"
	}
	parsed, err := url.Parse(wsURL)
	if err != nil {
		fail("invalid CTS_WS_URL: %v", err)
	}
	if !strings.EqualFold(parsed.Scheme, "wss") {
		fail("CTS_WS_URL must use wss://, got %s", wsURL)
	}

	caPath := envOrDefault("CTS_WS_CLIENT_CA_PATH", "../../volumes/pki/ca/ca.crt")
	certPath := envOrDefault("CTS_WS_CLIENT_CERT_PATH", "../../volumes/pki/cts-core/clients/trader-1/trader-1-cts.crt")
	keyPath := envOrDefault("CTS_WS_CLIENT_KEY_PATH", "../../volumes/pki/cts-core/clients/trader-1/trader-1-cts.key")
	serverName := os.Getenv("CTS_WS_SERVER_NAME")
	if serverName == "" {
		serverName = parsed.Hostname()
	}

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		fail("read CA cert: %v", err)
	}
	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(caPEM); !ok {
		fail("append CA cert: invalid PEM in %s", caPath)
	}

	clientCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		fail("load client cert/key: %v", err)
	}

	expectedTraderID := os.Getenv("CTS_SMOKE_CERTIFICATE_CN")
	if expectedTraderID == "" {
		expectedTraderID = "trader-1-cts-client"
	}

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      rootCAs,
		Certificates: []tls.Certificate{clientCert},
		ServerName:   serverName,
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

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	registerRequestID := "smoke-reg-" + runID
	heartbeatRequestID := "smoke-hb-" + runID

	registerReq := envelope{
		Type:            "request",
		Action:          "trader.register",
		ProtocolVersion: "2",
		RequestID:       registerRequestID,
		Payload: mustJSON(map[string]interface{}{
			"version": "1.0.0",
			"region":  "smoke",
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

	var regAck registerAckPayload
	if err := json.Unmarshal(regResp.Payload, &regAck); err != nil {
		fail("invalid register_ack payload: %v", err)
	}
	if regAck.TraderID == "" {
		fail("register_ack missing trader_id")
	}
	if regAck.SessionID == "" {
		fail("register_ack missing session_id")
	}
	if regAck.TraderID != expectedTraderID {
		fail("register_ack trader_id mismatch: expected=%s got=%s", expectedTraderID, regAck.TraderID)
	}

	heartbeatReq := envelope{
		Type:            "request",
		Action:          "trader.heartbeat",
		ProtocolVersion: "2",
		RequestID:       heartbeatRequestID,
		Payload: mustJSON(map[string]interface{}{
			"session_id": regAck.SessionID,
			"status":     "active",
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

	var hbAck heartbeatAckPayload
	if err := json.Unmarshal(hbResp.Payload, &hbAck); err != nil {
		fail("invalid heartbeat_ack payload: %v", err)
	}
	if hbAck.TraderID != regAck.TraderID {
		fail("heartbeat_ack trader_id mismatch: expected=%s got=%s", regAck.TraderID, hbAck.TraderID)
	}
	if hbAck.SessionID != regAck.SessionID {
		fail("heartbeat_ack session_id mismatch: expected=%s got=%s", regAck.SessionID, hbAck.SessionID)
	}

	if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "smoke-finish")); err != nil {
		fail("close ws: %v", err)
	}

	fmt.Printf("WS smoke lifecycle passed (run_id=%s register_request_id=%s heartbeat_request_id=%s)\n", runID, registerRequestID, heartbeatRequestID)
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
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
