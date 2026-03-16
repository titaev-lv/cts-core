package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/titaev-lv/cts-core/internal/api/middleware"
	"github.com/titaev-lv/cts-core/internal/logger"
)

type Handler struct {
	upgrader            websocket.Upgrader
	accessLog           *slog.Logger
	outLog              *slog.Logger
	sessionsMu          sync.RWMutex
	sessions            map[string]TraderSnapshot
	connectionsMu       sync.RWMutex
	connections         map[string]*wsConnectionEntry
	connectionsByConn   map[*websocket.Conn]*wsConnectionEntry
	heartbeatInterval   time.Duration
	heartbeatTimeout    time.Duration
	maxPayloadBytes     int
	maxMessagesPerSec   int
	maxUnknownActions   int
	unknownActionWindow time.Duration
	requestDedupWindow  time.Duration
	persistence         SessionPersistence
	dbRetry             DBRetryConfig
	stateManager        runtimeStateSink
	active              atomic.Int64
	total               atomic.Int64
	lastSeen            atomic.Int64
}

type wsConnectionEntry struct {
	sessionID string
	traderID  string
	conn      *websocket.Conn
	writeMu   sync.Mutex
}

type SessionCreateInput struct {
	TraderID       int
	SessionID      string
	WSConnectionID string
	IPAddress      string
	ConnectedAt    time.Time
	LastHeartbeat  time.Time
}

type SessionPersistence interface {
	ResolveTraderID(ctx context.Context, traderRef string) (int, error)
	CreateSession(ctx context.Context, input SessionCreateInput) error
	UpdateHeartbeat(ctx context.Context, sessionID string) error
	FinalizeSession(ctx context.Context, sessionID string, reason string, errorMsg *string) error
}

type DBRetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

type runtimeStateSink interface {
	SetRuntimeWS(active int64, lastConnectUnix int64)
}

type runtimeHeartbeatSink interface {
	SetRuntimeWSHeartbeat(lastHeartbeatUnix int64)
}

type runtimeTimeoutSink interface {
	IncrementRuntimeWSTimeout(lastTimeoutUnix int64)
}

type HandlerOptions struct {
	HeartbeatInterval   time.Duration
	HeartbeatTimeout    time.Duration
	MaxPayloadBytes     int
	MaxMessagesPerSec   int
	MaxUnknownActions   int
	UnknownActionWindow time.Duration
	RequestDedupWindow  time.Duration
	Persistence         SessionPersistence
	DBRetry             DBRetryConfig
	StateManager        runtimeStateSink
}

type Stats struct {
	ActiveConnections int64 `json:"active_connections"`
	TotalConnections  int64 `json:"total_connections"`
	LastConnectUnix   int64 `json:"last_connect_unix,omitempty"`
}

// TraderSnapshot is a runtime view of a single trader WS session.
type TraderSnapshot struct {
	TraderID           string             `json:"trader_id"`
	TraderDBID         int                `json:"trader_db_id,omitempty"`
	SessionID          string             `json:"session_id"`
	State              string             `json:"state"`
	RegisteredAtUnix   int64              `json:"registered_at_unix"`
	LastHeartbeatUnix  int64              `json:"last_heartbeat_unix"`
	TimedOutAtUnix     int64              `json:"timed_out_at_unix"`
	Role               string             `json:"role,omitempty"`
	Capabilities       []string           `json:"capabilities,omitempty"`
	EffectiveExchanges []string           `json:"effective_exchanges,omitempty"`
	LoadIndex          float64            `json:"load_index,omitempty"`
	TradeLoadIndex     float64            `json:"trade_load_index,omitempty"`
	LatencyProfileMs   float64            `json:"latency_profile_ms,omitempty"`
	ExchangeLatencies  map[string]float64 `json:"exchange_latencies,omitempty"`
}

const defaultExchangeCatalogVersion = "2026-03-15T00:00:00Z"

func defaultAvailableExchanges() []exchangeCatalogEntry {
	return []exchangeCatalogEntry{
		{
			ExchangeID:        1,
			Code:              "binance",
			Name:              "Binance",
			Enabled:           true,
			MarketTypes:       []string{"spot", "futures"},
			WSPublicEndpoint:  "wss://stream.binance.com:9443/ws",
			WSPrivateEndpoint: "wss://stream.binance.com:9443/ws",
			RESTEndpoint:      "https://api.binance.com",
			RateLimits:        map[string]int{"public_rps": 20, "private_rps": 10},
		},
		{
			ExchangeID:        2,
			Code:              "kucoin",
			Name:              "KuCoin",
			Enabled:           true,
			MarketTypes:       []string{"spot", "futures"},
			WSPublicEndpoint:  "wss://ws-api.kucoin.com",
			WSPrivateEndpoint: "wss://ws-api.kucoin.com",
			RESTEndpoint:      "https://api.kucoin.com",
			RateLimits:        map[string]int{"public_rps": 15, "private_rps": 10},
		},
		{
			ExchangeID:        3,
			Code:              "bybit",
			Name:              "Bybit",
			Enabled:           true,
			MarketTypes:       []string{"spot", "futures"},
			WSPublicEndpoint:  "wss://stream.bybit.com/v5/public",
			WSPrivateEndpoint: "wss://stream.bybit.com/v5/private",
			RESTEndpoint:      "https://api.bybit.com",
			RateLimits:        map[string]int{"public_rps": 20, "private_rps": 10},
		},
		{
			ExchangeID:        4,
			Code:              "okx",
			Name:              "OKX",
			Enabled:           true,
			MarketTypes:       []string{"spot", "futures"},
			WSPublicEndpoint:  "wss://ws.okx.com:8443/ws/v5/public",
			WSPrivateEndpoint: "wss://ws.okx.com:8443/ws/v5/private",
			RESTEndpoint:      "https://www.okx.com",
			RateLimits:        map[string]int{"public_rps": 20, "private_rps": 10},
		},
	}
}

func normalizeTraderRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case "monitor", "trade", "both":
		return normalized
	default:
		return "both"
	}
}

func normalizeCapabilities(capabilities []string) []string {
	if len(capabilities) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(capabilities))
	items := make([]string, 0, len(capabilities))
	for _, item := range capabilities {
		norm := strings.ToLower(strings.TrimSpace(item))
		if norm == "" {
			continue
		}
		if _, exists := seen[norm]; exists {
			continue
		}
		seen[norm] = struct{}{}
		items = append(items, norm)
	}
	return items
}

func buildEffectiveExchanges(capabilities []string, available []exchangeCatalogEntry) []string {
	enabledCodes := make([]string, 0, len(available))
	enabledSet := make(map[string]struct{}, len(available))
	for _, ex := range available {
		if !ex.Enabled {
			continue
		}
		code := strings.ToLower(strings.TrimSpace(ex.Code))
		if code == "" {
			continue
		}
		enabledCodes = append(enabledCodes, code)
		enabledSet[code] = struct{}{}
	}

	if len(capabilities) == 0 {
		return enabledCodes
	}

	effective := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if _, ok := enabledSet[capability]; ok {
			effective = append(effective, capability)
		}
	}
	return effective
}

func extractCurrentLoadIndices(currentLoad map[string]interface{}) (float64, float64) {
	load := 0.0
	tradeLoad := 0.0
	if currentLoad == nil {
		return load, tradeLoad
	}
	if v, ok := numberFromAny(currentLoad["load_index"]); ok {
		load = normalizeUnitRange(v)
	}
	if v, ok := numberFromAny(currentLoad["trade_load_index"]); ok {
		tradeLoad = normalizeUnitRange(v)
	}
	if tradeLoad == 0 && load > 0 {
		tradeLoad = load
	}
	return load, tradeLoad
}

func buildLatencyProfileFromExchangeStats(exchangeStats map[string]heartbeatExchangeStats, effectiveExchanges []string) (float64, bool) {
	latencies := extractLatencyMap(exchangeStats)
	if len(latencies) == 0 {
		return 0, false
	}
	return buildLatencyProfileFromLatencyMap(latencies, effectiveExchanges)
}

func extractLatencyMap(exchangeStats map[string]heartbeatExchangeStats) map[string]float64 {
	if len(exchangeStats) == 0 {
		return nil
	}
	latencies := make(map[string]float64, len(exchangeStats))
	for exchange, stats := range exchangeStats {
		if stats.LatencyMS == nil {
			continue
		}
		latency := *stats.LatencyMS
		if latency < 0 {
			continue
		}
		normExchange := strings.ToLower(strings.TrimSpace(exchange))
		if normExchange == "" {
			continue
		}
		latencies[normExchange] = latency
	}
	if len(latencies) == 0 {
		return nil
	}
	return latencies
}

func buildLatencyProfileFromLatencyMap(latenciesMap map[string]float64, effectiveExchanges []string) (float64, bool) {
	if len(latenciesMap) == 0 {
		return 0, false
	}

	filter := map[string]struct{}{}
	if len(effectiveExchanges) > 0 {
		for _, ex := range effectiveExchanges {
			filter[strings.ToLower(strings.TrimSpace(ex))] = struct{}{}
		}
	}

	latencies := make([]float64, 0, len(latenciesMap))
	for exchange, latency := range latenciesMap {
		if len(filter) > 0 {
			if _, ok := filter[strings.ToLower(strings.TrimSpace(exchange))]; !ok {
				continue
			}
		}
		latencies = append(latencies, latency)
	}

	if len(latencies) == 0 {
		return 0, false
	}

	sort.Float64s(latencies)
	maxLatency := latencies[len(latencies)-1]
	idx95 := int(math.Ceil(0.95*float64(len(latencies)))) - 1
	if idx95 < 0 {
		idx95 = 0
	}
	if idx95 >= len(latencies) {
		idx95 = len(latencies) - 1
	}
	p95 := latencies[idx95]
	spread := maxLatency - latencies[0]

	profile := p95 + 0.10*spread
	if len(effectiveExchanges) > 0 {
		coveragePenalty := float64(len(effectiveExchanges)-len(latencies)) * 300.0
		if coveragePenalty > 0 {
			profile += coveragePenalty
		}
	}
	return profile, true
}

func numberFromAny(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		n, err := val.Float64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// NewHandler creates a WebSocket handler with logging.
func NewHandler() *Handler {
	return NewHandlerWithOptions(HandlerOptions{})
}

// NewHandlerWithOptions creates a WebSocket handler with explicit runtime options.
func NewHandlerWithOptions(opts HandlerOptions) *Handler {
	heartbeatInterval := opts.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 5 * time.Second
	}

	heartbeatTimeout := opts.HeartbeatTimeout
	if heartbeatTimeout <= 0 {
		heartbeatTimeout = 15 * time.Second
	}

	return &Handler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(_ *http.Request) bool { return true },
		},
		accessLog:           logger.GetWSAccess("ws"),
		outLog:              logger.GetWSOut("ws"),
		sessions:            make(map[string]TraderSnapshot),
		connections:         make(map[string]*wsConnectionEntry),
		connectionsByConn:   make(map[*websocket.Conn]*wsConnectionEntry),
		heartbeatInterval:   heartbeatInterval,
		heartbeatTimeout:    heartbeatTimeout,
		maxPayloadBytes:     normalizeMaxPayloadBytes(opts.MaxPayloadBytes),
		maxMessagesPerSec:   normalizeMaxMessagesPerSec(opts.MaxMessagesPerSec),
		maxUnknownActions:   normalizeMaxUnknownActions(opts.MaxUnknownActions),
		unknownActionWindow: normalizeUnknownActionWindow(opts.UnknownActionWindow),
		requestDedupWindow:  normalizeRequestDedupWindow(opts.RequestDedupWindow),
		persistence:         opts.Persistence,
		dbRetry:             normalizeDBRetryConfig(opts.DBRetry),
		stateManager:        opts.StateManager,
	}
}

// Serve handles WebSocket connections (stub implementation).
func (h *Handler) Serve(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Get("ws").Error("WS upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	connID := generateConnID()
	requestID := middleware.GetRequestID(c)
	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()
	path := c.Request.URL.Path

	h.accessLog.Info("ws_connect", "conn_id", connID, "request_id", requestID, "ip", clientIP, "user_agent", userAgent, "ws_path", path)
	h.active.Add(1)
	h.total.Add(1)
	h.lastSeen.Store(time.Now().Unix())
	h.syncRuntimeState()
	defer func() {
		h.active.Add(-1)
		h.syncRuntimeState()
	}()

	sessionID := generateConnID()
	session := newSessionRuntime(sessionID)
	connEntry := h.addConnection(sessionID, conn)
	defer h.removeConnection(sessionID)
	persistedSession := false
	requestIDs := make(map[string]time.Time)
	rateWindowStart := time.Now().UTC()
	rateWindowCount := 0
	unknownWindowStart := time.Now().UTC()
	unknownActionCount := 0

	msgID := int64(0)
	if err := h.writeTextMessage(connEntry, []byte("{\"type\":\"connected\"}")); err == nil {
		msgID++
		h.outLog.Info("ws_out", "event", "connected", "conn_id", connID, "msg_id", msgID, "request_id", requestID)
	}

	for {
		msgType, rawPayload, err := conn.ReadMessage()
		if err != nil {
			nowMs := time.Now().UnixMilli()
			reason := classifyDisconnectReason(err)
			errMsg := err.Error()
			if reason == disconnectReasonTimeout && session.markTimedOut(nowMs) {
				h.syncRuntimeTimeout(nowMs / 1000)
				h.accessLog.Warn("ws_timeout", "conn_id", connID, "request_id", requestID, "trader_id", session.traderID, "session_id", session.sessionID, "session_state", session.state, "timed_out_at_ms", session.timedOutAtMs, "last_heartbeat_ms", session.lastHeartbeatMs)
			}

			if h.persistence != nil && persistedSession {
				if persistErr := h.withDBWriteRetry("finalize_session", func(ctx context.Context) error {
					return h.persistence.FinalizeSession(ctx, session.sessionID, string(reason), &errMsg)
				}); persistErr != nil {
					h.accessLog.Error("ws_persist_finalize_failed", "conn_id", connID, "session_id", session.sessionID, "reason", reason, "error", persistErr)
				}
			}

			stateBeforeDisconnect := session.state
			session.markDisconnected()
			h.deleteSessionSnapshot(session.sessionID)
			h.removeConnection(session.sessionID)
			h.accessLog.Warn("ws_disconnect", "conn_id", connID, "request_id", requestID, "trader_id", session.traderID, "session_id", session.sessionID, "previous_session_state", stateBeforeDisconnect, "session_state", session.state, "disconnect_reason", reason, "last_heartbeat_ms", session.lastHeartbeatMs, "error", err)
			return
		}

		msgID++
		h.accessLog.Info("ws_in", "conn_id", connID, "msg_id", msgID, "request_id", requestID, "msg_type", msgType, "size_bytes", len(rawPayload))

		if len(rawPayload) > h.maxPayloadBytes {
			msgRequestID := resolveRequestID("", msgID)
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errMessageTooLarge, "Payload exceeds maximum size", map[string]interface{}{"max_bytes": h.maxPayloadBytes, "size_bytes": len(rawPayload)}))
			continue
		}

		now := time.Now().UTC()
		if now.Sub(rateWindowStart) >= time.Second {
			rateWindowStart = now
			rateWindowCount = 0
		}
		rateWindowCount++
		if rateWindowCount > h.maxMessagesPerSec {
			msgRequestID := resolveRequestID("", msgID)
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errRateLimited, "Inbound message rate limit exceeded", map[string]interface{}{"limit_per_sec": h.maxMessagesPerSec}))
			continue
		}

		msgRequestID := resolveRequestID("", msgID)

		if msgType != websocket.TextMessage {
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidMessage, "Only text WS frames are supported", nil))
			continue
		}

		var msg envelope
		if err := json.Unmarshal(rawPayload, &msg); err != nil {
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidMessage, "Malformed JSON payload", map[string]interface{}{"error": err.Error()}))
			continue
		}

		msgRequestID = resolveRequestID(msg.RequestID, msgID)

		if msg.ProtocolVersion != "" && msg.ProtocolVersion != supportedProtocolVersion {
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errUnsupportedVersion, "Unsupported protocol version", map[string]interface{}{"supported": supportedProtocolVersion, "received": msg.ProtocolVersion}))
			continue
		}

		if msg.Type == msgTypeRequest && msg.RequestID != "" {
			cutoff := now.Add(-h.requestDedupWindow)
			for id, ts := range requestIDs {
				if ts.Before(cutoff) {
					delete(requestIDs, id)
				}
			}
			if _, exists := requestIDs[msg.RequestID]; exists {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errDuplicateRequest, "Duplicate request_id", map[string]interface{}{"request_id": msg.RequestID}))
				continue
			}
			requestIDs[msg.RequestID] = now
		}

		if msg.Type != msgTypeRequest && msg.Type != msgTypeEvent {
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidMessage, "Unsupported message type", map[string]interface{}{"type": msg.Type}))
			continue
		}

		switch msg.Action {
		case actionTraderRegister:
			if msg.Type != msgTypeRequest {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidMessage, "trader.register must be request type", nil))
				continue
			}

			if session.traderID != "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errDuplicateConnection, "Trader already registered for this connection", map[string]interface{}{"trader_id": session.traderID}))
				continue
			}

			var req registerRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "Invalid register payload", invalidPayloadDetails("payload", "payload", "object", err.Error())))
				continue
			}

			if req.TraderID == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id is required", requiredFieldDetails("trader_id", "payload.trader_id", "string")))
				continue
			}

			if req.Version == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "version is required", requiredFieldDetails("version", "payload.version", "string")))
				continue
			}

			nowMs := time.Now().UnixMilli()
			role := normalizeTraderRole(req.Role)
			capabilities := normalizeCapabilities(req.Capabilities)
			availableExchanges := defaultAvailableExchanges()
			effectiveExchanges := buildEffectiveExchanges(capabilities, availableExchanges)
			loadIndex, tradeLoadIndex := extractCurrentLoadIndices(req.CurrentLoad)
			resolvedTraderID := 0
			if h.persistence != nil {
				if err := h.withDBWriteRetry("resolve_trader", func(ctx context.Context) error {
					var resolveErr error
					resolvedTraderID, resolveErr = h.persistence.ResolveTraderID(ctx, req.TraderID)
					return resolveErr
				}); err != nil {
					h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInternalError, "Failed to resolve trader registration", nil))
					continue
				}

				if err := h.withDBWriteRetry("create_session", func(ctx context.Context) error {
					return h.persistence.CreateSession(ctx, SessionCreateInput{
						TraderID:       resolvedTraderID,
						SessionID:      session.sessionID,
						WSConnectionID: connID,
						IPAddress:      clientIP,
						ConnectedAt:    time.UnixMilli(nowMs).UTC(),
						LastHeartbeat:  time.UnixMilli(nowMs).UTC(),
					})
				}); err != nil {
					h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInternalError, "Failed to create trader session", nil))
					continue
				}
				persistedSession = true
			}

			session.markRegistered(req.TraderID, nowMs)
			session.setRegistrationInfo(role, capabilities, effectiveExchanges, resolvedTraderID)
			h.bindConnectionTrader(session.sessionID, req.TraderID)
			session.setTelemetry(loadIndex, tradeLoadIndex)
			h.upsertSessionSnapshot(session)
			if h.heartbeatTimeout > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(h.heartbeatTimeout))
			}
			timeoutSec := durationSecondsCeil(h.heartbeatTimeout)
			h.sendEnvelope(conn, connID, msgID, newRegisterAckEnvelope(msgRequestID, registerAck{
				Status:                 "ok",
				TraderID:               req.TraderID,
				SessionID:              session.sessionID,
				SessionTimeoutSec:      timeoutSec,
				ServerTime:             nowMs,
				ExchangeCatalogVersion: defaultExchangeCatalogVersion,
				AvailableExchanges:     availableExchanges,
				EffectiveExchanges:     effectiveExchanges,
			}))
			h.accessLog.Info("ws_register", "conn_id", connID, "request_id", msgRequestID, "trader_id", req.TraderID, "version", req.Version, "region", req.Region)
		case actionTraderHeartbeat:
			if session.traderID == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidMessage, "trader.register is required before heartbeat", nil))
				continue
			}

			var req heartbeatRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "Invalid heartbeat payload", invalidPayloadDetails("payload", "payload", "object", err.Error())))
				continue
			}

			if req.TraderID == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id is required", requiredFieldDetails("trader_id", "payload.trader_id", "string")))
				continue
			}

			if req.TraderID != session.traderID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id does not match registered trader", mismatchFieldDetails("trader_id", "payload.trader_id", session.traderID, req.TraderID)))
				continue
			}

			if req.SessionID != "" && req.SessionID != session.sessionID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "session_id does not match active session", mismatchFieldDetails("session_id", "payload.session_id", session.sessionID, req.SessionID)))
				continue
			}

			nowMs := time.Now().UnixMilli()
			if h.persistence != nil && persistedSession {
				if err := h.withDBWriteRetry("update_heartbeat", func(ctx context.Context) error {
					return h.persistence.UpdateHeartbeat(ctx, session.sessionID)
				}); err != nil {
					h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInternalError, "Failed to persist heartbeat", nil))
					continue
				}
			}

			session.markHeartbeat(nowMs)
			latencyMap := extractLatencyMap(req.ExchangeStats)
			if len(latencyMap) > 0 {
				session.setExchangeLatencies(latencyMap)
			}
			if profile, ok := buildLatencyProfileFromExchangeStats(req.ExchangeStats, session.effectiveExchs); ok {
				session.setLatencyProfile(profile)
			}
			if req.LoadIndex != nil || req.TradeLoadIndex != nil {
				load := session.loadIndex
				tradeLoad := session.tradeLoadIndex
				if req.LoadIndex != nil {
					load = normalizeUnitRange(*req.LoadIndex)
				}
				if req.TradeLoadIndex != nil {
					tradeLoad = normalizeUnitRange(*req.TradeLoadIndex)
				}
				if req.TradeLoadIndex == nil && req.LoadIndex != nil {
					tradeLoad = load
				}
				session.setTelemetry(load, tradeLoad)
			}
			h.upsertSessionSnapshot(session)
			h.syncRuntimeHeartbeat(nowMs / 1000)
			if h.heartbeatTimeout > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(h.heartbeatTimeout))
			}
			h.accessLog.Info("ws_heartbeat", "conn_id", connID, "request_id", msgRequestID, "trader_id", req.TraderID, "session_id", session.sessionID, "status", req.Status, "last_heartbeat_ms", session.lastHeartbeatMs, "session_state", session.state)

			if msg.Type == msgTypeRequest {
				h.sendEnvelope(conn, connID, msgID, newHeartbeatAckEnvelope(msgRequestID, heartbeatAck{
					Status:     "ok",
					TraderID:   req.TraderID,
					SessionID:  session.sessionID,
					ServerTime: session.lastHeartbeatMs,
				}))
			}
		case actionLatencyTestResp:
			if session.traderID == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidMessage, "trader.register is required before latency.test_result", nil))
				continue
			}

			var req latencyTestResultRequest
			if err := json.Unmarshal(msg.Payload, &req); err != nil {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "Invalid latency.test_result payload", invalidPayloadDetails("payload", "payload", "object", err.Error())))
				continue
			}

			if req.TraderID == "" {
				req.TraderID = session.traderID
			}
			if req.TraderID != session.traderID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id does not match registered trader", mismatchFieldDetails("trader_id", "payload.trader_id", session.traderID, req.TraderID)))
				continue
			}

			if req.SessionID != "" && req.SessionID != session.sessionID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "session_id does not match active session", mismatchFieldDetails("session_id", "payload.session_id", session.sessionID, req.SessionID)))
				continue
			}

			latencyMap := extractLatencyMap(req.ExchangeStats)
			if len(latencyMap) == 0 {
				latencyMap = mapLatencyResults(req)
			}
			if len(latencyMap) == 0 {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "latency.test_result must include exchange latency values", requiredFieldDetails("exchange_stats|results", "payload.exchange_stats|payload.results", "object|array")))
				continue
			}

			session.setExchangeLatencies(latencyMap)
			if profile, ok := buildLatencyProfileFromLatencyMap(latencyMap, session.effectiveExchs); ok {
				session.setLatencyProfile(profile)
			}
			h.upsertSessionSnapshot(session)

			if msg.Type == msgTypeRequest {
				h.sendEnvelope(conn, connID, msgID, newLatencyTestResultAckEnvelope(msgRequestID, latencyTestResultAck{
					Status:     "ok",
					TraderID:   session.traderID,
					SessionID:  session.sessionID,
					ServerTime: time.Now().UnixMilli(),
				}))
			}
			h.accessLog.Info("ws_latency_test_result", "conn_id", connID, "request_id", msgRequestID, "trader_id", session.traderID, "session_id", session.sessionID, "exchange_count", len(latencyMap))
		default:
			if now.Sub(unknownWindowStart) >= h.unknownActionWindow {
				unknownWindowStart = now
				unknownActionCount = 0
			}
			unknownActionCount++
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errUnknownAction, "Unsupported action", map[string]interface{}{"action": msg.Action}))
			if unknownActionCount >= h.maxUnknownActions {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errActionFlood, "Too many unknown actions", map[string]interface{}{"max_unknown_actions": h.maxUnknownActions}))
				return
			}
		}
	}
}

func (h *Handler) sendEnvelope(conn *websocket.Conn, connID string, msgID int64, msg envelope) {
	b, err := json.Marshal(msg)
	if err != nil {
		h.outLog.Error("ws_out_error", "conn_id", connID, "msg_id", msgID, "error", fmt.Sprintf("marshal ws response: %v", err))
		return
	}

	if err := h.writeMessage(conn, websocket.TextMessage, b); err != nil {
		h.outLog.Warn("ws_out_error", "conn_id", connID, "msg_id", msgID, "error", err)
		return
	}

	h.outLog.Info("ws_out", "event", msg.Action, "conn_id", connID, "msg_id", msgID, "request_id", msg.RequestID)
}

func (h *Handler) DispatchLatencyTest(_ context.Context, sessionID string, traderID string, exchanges []string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	h.connectionsMu.RLock()
	entry, ok := h.connections[sessionID]
	h.connectionsMu.RUnlock()
	if !ok || entry == nil {
		return fmt.Errorf("session %s is not connected", sessionID)
	}

	if traderID != "" && entry.traderID != "" && traderID != entry.traderID {
		return fmt.Errorf("session %s belongs to trader %s, got %s", sessionID, entry.traderID, traderID)
	}

	requestID := fmt.Sprintf("lat-test-%d", time.Now().UnixMilli())
	msg := newLatencyTestEnvelope(requestID, latencyTestRequest{
		Exchanges:   normalizeExchangeList(exchanges),
		Reason:      "periodic_retest",
		RequestedAt: time.Now().UnixMilli(),
	})
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal latency.test: %w", err)
	}

	if err := h.writeTextMessage(entry, b); err != nil {
		return fmt.Errorf("send latency.test: %w", err)
	}

	h.outLog.Info("ws_out", "event", actionLatencyTest, "conn_id", entry.sessionID, "msg_id", int64(0), "request_id", requestID)
	return nil
}

func (h *Handler) addConnection(sessionID string, conn *websocket.Conn) *wsConnectionEntry {
	entry := &wsConnectionEntry{sessionID: sessionID, conn: conn}
	h.connectionsMu.Lock()
	h.connections[sessionID] = entry
	h.connectionsByConn[conn] = entry
	h.connectionsMu.Unlock()
	return entry
}

func (h *Handler) bindConnectionTrader(sessionID string, traderID string) {
	if sessionID == "" || traderID == "" {
		return
	}
	h.connectionsMu.Lock()
	if entry, ok := h.connections[sessionID]; ok && entry != nil {
		entry.traderID = traderID
	}
	h.connectionsMu.Unlock()
}

func (h *Handler) removeConnection(sessionID string) {
	if sessionID == "" {
		return
	}
	h.connectionsMu.Lock()
	entry, ok := h.connections[sessionID]
	if ok {
		delete(h.connections, sessionID)
		if entry != nil && entry.conn != nil {
			delete(h.connectionsByConn, entry.conn)
		}
	}
	h.connectionsMu.Unlock()
}

func (h *Handler) writeMessage(conn *websocket.Conn, msgType int, data []byte) error {
	h.connectionsMu.RLock()
	entry := h.connectionsByConn[conn]
	h.connectionsMu.RUnlock()
	if entry == nil {
		return conn.WriteMessage(msgType, data)
	}

	entry.writeMu.Lock()
	defer entry.writeMu.Unlock()
	if err := entry.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	err := entry.conn.WriteMessage(msgType, data)
	_ = entry.conn.SetWriteDeadline(time.Time{})
	return err
}

func (h *Handler) writeTextMessage(entry *wsConnectionEntry, data []byte) error {
	if entry == nil || entry.conn == nil {
		return fmt.Errorf("connection entry is nil")
	}
	entry.writeMu.Lock()
	defer entry.writeMu.Unlock()
	if err := entry.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	err := entry.conn.WriteMessage(websocket.TextMessage, data)
	_ = entry.conn.SetWriteDeadline(time.Time{})
	return err
}

func (h *Handler) GetStats() Stats {
	return Stats{
		ActiveConnections: h.active.Load(),
		TotalConnections:  h.total.Load(),
		LastConnectUnix:   h.lastSeen.Load(),
	}
}

// GetTraderSnapshots returns a copy of current runtime trader session snapshots.
func (h *Handler) GetTraderSnapshots() []TraderSnapshot {
	h.sessionsMu.RLock()
	defer h.sessionsMu.RUnlock()

	items := make([]TraderSnapshot, 0, len(h.sessions))
	for _, snapshot := range h.sessions {
		items = append(items, snapshot)
	}
	return items
}

func (h *Handler) upsertSessionSnapshot(session *sessionRuntime) {
	if session == nil || session.traderID == "" {
		return
	}

	h.sessionsMu.Lock()
	h.sessions[session.sessionID] = TraderSnapshot{
		TraderID:           session.traderID,
		TraderDBID:         session.traderDBID,
		SessionID:          session.sessionID,
		State:              string(session.state),
		RegisteredAtUnix:   session.registeredAtMs / 1000,
		LastHeartbeatUnix:  session.lastHeartbeatMs / 1000,
		TimedOutAtUnix:     session.timedOutAtMs / 1000,
		Role:               session.role,
		Capabilities:       append([]string(nil), session.capabilities...),
		EffectiveExchanges: append([]string(nil), session.effectiveExchs...),
		LoadIndex:          session.loadIndex,
		TradeLoadIndex:     session.tradeLoadIndex,
		LatencyProfileMs:   session.latencyProfileMs,
		ExchangeLatencies:  cloneExchangeLatencies(session.exchangeLatencies),
	}
	h.sessionsMu.Unlock()
}

func cloneExchangeLatencies(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	items := make(map[string]float64, len(src))
	for k, v := range src {
		items[k] = v
	}
	return items
}

func (h *Handler) deleteSessionSnapshot(sessionID string) {
	if sessionID == "" {
		return
	}

	h.sessionsMu.Lock()
	delete(h.sessions, sessionID)
	h.sessionsMu.Unlock()
}

func (h *Handler) syncRuntimeState() {
	if h.stateManager == nil {
		return
	}
	h.stateManager.SetRuntimeWS(h.active.Load(), h.lastSeen.Load())
}

func (h *Handler) syncRuntimeHeartbeat(lastHeartbeatUnix int64) {
	if h.stateManager == nil {
		return
	}
	sink, ok := h.stateManager.(runtimeHeartbeatSink)
	if !ok {
		return
	}
	sink.SetRuntimeWSHeartbeat(lastHeartbeatUnix)
}

func (h *Handler) syncRuntimeTimeout(lastTimeoutUnix int64) {
	if h.stateManager == nil {
		return
	}
	sink, ok := h.stateManager.(runtimeTimeoutSink)
	if !ok {
		return
	}
	sink.IncrementRuntimeWSTimeout(lastTimeoutUnix)
}

func (h *Handler) withDBWriteRetry(op string, fn func(ctx context.Context) error) error {
	if h.persistence == nil {
		return nil
	}

	var lastErr error
	delay := h.dbRetry.InitialDelay

	for attempt := 1; attempt <= h.dbRetry.MaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := fn(ctx)
		cancel()
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt == h.dbRetry.MaxAttempts {
			break
		}

		h.accessLog.Warn("ws_db_retry", "operation", op, "attempt", attempt, "max_attempts", h.dbRetry.MaxAttempts, "retry_in", delay.String(), "error", err)
		time.Sleep(delay)
		delay = delay * 2
		if delay > h.dbRetry.MaxDelay {
			delay = h.dbRetry.MaxDelay
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", op, h.dbRetry.MaxAttempts, lastErr)
}

func generateConnID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

func resolveRequestID(requestID string, msgID int64) string {
	if requestID != "" {
		return requestID
	}
	return fmt.Sprintf("srv-%d", msgID)
}

func durationSecondsCeil(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		return 1
	}
	return secs
}

func normalizeDBRetryConfig(cfg DBRetryConfig) DBRetryConfig {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 1 * time.Second
	}
	if cfg.MaxDelay < cfg.InitialDelay {
		cfg.MaxDelay = cfg.InitialDelay
	}
	return cfg
}

func normalizeMaxPayloadBytes(v int) int {
	if v <= 0 {
		return 64 * 1024
	}
	return v
}

func normalizeMaxMessagesPerSec(v int) int {
	if v <= 0 {
		return 100
	}
	return v
}

func normalizeMaxUnknownActions(v int) int {
	if v <= 0 {
		return 5
	}
	return v
}

func normalizeUnknownActionWindow(v time.Duration) time.Duration {
	if v <= 0 {
		return 10 * time.Second
	}
	return v
}

func normalizeRequestDedupWindow(v time.Duration) time.Duration {
	if v <= 0 {
		return 1 * time.Minute
	}
	return v
}

func normalizeExchangeList(exchanges []string) []string {
	if len(exchanges) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(exchanges))
	items := make([]string, 0, len(exchanges))
	for _, exchange := range exchanges {
		norm := strings.ToLower(strings.TrimSpace(exchange))
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		items = append(items, norm)
	}
	return items
}

func mapLatencyResults(req latencyTestResultRequest) map[string]float64 {
	latencies := make(map[string]float64)

	if ex := strings.ToLower(strings.TrimSpace(req.Exchange)); ex != "" {
		if latency, ok := selectLatencyValue(req.WSLatencyMS, req.PingMS, req.OrderLatencyMS); ok {
			latencies[ex] = latency
		}
	}

	for _, item := range req.Results {
		ex := strings.ToLower(strings.TrimSpace(item.Exchange))
		if ex == "" {
			continue
		}
		if latency, ok := selectLatencyValue(item.WSLatencyMS, item.PingMS, item.OrderLatencyMS); ok {
			latencies[ex] = latency
		}
	}

	if len(latencies) == 0 {
		return nil
	}
	return latencies
}

func selectLatencyValue(primary *float64, alternatives ...*float64) (float64, bool) {
	if primary != nil && *primary >= 0 {
		return *primary, true
	}
	for _, item := range alternatives {
		if item == nil {
			continue
		}
		if *item < 0 {
			continue
		}
		return *item, true
	}
	return 0, false
}

func invalidPayloadDetails(field string, path string, expectedType string, reason string) map[string]interface{} {
	return map[string]interface{}{
		"field":         field,
		"path":          path,
		"expected_type": expectedType,
		"reason":        reason,
	}
}

func requiredFieldDetails(field string, path string, expectedType string) map[string]interface{} {
	return map[string]interface{}{
		"field":         field,
		"path":          path,
		"expected_type": expectedType,
		"reason":        "required",
	}
}

func mismatchFieldDetails(field string, path string, expected interface{}, received interface{}) map[string]interface{} {
	return map[string]interface{}{
		"field":    field,
		"path":     path,
		"expected": expected,
		"received": received,
		"reason":   "mismatch",
	}
}
