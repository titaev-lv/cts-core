package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/titaev-lv/cts-core/internal/api/middleware"
	"github.com/titaev-lv/cts-core/internal/logger"
)

type Handler struct {
	upgrader          websocket.Upgrader
	accessLog         *slog.Logger
	outLog            *slog.Logger
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	stateManager      runtimeStateSink
	active            atomic.Int64
	total             atomic.Int64
	lastSeen          atomic.Int64
}

type runtimeStateSink interface {
	SetRuntimeWS(active int64, lastConnectUnix int64)
}

type HandlerOptions struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	StateManager      runtimeStateSink
}

type Stats struct {
	ActiveConnections int64 `json:"active_connections"`
	TotalConnections  int64 `json:"total_connections"`
	LastConnectUnix   int64 `json:"last_connect_unix,omitempty"`
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
		accessLog:         logger.GetWSAccess("ws"),
		outLog:            logger.GetWSOut("ws"),
		heartbeatInterval: heartbeatInterval,
		heartbeatTimeout:  heartbeatTimeout,
		stateManager:      opts.StateManager,
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
			if reason == disconnectReasonTimeout && session.markTimedOut(nowMs) {
				h.accessLog.Warn("ws_timeout", "conn_id", connID, "request_id", requestID, "trader_id", session.traderID, "session_id", session.sessionID, "session_state", session.state, "timed_out_at_ms", session.timedOutAtMs, "last_heartbeat_ms", session.lastHeartbeatMs)
			}

			stateBeforeDisconnect := session.state
			session.markDisconnected()
			h.accessLog.Warn("ws_disconnect", "conn_id", connID, "request_id", requestID, "trader_id", session.traderID, "session_id", session.sessionID, "previous_session_state", stateBeforeDisconnect, "session_state", session.state, "disconnect_reason", reason, "last_heartbeat_ms", session.lastHeartbeatMs, "error", err)
			return
		}

		msgID++
		h.accessLog.Info("ws_in", "conn_id", connID, "msg_id", msgID, "request_id", requestID, "msg_type", msgType, "size_bytes", len(rawPayload))

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
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "Invalid register payload", map[string]interface{}{"error": err.Error()}))
				continue
			}

			if req.TraderID == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id is required", nil))
				continue
			}

			if req.Version == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "version is required", nil))
				continue
			}

			nowMs := time.Now().UnixMilli()
			session.markRegistered(req.TraderID, nowMs)
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
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "Invalid heartbeat payload", map[string]interface{}{"error": err.Error()}))
				continue
			}

			if req.TraderID == "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id is required", nil))
				continue
			}

			if req.TraderID != session.traderID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id does not match registered trader", map[string]interface{}{"registered_trader_id": session.traderID, "received_trader_id": req.TraderID}))
				continue
			}

			if req.SessionID != "" && req.SessionID != session.sessionID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "session_id does not match active session", map[string]interface{}{"session_id": req.SessionID}))
				continue
			}

			nowMs := time.Now().UnixMilli()
			session.markHeartbeat(nowMs)
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
			h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errUnknownAction, "Unsupported action", map[string]interface{}{"action": msg.Action}))
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

func (h *Handler) syncRuntimeState() {
	if h.stateManager == nil {
		return
	}
	h.stateManager.SetRuntimeWS(h.active.Load(), h.lastSeen.Load())
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
