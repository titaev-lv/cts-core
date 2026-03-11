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
	upgrader  websocket.Upgrader
	accessLog *slog.Logger
	outLog    *slog.Logger
	active    atomic.Int64
	total     atomic.Int64
	lastSeen  atomic.Int64
}

type Stats struct {
	ActiveConnections int64 `json:"active_connections"`
	TotalConnections  int64 `json:"total_connections"`
	LastConnectUnix   int64 `json:"last_connect_unix,omitempty"`
}

// NewHandler creates a WebSocket handler with logging.
func NewHandler() *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(_ *http.Request) bool { return true },
		},
		accessLog: logger.GetWSAccess("ws"),
		outLog:    logger.GetWSOut("ws"),
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
	defer h.active.Add(-1)

	sessionID := generateConnID()
	registeredTraderID := ""
	sessionState := "connected"
	lastHeartbeatMs := int64(0)

	msgID := int64(0)
	if err := conn.WriteMessage(websocket.TextMessage, []byte("{\"type\":\"connected\"}")); err == nil {
		msgID++
		h.outLog.Info("ws_out", "event", "connected", "conn_id", connID, "msg_id", msgID, "request_id", requestID)
	}

	for {
		msgType, rawPayload, err := conn.ReadMessage()
		if err != nil {
			h.accessLog.Warn("ws_disconnect", "conn_id", connID, "request_id", requestID, "trader_id", registeredTraderID, "error", err)
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

			if registeredTraderID != "" {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errDuplicateConnection, "Trader already registered for this connection", map[string]interface{}{"trader_id": registeredTraderID}))
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

			registeredTraderID = req.TraderID
			sessionState = "registered"
			h.sendEnvelope(conn, connID, msgID, newRegisterAckEnvelope(msgRequestID, registerAck{
				Status:            "ok",
				TraderID:          req.TraderID,
				SessionID:         sessionID,
				SessionTimeoutSec: 30,
				ServerTime:        time.Now().UnixMilli(),
			}))
			h.accessLog.Info("ws_register", "conn_id", connID, "request_id", msgRequestID, "trader_id", req.TraderID, "version", req.Version, "region", req.Region)
		case actionTraderHeartbeat:
			if registeredTraderID == "" {
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

			if req.TraderID != registeredTraderID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "trader_id does not match registered trader", map[string]interface{}{"registered_trader_id": registeredTraderID, "received_trader_id": req.TraderID}))
				continue
			}

			if req.SessionID != "" && req.SessionID != sessionID {
				h.sendEnvelope(conn, connID, msgID, newErrorEnvelope(msgRequestID, errInvalidPayload, "session_id does not match active session", map[string]interface{}{"session_id": req.SessionID}))
				continue
			}

			lastHeartbeatMs = time.Now().UnixMilli()
			sessionState = "active"
			h.accessLog.Info("ws_heartbeat", "conn_id", connID, "request_id", msgRequestID, "trader_id", req.TraderID, "session_id", sessionID, "status", req.Status, "last_heartbeat_ms", lastHeartbeatMs, "session_state", sessionState)

			if msg.Type == msgTypeRequest {
				h.sendEnvelope(conn, connID, msgID, newHeartbeatAckEnvelope(msgRequestID, heartbeatAck{
					Status:     "ok",
					TraderID:   req.TraderID,
					SessionID:  sessionID,
					ServerTime: lastHeartbeatMs,
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
