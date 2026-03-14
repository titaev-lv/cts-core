package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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
	TraderID          string `json:"trader_id"`
	SessionID         string `json:"session_id"`
	State             string `json:"state"`
	RegisteredAtUnix  int64  `json:"registered_at_unix"`
	LastHeartbeatUnix int64  `json:"last_heartbeat_unix"`
	TimedOutAtUnix    int64  `json:"timed_out_at_unix"`
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
	persistedSession := false
	requestIDs := make(map[string]time.Time)
	rateWindowStart := time.Now().UTC()
	rateWindowCount := 0
	unknownWindowStart := time.Now().UTC()
	unknownActionCount := 0

	msgID := int64(0)
	if err := conn.WriteMessage(websocket.TextMessage, []byte("{\"type\":\"connected\"}")); err == nil {
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
			if h.persistence != nil {
				var resolvedTraderID int
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
			h.upsertSessionSnapshot(session)
			if h.heartbeatTimeout > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(h.heartbeatTimeout))
			}
			timeoutSec := durationSecondsCeil(h.heartbeatTimeout)
			h.sendEnvelope(conn, connID, msgID, newRegisterAckEnvelope(msgRequestID, registerAck{
				Status:            "ok",
				TraderID:          req.TraderID,
				SessionID:         session.sessionID,
				SessionTimeoutSec: timeoutSec,
				ServerTime:        nowMs,
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

	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		h.outLog.Warn("ws_out_error", "conn_id", connID, "msg_id", msgID, "error", err)
		return
	}

	h.outLog.Info("ws_out", "event", msg.Action, "conn_id", connID, "msg_id", msgID, "request_id", msg.RequestID)
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
		TraderID:          session.traderID,
		SessionID:         session.sessionID,
		State:             string(session.state),
		RegisteredAtUnix:  session.registeredAtMs / 1000,
		LastHeartbeatUnix: session.lastHeartbeatMs / 1000,
		TimedOutAtUnix:    session.timedOutAtMs / 1000,
	}
	h.sessionsMu.Unlock()
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
