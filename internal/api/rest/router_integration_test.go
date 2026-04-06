//go:build integration
// +build integration

package rest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
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

	const traderCN = "it-trader-cn-1"

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

	caCert, caKey, caPool := generateTestCA(t)
	serverCert := generateSignedTLSCert(t, caCert, caKey, "localhost", false)
	clientCert := generateSignedTLSCert(t, caCert, caKey, traderCN, true)

	ts := httptest.NewUnstartedServer(router)
	ts.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	ts.StartTLS()
	defer ts.Close()

	tlsClientConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
		ServerName:   "localhost",
	}

	wsURL := toWSURL(ts.URL + "/ws")
	dialer := websocket.Dialer{TLSClientConfig: tlsClientConfig}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsClientConfig}}

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
			"trader_id": traderCN,
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
			"trader_id": traderCN,
			"status":    "active",
		}),
	}
	writeWS(t, conn, heartbeat)
	hbResp := readWS(t, conn)
	if hbResp.Action != "trader.heartbeat_ack" {
		t.Fatalf("expected trader.heartbeat_ack, got %q", hbResp.Action)
	}

	healthResp, err := httpClient.Get(ts.URL + "/health")
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

	metricsResp, err := httpClient.Get(ts.URL + "/metrics")
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
	if strings.EqualFold(u.Scheme, "https") {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	return u.String()
}

func generateTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, *x509.CertPool) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          mustRandomSerial(t),
		Subject:               pkix.Name{CommonName: "cts-rest-integration-test-ca"},
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
