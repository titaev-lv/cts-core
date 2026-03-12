package ws

import (
	"io"
	"strings"
	"time"

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

type sessionRuntime struct {
	state           sessionState
	sessionID       string
	traderID        string
	registeredAtMs  int64
	lastHeartbeatMs int64
	timedOutAtMs    int64
}

func newSessionRuntime(sessionID string) *sessionRuntime {
	return &sessionRuntime{
		state:     sessionStateConnected,
		sessionID: sessionID,
	}
}

func (s *sessionRuntime) markRegistered(traderID string, nowMs int64) {
	s.traderID = traderID
	s.registeredAtMs = nowMs
	s.state = sessionStateRegistered
}

func (s *sessionRuntime) markHeartbeat(nowMs int64) {
	s.lastHeartbeatMs = nowMs
	s.state = sessionStateActive
}

func (s *sessionRuntime) shouldTimeout(nowMs int64, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}

	if s.state != sessionStateRegistered && s.state != sessionStateActive {
		return false
	}

	lastActivityMs := s.registeredAtMs
	if s.lastHeartbeatMs > 0 {
		lastActivityMs = s.lastHeartbeatMs
	}

	if lastActivityMs == 0 {
		return false
	}

	return nowMs-lastActivityMs >= timeout.Milliseconds()
}

func (s *sessionRuntime) markTimedOut(nowMs int64) bool {
	if s.state == sessionStateDisconnected || s.state == sessionStateTimedOut {
		return false
	}

	s.state = sessionStateTimedOut
	s.timedOutAtMs = nowMs
	return true
}

func (s *sessionRuntime) markDisconnected() {
	s.state = sessionStateDisconnected
}

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
