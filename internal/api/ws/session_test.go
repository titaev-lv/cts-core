package ws

import (
	"errors"
	"fmt"
	"testing"

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
