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
	ActiveWSConnections         int64 `json:"active_ws_connections"`
	LastWSConnectUnix           int64 `json:"last_ws_connect_unix"`
	LastWSHeartbeatUnix         int64 `json:"last_ws_heartbeat_unix"`
	LastWSTimeoutUnix           int64 `json:"last_ws_timeout_unix"`
	WSTimeoutCount              int64 `json:"ws_timeout_count"`
	SchedulerCycleCount         int64 `json:"scheduler_cycle_count"`
	SchedulerLastCandidateCount int64 `json:"scheduler_last_candidate_count"`
	SchedulerLastRunUnix        int64 `json:"scheduler_last_run_unix"`
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
		Runtime: RuntimeState{},
	}
}
