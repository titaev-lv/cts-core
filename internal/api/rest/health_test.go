package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/titaev-lv/cts-core/internal/state"
)

type testDBPinger struct {
	err error
}

func (d testDBPinger) Ping() error {
	return d.err
}

func TestHealthIncludesWSTelemetryFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mgr, err := state.NewManager(state.ManagerConfig{
		StateFile:    t.TempDir() + "/daemon.state",
		SyncInterval: time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("new state manager: %v", err)
	}

	mgr.SetRuntimeWSHeartbeat(1700000001)
	mgr.IncrementRuntimeWSTimeout(1700000002)
	mgr.SetRuntimeScheduler(11, 4, 1700000003)
	mgr.SetSchedulerLastAssignStatus("success")
	mgr.SetSchedulerAssignLatencyMs(12.5)
	mgr.SetSchedulerScoreDistribution(100, 200)
	mgr.RecordSchedulerAssignAttempt("success")
	mgr.RecordSchedulerResourceRejection("hard_limit")

	h := &HealthHandler{
		dbClient:       testDBPinger{},
		stateManager:   mgr,
		startedAt:      time.Now().Add(-10 * time.Second),
		serviceName:    "cts-core",
		serviceVersion: "test",
	}

	r := gin.New()
	r.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	components := body["components"].(map[string]interface{})
	websocketComp := components["websocket"].(map[string]interface{})
	tradersComp := components["traders"].(map[string]interface{})
	schedulerComp := components["scheduler"].(map[string]interface{})

	if websocketComp["last_heartbeat_unix"].(float64) != 1700000001 {
		t.Fatalf("unexpected websocket.last_heartbeat_unix: %v", websocketComp["last_heartbeat_unix"])
	}
	if websocketComp["last_timeout_unix"].(float64) != 1700000002 {
		t.Fatalf("unexpected websocket.last_timeout_unix: %v", websocketComp["last_timeout_unix"])
	}
	if websocketComp["timeout_count"].(float64) != 1 {
		t.Fatalf("unexpected websocket.timeout_count: %v", websocketComp["timeout_count"])
	}

	if tradersComp["last_heartbeat_unix"].(float64) != 1700000001 {
		t.Fatalf("unexpected traders.last_heartbeat_unix: %v", tradersComp["last_heartbeat_unix"])
	}
	if tradersComp["last_timeout_unix"].(float64) != 1700000002 {
		t.Fatalf("unexpected traders.last_timeout_unix: %v", tradersComp["last_timeout_unix"])
	}
	if tradersComp["timeout_count"].(float64) != 1 {
		t.Fatalf("unexpected traders.timeout_count: %v", tradersComp["timeout_count"])
	}

	if schedulerComp["cycle_count"].(float64) != 11 {
		t.Fatalf("unexpected scheduler.cycle_count: %v", schedulerComp["cycle_count"])
	}
	if schedulerComp["last_candidate_count"].(float64) != 4 {
		t.Fatalf("unexpected scheduler.last_candidate_count: %v", schedulerComp["last_candidate_count"])
	}
	if schedulerComp["last_run_unix"].(float64) != 1700000003 {
		t.Fatalf("unexpected scheduler.last_run_unix: %v", schedulerComp["last_run_unix"])
	}
	if schedulerComp["last_assign_status"].(string) != "success" {
		t.Fatalf("unexpected scheduler.last_assign_status: %v", schedulerComp["last_assign_status"])
	}
	if schedulerComp["assign_latency_ms"].(float64) != 12.5 {
		t.Fatalf("unexpected scheduler.assign_latency_ms: %v", schedulerComp["assign_latency_ms"])
	}
}

func TestHealthDatabaseErrorStillReturnsTelemetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mgr, err := state.NewManager(state.ManagerConfig{
		StateFile:    t.TempDir() + "/daemon.state",
		SyncInterval: time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("new state manager: %v", err)
	}

	mgr.SetRuntimeWSHeartbeat(42)

	h := &HealthHandler{
		dbClient:       testDBPinger{err: errors.New("db down")},
		stateManager:   mgr,
		startedAt:      time.Now().Add(-10 * time.Second),
		serviceName:    "cts-core",
		serviceVersion: "test",
	}

	r := gin.New()
	r.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	components := body["components"].(map[string]interface{})
	websocketComp := components["websocket"].(map[string]interface{})
	if websocketComp["last_heartbeat_unix"].(float64) != 42 {
		t.Fatalf("unexpected websocket.last_heartbeat_unix: %v", websocketComp["last_heartbeat_unix"])
	}
}

func TestNewHealthHandler_NilDBClientDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := NewHealthHandler(nil, HealthHandlerOptions{
		StartedAt:      time.Now().Add(-time.Second),
		ServiceName:    "cts-core",
		ServiceVersion: "test",
	})

	r := gin.New()
	r.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	components, ok := body["components"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected components map in response")
	}
	databaseComp, ok := components["database"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected database component map")
	}
	if databaseComp["status"] != "not_configured" {
		t.Fatalf("expected database status not_configured, got %v", databaseComp["status"])
	}
}
