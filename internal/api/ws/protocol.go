package ws

import (
	"encoding/json"
	"time"
)

const (
	msgTypeRequest  = "request"
	msgTypeEvent    = "event"
	msgTypeResponse = "response"

	supportedProtocolVersion = "2"

	actionTraderRegister  = "trader.register"
	actionRegisterAck     = "trader.register_ack"
	actionTraderHeartbeat = "trader.heartbeat"
	actionHeartbeatAck    = "trader.heartbeat_ack"
	actionLatencyTest     = "latency.test"
	actionLatencyTestResp = "latency.test_result"
	actionLatencyTestAck  = "latency.test_result_ack"
	actionError           = "error"
)

const (
	errInvalidMessage      = "INVALID_MESSAGE"
	errInvalidPayload      = "INVALID_PAYLOAD"
	errUnknownAction       = "UNKNOWN_ACTION"
	errDuplicateConnection = "DUPLICATE_CONNECTION"
	errInternalError       = "INTERNAL_ERROR"
	errUnsupportedVersion  = "UNSUPPORTED_VERSION"
	errRateLimited         = "RATE_LIMITED"
	errDuplicateRequest    = "DUPLICATE_REQUEST"
	errMessageTooLarge     = "MESSAGE_TOO_LARGE"
	errActionFlood         = "ACTION_FLOOD"
)

var supportedWSErrorCodes = map[string]struct{}{
	errInvalidMessage:      {},
	errInvalidPayload:      {},
	errUnknownAction:       {},
	errDuplicateConnection: {},
	errInternalError:       {},
	errUnsupportedVersion:  {},
	errRateLimited:         {},
	errDuplicateRequest:    {},
	errMessageTooLarge:     {},
	errActionFlood:         {},
}

func isSupportedWSErrorCode(code string) bool {
	_, ok := supportedWSErrorCodes[code]
	return ok
}

type envelope struct {
	Type            string          `json:"type"`
	Action          string          `json:"action"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	RequestID       string          `json:"request_id,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	TS              int64           `json:"ts,omitempty"`
}

type registerRequest struct {
	TraderID     string                 `json:"trader_id"`
	Version      string                 `json:"version"`
	Region       string                 `json:"region"`
	Role         string                 `json:"role,omitempty"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Resources    map[string]interface{} `json:"resources,omitempty"`
	CurrentLoad  map[string]interface{} `json:"current_load,omitempty"`
}

type exchangeCatalogEntry struct {
	ExchangeID        int            `json:"exchange_id"`
	Code              string         `json:"code"`
	Name              string         `json:"name"`
	Enabled           bool           `json:"enabled"`
	MarketTypes       []string       `json:"market_types,omitempty"`
	WSPublicEndpoint  string         `json:"ws_public_endpoint,omitempty"`
	WSPrivateEndpoint string         `json:"ws_private_endpoint,omitempty"`
	RESTEndpoint      string         `json:"rest_endpoint,omitempty"`
	RateLimits        map[string]int `json:"rate_limits,omitempty"`
}

type registerAck struct {
	Status                 string                 `json:"status"`
	TraderID               string                 `json:"trader_id"`
	SessionID              string                 `json:"session_id"`
	SessionTimeoutSec      int                    `json:"session_timeout_sec"`
	ServerTime             int64                  `json:"server_time"`
	ExchangeCatalogVersion string                 `json:"exchange_catalog_version,omitempty"`
	AvailableExchanges     []exchangeCatalogEntry `json:"available_exchanges,omitempty"`
	EffectiveExchanges     []string               `json:"effective_exchanges,omitempty"`
}

type heartbeatRequest struct {
	TraderID       string                            `json:"trader_id"`
	SessionID      string                            `json:"session_id"`
	Status         string                            `json:"status,omitempty"`
	LoadIndex      *float64                          `json:"load_index,omitempty"`
	TradeLoadIndex *float64                          `json:"trade_load_index,omitempty"`
	ExchangeStats  map[string]heartbeatExchangeStats `json:"exchange_stats,omitempty"`
}

type heartbeatExchangeStats struct {
	LatencyMS *float64 `json:"latency_ms,omitempty"`
}

type heartbeatAck struct {
	Status     string `json:"status"`
	TraderID   string `json:"trader_id"`
	SessionID  string `json:"session_id"`
	ServerTime int64  `json:"server_time"`
}

type latencyTestRequest struct {
	Exchanges   []string `json:"exchanges"`
	Reason      string   `json:"reason,omitempty"`
	RequestedAt int64    `json:"requested_at"`
}

type latencyTestResultRequest struct {
	TraderID       string                            `json:"trader_id"`
	SessionID      string                            `json:"session_id,omitempty"`
	ExchangeStats  map[string]heartbeatExchangeStats `json:"exchange_stats,omitempty"`
	Results        []latencyTestExchangeResult       `json:"results,omitempty"`
	Exchange       string                            `json:"exchange,omitempty"`
	PingMS         *float64                          `json:"ping_ms,omitempty"`
	WSLatencyMS    *float64                          `json:"ws_latency_ms,omitempty"`
	OrderLatencyMS *float64                          `json:"order_latency_ms,omitempty"`
	Timestamp      int64                             `json:"timestamp,omitempty"`
}

type latencyTestExchangeResult struct {
	Exchange       string   `json:"exchange"`
	PingMS         *float64 `json:"ping_ms,omitempty"`
	WSLatencyMS    *float64 `json:"ws_latency_ms,omitempty"`
	OrderLatencyMS *float64 `json:"order_latency_ms,omitempty"`
}

type latencyTestResultAck struct {
	Status     string `json:"status"`
	TraderID   string `json:"trader_id"`
	SessionID  string `json:"session_id"`
	ServerTime int64  `json:"server_time"`
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

func newHeartbeatAckEnvelope(requestID string, ack heartbeatAck) envelope {
	payload, _ := json.Marshal(ack)
	return envelope{
		Type:      msgTypeResponse,
		Action:    actionHeartbeatAck,
		RequestID: requestID,
		Payload:   payload,
		TS:        time.Now().UnixMilli(),
	}
}

func newLatencyTestEnvelope(requestID string, req latencyTestRequest) envelope {
	payload, _ := json.Marshal(req)
	return envelope{
		Type:      msgTypeRequest,
		Action:    actionLatencyTest,
		RequestID: requestID,
		Payload:   payload,
		TS:        time.Now().UnixMilli(),
	}
}

func newLatencyTestResultAckEnvelope(requestID string, ack latencyTestResultAck) envelope {
	payload, _ := json.Marshal(ack)
	return envelope{
		Type:      msgTypeResponse,
		Action:    actionLatencyTestAck,
		RequestID: requestID,
		Payload:   payload,
		TS:        time.Now().UnixMilli(),
	}
}
