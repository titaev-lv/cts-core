package ws

import (
	"encoding/json"
	"time"
)

const (
	msgTypeRequest  = "request"
	msgTypeResponse = "response"

	actionTraderRegister = "trader.register"
	actionRegisterAck    = "trader.register_ack"
	actionError          = "error"
)

const (
	errInvalidMessage      = "INVALID_MESSAGE"
	errInvalidPayload      = "INVALID_PAYLOAD"
	errUnknownAction       = "UNKNOWN_ACTION"
	errDuplicateConnection = "DUPLICATE_CONNECTION"
)

type envelope struct {
	Type      string          `json:"type"`
	Action    string          `json:"action"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	TS        int64           `json:"ts,omitempty"`
}

type registerRequest struct {
	TraderID     string                 `json:"trader_id"`
	Version      string                 `json:"version"`
	Region       string                 `json:"region"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Resources    map[string]interface{} `json:"resources,omitempty"`
	CurrentLoad  map[string]interface{} `json:"current_load,omitempty"`
}

type registerAck struct {
	Status            string `json:"status"`
	TraderID          string `json:"trader_id"`
	SessionID         string `json:"session_id"`
	SessionTimeoutSec int    `json:"session_timeout_sec"`
	ServerTime        int64  `json:"server_time"`
}

type errorPayload struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func newErrorEnvelope(requestID, code, message string, details map[string]interface{}) envelope {
	payload, _ := json.Marshal(errorPayload{
		Code:    code,
		Message: message,
		Details: details,
	})

	return envelope{
		Type:      msgTypeResponse,
		Action:    actionError,
		RequestID: requestID,
		Payload:   payload,
		TS:        time.Now().UnixMilli(),
	}
}

func newRegisterAckEnvelope(requestID string, ack registerAck) envelope {
	payload, _ := json.Marshal(ack)
	return envelope{
		Type:      msgTypeResponse,
		Action:    actionRegisterAck,
		RequestID: requestID,
		Payload:   payload,
		TS:        time.Now().UnixMilli(),
	}
}
