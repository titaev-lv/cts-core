package state

import "time"

type DaemonState struct {
	Version   string       `json:"version"`
	UpdatedAt time.Time    `json:"updated_at"`
	Server    ServerState  `json:"server"`
	Runtime   RuntimeState `json:"runtime"`
}

type ServerState struct {
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
}

type RuntimeState struct {
	ActiveWSConnections         int64            `json:"active_ws_connections"`
	LastWSConnectUnix           int64            `json:"last_ws_connect_unix"`
	LastWSPingUnix              int64            `json:"last_ws_ping_unix"`
	LastWSPongUnix              int64            `json:"last_ws_pong_unix"`
	LastWSPingRTTMs             int64            `json:"last_ws_ping_rtt_ms"`
	LastWSHeartbeatUnix         int64            `json:"last_ws_heartbeat_unix"`
	LastWSTimeoutUnix           int64            `json:"last_ws_timeout_unix"`
	WSTimeoutCount              int64            `json:"ws_timeout_count"`
	WSDisconnectTotal           int64            `json:"ws_disconnect_total"`
	WSDisconnectClose4009       int64            `json:"ws_disconnect_close_4009"`
	WSDisconnectByReason        map[string]int64 `json:"ws_disconnect_by_reason"`
	SchedulerCycleCount         int64            `json:"scheduler_cycle_count"`
	SchedulerLastCandidateCount int64            `json:"scheduler_last_candidate_count"`
	SchedulerLastRunUnix        int64            `json:"scheduler_last_run_unix"`
	SchedulerLastAssignStatus   string           `json:"scheduler_last_assign_status"`
	SchedulerAssignLatencyMs    float64          `json:"scheduler_assign_latency_ms"`
	SchedulerScoreP50           float64          `json:"scheduler_score_p50"`
	SchedulerScoreP95           float64          `json:"scheduler_score_p95"`
	SchedulerAssignAttempts     map[string]int64 `json:"scheduler_assign_attempts"`
	SchedulerResourceRejections map[string]int64 `json:"scheduler_resource_rejections"`
}

func NewDaemonState() *DaemonState {
	now := time.Now().UTC()
	return &DaemonState{
		Version:   "1.0",
		UpdatedAt: now,
		Server: ServerState{
			StartedAt: now,
			Status:    "starting",
		},
		Runtime: RuntimeState{
			WSDisconnectByReason:        map[string]int64{},
			SchedulerAssignAttempts:     map[string]int64{},
			SchedulerResourceRejections: map[string]int64{},
		},
	}
}
