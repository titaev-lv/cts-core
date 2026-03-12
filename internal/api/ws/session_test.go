package ws

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClassifyDisconnectReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want disconnectReason
	}{
		{
			name: "nil error means server shutdown",
			err:  nil,
			want: disconnectReasonServerShutdown,
		},
		{
			name: "normal close frame",
			err:  &websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "bye"},
			want: disconnectReasonClientClose,
		},
		{
			name: "read timeout",
			err:  errors.New("read tcp 127.0.0.1:8080: i/o timeout"),
			want: disconnectReasonTimeout,
		},
		{
			name: "unexpected eof",
			err:  errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"),
			want: disconnectReasonClientClose,
		},
		{
			name: "generic read error",
			err:  fmt.Errorf("socket failed"),
			want: disconnectReasonReadError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDisconnectReason(tc.err)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSessionLifecycleTransitions(t *testing.T) {
	s := newSessionRuntime("sess-1")
	if s.state != sessionStateConnected {
		t.Fatalf("expected initial state %q, got %q", sessionStateConnected, s.state)
	}

	s.markRegistered("trader-1", 1000)
	if s.state != sessionStateRegistered {
		t.Fatalf("expected state %q, got %q", sessionStateRegistered, s.state)
	}
	if s.traderID != "trader-1" {
		t.Fatalf("expected trader id trader-1, got %q", s.traderID)
	}

	s.markHeartbeat(2000)
	if s.state != sessionStateActive {
		t.Fatalf("expected state %q, got %q", sessionStateActive, s.state)
	}
	if s.lastHeartbeatMs != 2000 {
		t.Fatalf("expected lastHeartbeatMs=2000, got %d", s.lastHeartbeatMs)
	}

	if !s.shouldTimeout(18000, 15*time.Second) {
		t.Fatalf("expected timeout=true when inactivity exceeds threshold")
	}

	if changed := s.markTimedOut(18000); !changed {
		t.Fatalf("expected markTimedOut to change state")
	}
	if s.state != sessionStateTimedOut {
		t.Fatalf("expected state %q, got %q", sessionStateTimedOut, s.state)
	}

	s.markDisconnected()
	if s.state != sessionStateDisconnected {
		t.Fatalf("expected state %q, got %q", sessionStateDisconnected, s.state)
	}
}

func TestSessionShouldTimeoutUsesRegisterTimeBeforeFirstHeartbeat(t *testing.T) {
	s := newSessionRuntime("sess-2")
	s.markRegistered("trader-2", 1000)

	if s.shouldTimeout(5000, 15*time.Second) {
		t.Fatalf("expected timeout=false before threshold")
	}

	if !s.shouldTimeout(17000, 15*time.Second) {
		t.Fatalf("expected timeout=true using register time when no heartbeat yet")
	}
}
