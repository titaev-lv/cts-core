package ws

import (
	"io"
	"strings"

	"github.com/gorilla/websocket"
)

type sessionState string

const (
	sessionStateConnected    sessionState = "connected"
	sessionStateRegistered   sessionState = "registered"
	sessionStateActive       sessionState = "active"
	sessionStateTimedOut     sessionState = "timed_out"
	sessionStateDisconnected sessionState = "disconnected"
)

type disconnectReason string

const (
	disconnectReasonClientClose    disconnectReason = "client_close"
	disconnectReasonTimeout        disconnectReason = "timeout"
	disconnectReasonServerShutdown disconnectReason = "server_shutdown"
	disconnectReasonProtocolError  disconnectReason = "protocol_error"
	disconnectReasonReadError      disconnectReason = "read_error"
)

func classifyDisconnectReason(err error) disconnectReason {
	if err == nil {
		return disconnectReasonServerShutdown
	}

	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) {
		return disconnectReasonClientClose
	}

	if strings.Contains(err.Error(), "i/o timeout") {
		return disconnectReasonTimeout
	}

	if err == io.EOF || strings.Contains(err.Error(), "unexpected EOF") {
		return disconnectReasonClientClose
	}

	return disconnectReasonReadError
}
