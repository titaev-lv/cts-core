package ws

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const defaultTestClientCN = "trader-eu-1"

func TestAuthorizeWSClientCertificateNotRequired(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{RequireClientCert: false})
	ok, reason := h.authorizeWSClientCertificate(&http.Request{})
	if !ok {
		t.Fatalf("expected allow when client cert is not required, got reason=%q", reason)
	}
}

func TestAuthorizeWSClientCertificateMissing(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{RequireClientCert: true})
	ok, reason := h.authorizeWSClientCertificate(&http.Request{TLS: &tls.ConnectionState{}})
	if ok {
		t.Fatalf("expected reject when client cert is missing")
	}
	if reason != "client_cert_missing" {
		t.Fatalf("expected reason client_cert_missing, got %q", reason)
	}
}

func TestAuthorizeWSClientCertificateAllowlistMatch(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{
		RequireClientCert:  true,
		AllowedCommonNames: []string{"trader-alpha"},
		AllowedOUs:         []string{"trading"},
		AllowedDNSNames:    []string{"trader.alpha.internal"},
	})

	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         "trader-alpha",
			OrganizationalUnit: []string{"Trading"},
		},
		DNSNames: []string{"trader.alpha.internal"},
	}

	ok, reason := h.authorizeWSClientCertificate(&http.Request{TLS: &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}})
	if !ok {
		t.Fatalf("expected allow for matching allowlists, got reason=%q", reason)
	}
}

func TestWSSHandshakeRejectsMissingClientCertificate(t *testing.T) {
	g := gin.New()
	h := NewHandlerWithOptions(HandlerOptions{RequireClientCert: true})
	g.GET("/ws", h.Serve)

	caCert, caKey, caPool := generateTestCA(t)
	serverCert := generateSignedTLSCert(t, caCert, caKey, "localhost", false)

	ts := httptest.NewUnstartedServer(g)
	ts.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	ts.StartTLS()
	defer ts.Close()

	wsURL := "wss" + strings.TrimPrefix(ts.URL, "https") + "/ws"
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    caPool,
		ServerName: "localhost",
	}}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected TLS handshake failure without client certificate")
	}
}

func TestRegisterUsesCertificateCNWhenClientCertRequired(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{RequireClientCert: true})
	conn := dialTestWSSWithHandlerAndClientCN(t, h, "trader-cn-1")
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-cn-priority-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "payload-trader-id",
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
	if ack.TraderID != "trader-cn-1" {
		t.Fatalf("expected trader_id from certificate CN %q, got %q", "trader-cn-1", ack.TraderID)
	}
}

func TestRegisterRejectsEmptyCertificateCNWhenClientCertRequired(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{RequireClientCert: true})
	conn := dialTestWSSWithHandlerAndClientCN(t, h, "")
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-cn-missing-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "payload-trader-id",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errInvalidPayload)
	assertErrorDetail(t, resp, "field", "certificate_cn")
	assertErrorDetail(t, resp, "path", "tls.peer_certificate.subject.cn")
	assertErrorDetail(t, resp, "reason", "required")
}

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

func TestRegisterAckIncludesExchangeCatalogAndEffectiveExchanges(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-reg-catalog-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id":    "trader-eu-1",
			"version":      "1.0.0",
			"region":       "eu-frankfurt",
			"capabilities": []string{"binance", "kucoin", "unknown"},
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
	if ack.ExchangeCatalogVersion == "" {
		t.Fatalf("expected exchange_catalog_version in ack")
	}
	if len(ack.AvailableExchanges) == 0 {
		t.Fatalf("expected available_exchanges in ack")
	}
	if len(ack.EffectiveExchanges) == 0 {
		t.Fatalf("expected effective_exchanges in ack")
	}

	joined := strings.Join(ack.EffectiveExchanges, ",")
	if !strings.Contains(joined, "binance") || !strings.Contains(joined, "kucoin") {
		t.Fatalf("expected effective_exchanges to include binance and kucoin, got %v", ack.EffectiveExchanges)
	}
	if strings.Contains(joined, "unknown") {
		t.Fatalf("expected unknown capability to be filtered out, got %v", ack.EffectiveExchanges)
	}
}

func TestRegisterMissingTraderID(t *testing.T) {
	conn := dialTestWSSWithHandlerAndClientCN(t, NewHandler(), "")
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
	assertErrorDetail(t, resp, "field", "certificate_cn")
	assertErrorDetail(t, resp, "path", "tls.peer_certificate.subject.cn")
	assertErrorDetail(t, resp, "reason", "required")
}

func TestRegisterInvalidPayloadDetails(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "req-invalid-register-payload",
		Payload:   mustJSON(t, "not-an-object"),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errInvalidPayload)
	assertErrorDetail(t, resp, "field", "payload")
	assertErrorDetail(t, resp, "path", "payload")
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

func TestSnapshotStoresRegisterAndHeartbeatTelemetry(t *testing.T) {
	h := NewHandler()
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	regReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "snapshot-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id":    "trader-snap-1",
			"version":      "1.0.0",
			"region":       "eu-frankfurt",
			"role":         "monitor",
			"capabilities": []string{"binance", "kucoin"},
			"current_load": map[string]interface{}{
				"load_index":       0.21,
				"trade_load_index": 0.11,
			},
		}),
	}
	writeJSON(t, conn, regReq)

	regResp := readEnvelope(t, conn)
	if regResp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, regResp.Action)
	}

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "snapshot-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id":        "trader-snap-1",
			"status":           "active",
			"load_index":       0.62,
			"trade_load_index": 0.35,
		}),
	}
	writeJSON(t, conn, hbReq)
	hbResp := readEnvelope(t, conn)
	if hbResp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, hbResp.Action)
	}

	waitForCondition(t, 200*time.Millisecond, func() bool {
		snaps := h.GetTraderSnapshots()
		return len(snaps) == 1 && snaps[0].LastHeartbeatUnix > 0
	})

	snaps := h.GetTraderSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected exactly one snapshot, got %d", len(snaps))
	}
	s := snaps[0]
	if s.Role != "monitor" {
		t.Fatalf("expected role monitor, got %q", s.Role)
	}
	if s.LoadIndex != 0.62 {
		t.Fatalf("expected load_index=0.62, got %v", s.LoadIndex)
	}
	if s.TradeLoadIndex != 0.35 {
		t.Fatalf("expected trade_load_index=0.35, got %v", s.TradeLoadIndex)
	}
	if len(s.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %v", s.Capabilities)
	}
	if len(s.EffectiveExchanges) == 0 {
		t.Fatalf("expected non-empty effective exchanges")
	}
}

func TestDispatchLatencyTestToRegisteredSession(t *testing.T) {
	h := NewHandler()
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	regReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "lat-dispatch-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id":    "trader-lat-disp-1",
			"version":      "1.0.0",
			"region":       "eu-frankfurt",
			"capabilities": []string{"binance", "kucoin"},
		}),
	}
	writeJSON(t, conn, regReq)

	regResp := readEnvelope(t, conn)
	if regResp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, regResp.Action)
	}

	var ack registerAck
	if err := json.Unmarshal(regResp.Payload, &ack); err != nil {
		t.Fatalf("unmarshal ack: %v", err)
	}

	if err := h.DispatchLatencyTest(context.Background(), ack.SessionID, ack.TraderID, []string{"binance", "kucoin"}); err != nil {
		t.Fatalf("dispatch latency test: %v", err)
	}

	dispatchResp := readEnvelope(t, conn)
	if dispatchResp.Action != actionLatencyTest {
		t.Fatalf("expected action %q, got %q", actionLatencyTest, dispatchResp.Action)
	}

	var payload latencyTestRequest
	if err := json.Unmarshal(dispatchResp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal latency test payload: %v", err)
	}
	if len(payload.Exchanges) != 2 {
		t.Fatalf("expected 2 exchanges, got %v", payload.Exchanges)
	}
}

func TestLatencyTestResultUpdatesSnapshotProfile(t *testing.T) {
	h := NewHandler()
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)
	registerTrader(t, conn, "trader-lat-res-1", "lat-res-reg-1")

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionLatencyTestResp,
		RequestID: "lat-res-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-lat-res-1",
			"results": []map[string]interface{}{
				{"exchange": "binance", "ws_latency_ms": 22.0},
				{"exchange": "kucoin", "ping_ms": 45.0},
			},
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	if resp.Action != actionLatencyTestAck {
		t.Fatalf("expected action %q, got %q", actionLatencyTestAck, resp.Action)
	}

	snaps := h.GetTraderSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected one snapshot, got %d", len(snaps))
	}
	if len(snaps[0].ExchangeLatencies) != 2 {
		t.Fatalf("expected 2 exchange latencies, got %v", snaps[0].ExchangeLatencies)
	}
	if snaps[0].LatencyProfileMs <= 0 {
		t.Fatalf("expected latency profile to be updated, got %f", snaps[0].LatencyProfileMs)
	}
}

func TestHeartbeatExchangeStatsUpdatesLatencyProfileInSnapshot(t *testing.T) {
	h := NewHandler()
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	regReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "snapshot-lat-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id":    "trader-lat-1",
			"version":      "1.0.0",
			"region":       "eu-frankfurt",
			"capabilities": []string{"binance", "kucoin"},
		}),
	}
	writeJSON(t, conn, regReq)
	regResp := readEnvelope(t, conn)
	if regResp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, regResp.Action)
	}

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "snapshot-lat-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-lat-1",
			"status":    "active",
			"exchange_stats": map[string]interface{}{
				"binance": map[string]interface{}{"latency_ms": 40.0},
				"kucoin":  map[string]interface{}{"latency_ms": 80.0},
			},
		}),
	}
	writeJSON(t, conn, hbReq)
	hbResp := readEnvelope(t, conn)
	if hbResp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, hbResp.Action)
	}

	snaps := h.GetTraderSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("expected exactly one snapshot, got %d", len(snaps))
	}
	if snaps[0].LatencyProfileMs <= 0 {
		t.Fatalf("expected latency_profile_ms to be populated from exchange_stats, got %f", snaps[0].LatencyProfileMs)
	}
	if len(snaps[0].ExchangeLatencies) != 2 {
		t.Fatalf("expected exchange_latencies to contain 2 entries, got %v", snaps[0].ExchangeLatencies)
	}
	if snaps[0].ExchangeLatencies["binance"] <= 0 || snaps[0].ExchangeLatencies["kucoin"] <= 0 {
		t.Fatalf("expected binance/kucoin latencies in snapshot, got %v", snaps[0].ExchangeLatencies)
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

func TestHeartbeatTraderIDMismatchDetails(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)
	registerTrader(t, conn, "trader-eu-1", "reg-mismatch-1")

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "hb-mismatch-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "other-trader",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)

	resp := readEnvelope(t, conn)
	if resp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, resp.Action)
	}

	var payload heartbeatAck
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal heartbeat ack payload: %v", err)
	}
	if payload.Status != "ok" {
		t.Fatalf("expected status %q, got %q", "ok", payload.Status)
	}
	if payload.TraderID != "trader-eu-1" {
		t.Fatalf("expected trader_id %q, got %q", "trader-eu-1", payload.TraderID)
	}
	if payload.SessionID == "" {
		t.Fatalf("expected non-empty session_id in heartbeat ack")
	}
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

func TestSeqAckStampedOnServerResponses(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	registerReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		Seq:       1,
		RequestID: "seq-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-seq-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, registerReq)

	regResp := readEnvelope(t, conn)
	if regResp.Action != actionRegisterAck {
		t.Fatalf("expected action %q, got %q", actionRegisterAck, regResp.Action)
	}
	if regResp.Seq != 1 {
		t.Fatalf("expected outbound seq=1, got %d", regResp.Seq)
	}
	if regResp.Ack != 1 {
		t.Fatalf("expected outbound ack=1, got %d", regResp.Ack)
	}

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		Seq:       2,
		RequestID: "seq-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-seq-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)

	hbResp := readEnvelope(t, conn)
	if hbResp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, hbResp.Action)
	}
	if hbResp.Seq != 2 {
		t.Fatalf("expected outbound seq=2, got %d", hbResp.Seq)
	}
	if hbResp.Ack != 2 {
		t.Fatalf("expected outbound ack=2, got %d", hbResp.Ack)
	}
}

func TestSequenceGapClosesConnection(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	registerReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		Seq:       1,
		RequestID: "gap-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-gap-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, registerReq)
	_ = readEnvelope(t, conn)

	gapReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		Seq:       3,
		RequestID: "gap-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-gap-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, gapReq)

	_, _, err := readMessageWithTimeout(conn, 300*time.Millisecond)
	if err == nil {
		t.Fatalf("expected connection close after sequence gap")
	}

	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected websocket.CloseError, got %T (%v)", err, err)
	}
	if closeErr.Code != wsCloseCodeSequenceGap {
		t.Fatalf("expected close code %d, got %d", wsCloseCodeSequenceGap, closeErr.Code)
	}
}

func TestDuplicateInboundSeqIsIgnored(t *testing.T) {
	conn := dialTestWS(t)
	defer conn.Close()

	consumeConnected(t, conn)

	registerReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		Seq:       1,
		RequestID: "dup-reg-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-dup-1",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, registerReq)
	_ = readEnvelope(t, conn)

	hb := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		Seq:       2,
		RequestID: "dup-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-dup-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hb)

	hbAck := readEnvelope(t, conn)
	if hbAck.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q, got %q", actionHeartbeatAck, hbAck.Action)
	}

	// Duplicate seq frame should be ignored without protocol error.
	hb.RequestID = "dup-hb-2"
	writeJSON(t, conn, hb)

	nextHB := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		Seq:       3,
		RequestID: "dup-hb-3",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-dup-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, nextHB)
	nextAck := readEnvelope(t, conn)
	if nextAck.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q after duplicate, got %q", actionHeartbeatAck, nextAck.Action)
	}
	if nextAck.RequestID != "dup-hb-3" {
		t.Fatalf("expected ack for request_id dup-hb-3, got %q", nextAck.RequestID)
	}
}

func TestCloseAllClosesActiveConnection(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{WriteTimeout: 250 * time.Millisecond})
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)
	registerTrader(t, conn, "trader-close-all-1", "close-all-reg-1")

	done := make(chan struct{})
	go func() {
		h.CloseAll(websocket.CloseNormalClosure, "server_shutdown")
		close(done)
	}()

	_, _, err := readMessageWithTimeout(conn, 500*time.Millisecond)
	if err == nil {
		t.Fatalf("expected close error after CloseAll")
	}
	if _, ok := err.(*websocket.CloseError); !ok {
		t.Fatalf("expected websocket.CloseError, got %T (%v)", err, err)
	}

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("CloseAll did not complete in time")
	}

	waitForCondition(t, 500*time.Millisecond, func() bool {
		return h.GetStats().ActiveConnections == 0
	})
}

func TestWaitForConnectionCloseTimeout(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{WriteTimeout: 100 * time.Millisecond})
	entry := &wsConnectionEntry{closed: make(chan struct{})}

	if h.waitForConnectionClose(entry, 30*time.Millisecond) {
		t.Fatalf("expected waitForConnectionClose to time out")
	}
}

func TestWaitForConnectionCloseSignals(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{WriteTimeout: 100 * time.Millisecond})
	entry := &wsConnectionEntry{closed: make(chan struct{})}

	go func() {
		time.Sleep(20 * time.Millisecond)
		entry.markClosed()
	}()

	if !h.waitForConnectionClose(entry, 200*time.Millisecond) {
		t.Fatalf("expected waitForConnectionClose to observe close signal")
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
	persistence := &testSessionPersistence{resolvedTrader: TraderIdentity{TraderDBID: 101, TraderID: "trader-eu-1", Status: "active"}}
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
		t.Fatalf("expected ResolveOrCreateTraderByCN to be called")
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

func TestRegisterFailsWithDuplicateConnectionWhenActiveSessionExists(t *testing.T) {
	persistence := &testSessionPersistence{
		resolvedTrader: TraderIdentity{TraderDBID: 101, TraderID: defaultTestClientCN, Status: "active"},
		createErr:      ErrActiveSessionExists,
	}
	h := NewHandlerWithOptions(HandlerOptions{
		Persistence: persistence,
	})

	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "active-session-conflict-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "ignored-by-cn",
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errDuplicateConnection)
	assertErrorDetail(t, resp, "trader_id", defaultTestClientCN)
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

func TestMessageTooLarge(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{MaxPayloadBytes: 64})
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	big := strings.Repeat("x", 512)
	req := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderRegister,
		RequestID: "too-large-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": big,
			"version":   "1.0.0",
			"region":    "eu-frankfurt",
		}),
	}
	writeJSON(t, conn, req)

	resp := readEnvelope(t, conn)
	assertErrorCode(t, resp, errMessageTooLarge)
}

func TestUnknownActionFloodClosesConnection(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{
		MaxUnknownActions:   2,
		UnknownActionWindow: 10 * time.Second,
		MaxMessagesPerSec:   100,
	})
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    "unknown.action",
		RequestID: "flood-1",
		Payload:   mustJSON(t, map[string]interface{}{}),
	}
	writeJSON(t, conn, req)
	resp1 := readEnvelope(t, conn)
	assertErrorCode(t, resp1, errUnknownAction)

	req.RequestID = "flood-2"
	writeJSON(t, conn, req)
	resp2 := readEnvelope(t, conn)
	assertErrorCode(t, resp2, errUnknownAction)

	resp3 := readEnvelope(t, conn)
	assertErrorCode(t, resp3, errActionFlood)

	if _, _, err := readMessageWithTimeout(conn, 200*time.Millisecond); err == nil {
		t.Fatalf("expected connection to be closed after action flood")
	}
}

func TestDuplicateRequestIDWindowExpires(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{
		RequestDedupWindow: 50 * time.Millisecond,
		MaxMessagesPerSec:  100,
	})
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)
	registerTrader(t, conn, "trader-dedup-1", "dedup-reg-1")

	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "dedup-window-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-dedup-1",
			"status":    "active",
		}),
	}
	writeJSON(t, conn, hbReq)
	first := readEnvelope(t, conn)
	if first.Action != actionHeartbeatAck {
		t.Fatalf("expected first action %q, got %q", actionHeartbeatAck, first.Action)
	}

	time.Sleep(80 * time.Millisecond)
	writeJSON(t, conn, hbReq)
	second := readEnvelope(t, conn)
	if second.Action != actionHeartbeatAck {
		t.Fatalf("expected second action %q after dedup window expiry, got %q", actionHeartbeatAck, second.Action)
	}
}

func TestUnknownActionWindowReset(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{
		MaxUnknownActions:   2,
		UnknownActionWindow: 60 * time.Millisecond,
		MaxMessagesPerSec:   100,
	})
	conn := dialTestWSWithHandler(t, h)
	defer conn.Close()

	consumeConnected(t, conn)

	req := envelope{
		Type:      msgTypeRequest,
		Action:    "unknown.action",
		RequestID: "unknown-window-1",
		Payload:   mustJSON(t, map[string]interface{}{}),
	}
	writeJSON(t, conn, req)
	resp1 := readEnvelope(t, conn)
	assertErrorCode(t, resp1, errUnknownAction)

	time.Sleep(100 * time.Millisecond)
	req.RequestID = "unknown-window-2"
	writeJSON(t, conn, req)
	resp2 := readEnvelope(t, conn)
	assertErrorCode(t, resp2, errUnknownAction)

	if _, _, err := readMessageWithTimeout(conn, 120*time.Millisecond); err == nil {
		t.Fatalf("expected no ACTION_FLOOD after unknown action window reset")
	}
}

func TestUnknownActionFloodIsolationAcrossConnections(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{
		MaxUnknownActions:   2,
		UnknownActionWindow: 10 * time.Second,
		MaxMessagesPerSec:   100,
	})

	connFlood := dialTestWSWithHandler(t, h)
	defer connFlood.Close()
	connHealthy := dialTestWSWithHandler(t, h)
	defer connHealthy.Close()

	consumeConnected(t, connFlood)
	consumeConnected(t, connHealthy)

	unknownReq := envelope{
		Type:      msgTypeRequest,
		Action:    "unknown.action",
		RequestID: "iso-flood-1",
		Payload:   mustJSON(t, map[string]interface{}{}),
	}
	writeJSON(t, connFlood, unknownReq)
	assertErrorCode(t, readEnvelope(t, connFlood), errUnknownAction)

	unknownReq.RequestID = "iso-flood-2"
	writeJSON(t, connFlood, unknownReq)
	assertErrorCode(t, readEnvelope(t, connFlood), errUnknownAction)
	assertErrorCode(t, readEnvelope(t, connFlood), errActionFlood)

	// Ensure another connection is unaffected and still accepts normal flow.
	registerTrader(t, connHealthy, "trader-isolated-1", "iso-reg-1")
	hbReq := envelope{
		Type:      msgTypeRequest,
		Action:    actionTraderHeartbeat,
		RequestID: "iso-hb-1",
		Payload: mustJSON(t, map[string]interface{}{
			"trader_id": "trader-isolated-1",
			"status":    "active",
		}),
	}
	writeJSON(t, connHealthy, hbReq)
	resp := readEnvelope(t, connHealthy)
	if resp.Action != actionHeartbeatAck {
		t.Fatalf("expected action %q on healthy connection, got %q", actionHeartbeatAck, resp.Action)
	}
}

func TestParallelConnectionsRegisterHeartbeatStability(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{
		MaxMessagesPerSec: 200,
	})
	caCert, caKey, caPool := generateTestCA(t)
	serverCert := generateSignedTLSCert(t, caCert, caKey, "localhost", false)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", h.Serve)
	ts := httptest.NewUnstartedServer(r)
	ts.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	ts.StartTLS()
	defer ts.Close()

	wsURL := "wss" + strings.TrimPrefix(ts.URL, "https") + "/ws"

	const workers = 20
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	clientCerts := make([]tls.Certificate, workers)
	for i := 0; i < workers; i++ {
		clientCerts[i] = generateSignedTLSCert(t, caCert, caKey, fmt.Sprintf("trader-par-%d", i), true)
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			dialer := websocket.Dialer{TLSClientConfig: &tls.Config{
				MinVersion:   tls.VersionTLS13,
				RootCAs:      caPool,
				Certificates: []tls.Certificate{clientCerts[i]},
				ServerName:   "localhost",
			}}

			conn, _, err := dialer.Dial(wsURL, nil)
			if err != nil {
				errCh <- fmt.Errorf("dial worker %d: %w", i, err)
				return
			}
			defer conn.Close()

			if _, _, err := conn.ReadMessage(); err != nil {
				errCh <- fmt.Errorf("read connected worker %d: %w", i, err)
				return
			}

			register := envelope{
				Type:      msgTypeRequest,
				Action:    actionTraderRegister,
				RequestID: fmt.Sprintf("par-reg-%d", i),
				Payload: mustJSON(t, map[string]interface{}{
					"trader_id": fmt.Sprintf("trader-par-%d", i),
					"version":   "1.0.0",
					"region":    "eu-frankfurt",
				}),
			}
			b, _ := json.Marshal(register)
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				errCh <- fmt.Errorf("write register worker %d: %w", i, err)
				return
			}

			_, regRespRaw, err := conn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("read register ack worker %d: %w", i, err)
				return
			}
			var regResp envelope
			if err := json.Unmarshal(regRespRaw, &regResp); err != nil {
				errCh <- fmt.Errorf("unmarshal register ack worker %d: %w", i, err)
				return
			}
			if regResp.Action != actionRegisterAck {
				errCh <- fmt.Errorf("unexpected register action worker %d: %s", i, regResp.Action)
				return
			}

			heartbeat := envelope{
				Type:      msgTypeRequest,
				Action:    actionTraderHeartbeat,
				RequestID: fmt.Sprintf("par-hb-%d", i),
				Payload: mustJSON(t, map[string]interface{}{
					"trader_id": fmt.Sprintf("trader-par-%d", i),
					"status":    "active",
				}),
			}
			hb, _ := json.Marshal(heartbeat)
			if err := conn.WriteMessage(websocket.TextMessage, hb); err != nil {
				errCh <- fmt.Errorf("write heartbeat worker %d: %w", i, err)
				return
			}

			_, hbRespRaw, err := conn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("read heartbeat ack worker %d: %w", i, err)
				return
			}
			var hbResp envelope
			if err := json.Unmarshal(hbRespRaw, &hbResp); err != nil {
				errCh <- fmt.Errorf("unmarshal heartbeat ack worker %d: %w", i, err)
				return
			}
			if hbResp.Action != actionHeartbeatAck {
				errCh <- fmt.Errorf("unexpected heartbeat action worker %d: %s", i, hbResp.Action)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	waitForCondition(t, 500*time.Millisecond, func() bool {
		return h.GetStats().ActiveConnections == 0
	})
}

func dialTestWSSWithHandlerAndClientCN(t *testing.T, h *Handler, clientCN string) *websocket.Conn {
	t.Helper()
	gin.SetMode(gin.TestMode)

	caCert, caKey, caPool := generateTestCA(t)
	serverCert := generateSignedTLSCert(t, caCert, caKey, "localhost", false)
	clientCert := generateSignedTLSCert(t, caCert, caKey, clientCN, true)

	r := gin.New()
	r.GET("/ws", h.Serve)

	ts := httptest.NewUnstartedServer(r)
	ts.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	ts.StartTLS()
	t.Cleanup(ts.Close)

	wsURL := "wss" + strings.TrimPrefix(ts.URL, "https") + "/ws"
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			RootCAs:      caPool,
			Certificates: []tls.Certificate{clientCert},
			ServerName:   "localhost",
		},
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial wss: %v", err)
	}
	return conn
}

func generateTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, *x509.CertPool) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          mustRandomSerial(t),
		Subject:               pkix.Name{CommonName: "cts-test-ca"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create ca certificate: %v", err)
	}

	parsedCA, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca certificate: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(parsedCA)

	return parsedCA, caKey, caPool
}

func generateSignedTLSCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, commonName string, isClient bool) tls.Certificate {
	t.Helper()

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: mustRandomSerial(t),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	if isClient {
		leafTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		leafTemplate.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		leafTemplate.DNSNames = []string{"localhost"}
		leafTemplate.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}

	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})

	leafTLS, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build tls key pair: %v", err)
	}

	return leafTLS
}

func mustRandomSerial(t *testing.T) *big.Int {
	t.Helper()

	max := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, max)
	if err != nil {
		t.Fatalf("generate serial number: %v", err)
	}
	if serial.Sign() == 0 {
		return big.NewInt(1)
	}
	return serial
}

func dialTestWS(t *testing.T) *websocket.Conn {
	return dialTestWSSWithHandlerAndClientCN(t, NewHandler(), defaultTestClientCN)
}

func dialTestWSWithHandler(t *testing.T, h *Handler) *websocket.Conn {
	return dialTestWSSWithHandlerAndClientCN(t, h, defaultTestClientCN)
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
	if !isSupportedWSErrorCode(p.Code) {
		t.Fatalf("error code %q is not in supported WS error codes registry", p.Code)
	}
}

func assertErrorDetail(t *testing.T, resp envelope, key string, expected string) {
	t.Helper()
	var p errorPayload
	if err := json.Unmarshal(resp.Payload, &p); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if p.Details == nil {
		t.Fatalf("expected details map for error response")
	}
	actual, ok := p.Details[key]
	if !ok {
		t.Fatalf("expected details key %q to exist", key)
	}
	if actual != expected {
		t.Fatalf("expected details[%q]=%q, got %v", key, expected, actual)
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
	mu                  sync.Mutex
	active              int64
	lastConnectUnix     int64
	lastHeartbeatUnix   int64
	lastTimeoutUnix     int64
	timeoutCount        int64
	disconnectTotal     uint64
	disconnectClose4009 uint64
	disconnectByReason  map[string]uint64
}

type testSessionPersistence struct {
	mu             sync.Mutex
	resolvedTrader TraderIdentity
	resolveErr     error
	createErr      error
	heartbeatErr   error
	finalizeErr    error
	resolveCount   int
	createCount    int
	heartbeatCount int
	finalizeCount  int
}

func (p *testSessionPersistence) ResolveOrCreateTraderByCN(_ context.Context, _ string) (TraderIdentity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resolveCount++
	if p.resolveErr != nil {
		return TraderIdentity{}, p.resolveErr
	}
	return p.resolvedTrader, nil
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

func (s *testRuntimeStateSink) SetRuntimeWSDisconnect(total uint64, close4009 uint64, byReason map[string]uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disconnectTotal = total
	s.disconnectClose4009 = close4009
	reasons := make(map[string]uint64, len(byReason))
	for k, v := range byReason {
		reasons[k] = v
	}
	s.disconnectByReason = reasons
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

func (s *testRuntimeStateSink) disconnectSnapshot() (uint64, uint64, map[string]uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reasons := make(map[string]uint64, len(s.disconnectByReason))
	for k, v := range s.disconnectByReason {
		reasons[k] = v
	}
	return s.disconnectTotal, s.disconnectClose4009, reasons
}

func TestRecordDisconnectSyncsRuntimeState(t *testing.T) {
	sink := &testRuntimeStateSink{}
	h := NewHandlerWithOptions(HandlerOptions{StateManager: sink})

	h.recordDisconnect("timeout", false)
	h.recordDisconnect(wsDisconnectReasonClose4009, true)

	total, close4009, reasons := sink.disconnectSnapshot()
	if total != 2 {
		t.Fatalf("expected total disconnects=2, got %d", total)
	}
	if close4009 != 1 {
		t.Fatalf("expected close4009=1, got %d", close4009)
	}
	if reasons["timeout"] != 1 {
		t.Fatalf("expected timeout reason count=1, got %d", reasons["timeout"])
	}
	if reasons[wsDisconnectReasonClose4009] != 1 {
		t.Fatalf("expected close_4009 reason count=1, got %d", reasons[wsDisconnectReasonClose4009])
	}
}

func TestNewHandlerWithOptionsUsesConfiguredWriteTimeout(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{WriteTimeout: 17 * time.Second})
	if h.writeTimeout != 17*time.Second {
		t.Fatalf("expected handler write timeout=17s, got %s", h.writeTimeout)
	}
}

func TestNewHandlerWithOptionsWriteTimeoutFallbackToDefault(t *testing.T) {
	h := NewHandlerWithOptions(HandlerOptions{WriteTimeout: 0})
	if h.writeTimeout != defaultWSWriteTimeout {
		t.Fatalf("expected default handler write timeout=%s, got %s", defaultWSWriteTimeout, h.writeTimeout)
	}

	h = NewHandlerWithOptions(HandlerOptions{WriteTimeout: -1 * time.Second})
	if h.writeTimeout != defaultWSWriteTimeout {
		t.Fatalf("expected default handler write timeout=%s for negative input, got %s", defaultWSWriteTimeout, h.writeTimeout)
	}
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
