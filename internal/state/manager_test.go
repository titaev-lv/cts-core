package state

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmpDir := t.TempDir()
	mgr, err := NewManager(ManagerConfig{
		StateFile:    filepath.Join(tmpDir, "daemon.state"),
		BackupDir:    filepath.Join(tmpDir, "backups"),
		MaxBackups:   2,
		SyncInterval: 50 * time.Millisecond,
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return mgr
}

func TestLoadWhenFileMissing(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	state := mgr.GetState()
	if state.Version == "" {
		t.Fatalf("expected state version to be set")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetServerStatus("running")
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	mgr2, err := NewManager(ManagerConfig{
		StateFile:  mgr.stateFile,
		BackupDir:  mgr.backupDir,
		MaxBackups: 2,
	}, slog.Default())
	if err != nil {
		t.Fatalf("NewManager() second instance error = %v", err)
	}
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load() second instance error = %v", err)
	}
	if got := mgr2.GetState().Server.Status; got != "running" {
		t.Fatalf("expected status=running, got %s", got)
	}
}

func TestLoadCorruptedFileFallback(t *testing.T) {
	mgr := newTestManager(t)
	if err := os.WriteFile(mgr.stateFile, []byte("{broken-json"), 0o600); err != nil {
		t.Fatalf("write corrupted state: %v", err)
	}
	if err := mgr.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := mgr.GetState().Version; got == "" {
		t.Fatalf("expected fallback state version, got empty")
	}
}

func TestBackupRotation(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	for i := 0; i < 4; i++ {
		time.Sleep(1 * time.Second)
		if err := mgr.Backup(); err != nil {
			t.Fatalf("Backup() error = %v", err)
		}
	}
	files, err := filepath.Glob(filepath.Join(mgr.backupDir, "daemon.state.*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) > mgr.maxBackups {
		t.Fatalf("expected backups <= %d, got %d", mgr.maxBackups, len(files))
	}
}

func TestUpdatedAtChangesOnMutations(t *testing.T) {
	mgr := newTestManager(t)
	before := mgr.GetState().UpdatedAt

	time.Sleep(10 * time.Millisecond)
	mgr.SetServerStatus("running")
	afterStatus := mgr.GetState().UpdatedAt
	if !afterStatus.After(before) {
		t.Fatalf("expected UpdatedAt to move forward after SetServerStatus")
	}

	time.Sleep(10 * time.Millisecond)
	mgr.SetRuntimeWS(3, time.Now().Unix())
	afterWS := mgr.GetState().UpdatedAt
	if !afterWS.After(afterStatus) {
		t.Fatalf("expected UpdatedAt to move forward after SetRuntimeWS")
	}

	time.Sleep(10 * time.Millisecond)
	mgr.SetRuntimeWSPing(101, 202, 15)
	afterPing := mgr.GetState().UpdatedAt
	if !afterPing.After(afterWS) {
		t.Fatalf("expected UpdatedAt to move forward after SetRuntimeWSPing")
	}
	statePing := mgr.GetState().Runtime
	if statePing.LastWSPingUnix != 101 || statePing.LastWSPongUnix != 202 || statePing.LastWSPingRTTMs != 15 {
		t.Fatalf("unexpected ping state: %+v", statePing)
	}

	time.Sleep(10 * time.Millisecond)
	heartbeatUnix := time.Now().Unix()
	mgr.SetRuntimeWSHeartbeat(heartbeatUnix)
	afterHB := mgr.GetState().UpdatedAt
	if !afterHB.After(afterWS) {
		t.Fatalf("expected UpdatedAt to move forward after SetRuntimeWSHeartbeat")
	}
	if got := mgr.GetState().Runtime.LastWSHeartbeatUnix; got != heartbeatUnix {
		t.Fatalf("expected LastWSHeartbeatUnix=%d, got %d", heartbeatUnix, got)
	}

	time.Sleep(10 * time.Millisecond)
	timeoutUnix := time.Now().Unix()
	mgr.IncrementRuntimeWSTimeout(timeoutUnix)
	afterTimeout := mgr.GetState().UpdatedAt
	if !afterTimeout.After(afterHB) {
		t.Fatalf("expected UpdatedAt to move forward after IncrementRuntimeWSTimeout")
	}
	state := mgr.GetState()
	if state.Runtime.LastWSTimeoutUnix != timeoutUnix {
		t.Fatalf("expected LastWSTimeoutUnix=%d, got %d", timeoutUnix, state.Runtime.LastWSTimeoutUnix)
	}
	if state.Runtime.WSTimeoutCount != 1 {
		t.Fatalf("expected WSTimeoutCount=1, got %d", state.Runtime.WSTimeoutCount)
	}

	time.Sleep(10 * time.Millisecond)
	disconnectByReason := map[string]uint64{"timeout": 2, "close_4009_sequence_gap": 1}
	mgr.SetRuntimeWSDisconnect(3, 1, disconnectByReason)
	afterDisconnect := mgr.GetState().UpdatedAt
	if !afterDisconnect.After(afterTimeout) {
		t.Fatalf("expected UpdatedAt to move forward after SetRuntimeWSDisconnect")
	}
	state = mgr.GetState()
	if state.Runtime.WSDisconnectTotal != 3 {
		t.Fatalf("expected WSDisconnectTotal=3, got %d", state.Runtime.WSDisconnectTotal)
	}
	if state.Runtime.WSDisconnectClose4009 != 1 {
		t.Fatalf("expected WSDisconnectClose4009=1, got %d", state.Runtime.WSDisconnectClose4009)
	}
	if state.Runtime.WSDisconnectByReason["timeout"] != 2 {
		t.Fatalf("expected WSDisconnectByReason[timeout]=2, got %d", state.Runtime.WSDisconnectByReason["timeout"])
	}
	if state.Runtime.WSDisconnectByReason["close_4009_sequence_gap"] != 1 {
		t.Fatalf("expected WSDisconnectByReason[close_4009_sequence_gap]=1, got %d", state.Runtime.WSDisconnectByReason["close_4009_sequence_gap"])
	}

	time.Sleep(10 * time.Millisecond)
	schedulerRunUnix := time.Now().Unix()
	mgr.SetRuntimeScheduler(7, 3, schedulerRunUnix)
	afterScheduler := mgr.GetState().UpdatedAt
	if !afterScheduler.After(afterTimeout) {
		t.Fatalf("expected UpdatedAt to move forward after SetRuntimeScheduler")
	}
	state = mgr.GetState()
	if state.Runtime.SchedulerCycleCount != 7 {
		t.Fatalf("expected SchedulerCycleCount=7, got %d", state.Runtime.SchedulerCycleCount)
	}
	if state.Runtime.SchedulerLastCandidateCount != 3 {
		t.Fatalf("expected SchedulerLastCandidateCount=3, got %d", state.Runtime.SchedulerLastCandidateCount)
	}
	if state.Runtime.SchedulerLastRunUnix != schedulerRunUnix {
		t.Fatalf("expected SchedulerLastRunUnix=%d, got %d", schedulerRunUnix, state.Runtime.SchedulerLastRunUnix)
	}

	time.Sleep(10 * time.Millisecond)
	mgr.RecordSchedulerAssignAttempt("success")
	mgr.SetSchedulerAssignLatencyMs(12.5)
	mgr.SetSchedulerScoreDistribution(110.0, 240.0)
	mgr.SetSchedulerLastAssignStatus("success")
	mgr.RecordSchedulerResourceRejection("hard_limit")
	state = mgr.GetState()
	if state.Runtime.SchedulerAssignAttempts["success"] != 1 {
		t.Fatalf("expected SchedulerAssignAttempts[success]=1, got %d", state.Runtime.SchedulerAssignAttempts["success"])
	}
	if state.Runtime.SchedulerAssignLatencyMs != 12.5 {
		t.Fatalf("expected SchedulerAssignLatencyMs=12.5, got %f", state.Runtime.SchedulerAssignLatencyMs)
	}
	if state.Runtime.SchedulerScoreP50 != 110.0 || state.Runtime.SchedulerScoreP95 != 240.0 {
		t.Fatalf("unexpected scheduler score distribution p50=%f p95=%f", state.Runtime.SchedulerScoreP50, state.Runtime.SchedulerScoreP95)
	}
	if state.Runtime.SchedulerLastAssignStatus != "success" {
		t.Fatalf("expected SchedulerLastAssignStatus=success, got %s", state.Runtime.SchedulerLastAssignStatus)
	}
	if state.Runtime.SchedulerResourceRejections["hard_limit"] != 1 {
		t.Fatalf("expected SchedulerResourceRejections[hard_limit]=1, got %d", state.Runtime.SchedulerResourceRejections["hard_limit"])
	}
}

func TestSetRuntimeWSDisconnectCopiesMap(t *testing.T) {
	mgr := newTestManager(t)

	input := map[string]uint64{"timeout": 4}
	mgr.SetRuntimeWSDisconnect(4, 0, input)
	input["timeout"] = 99

	state := mgr.GetState()
	if state.Runtime.WSDisconnectByReason["timeout"] != 4 {
		t.Fatalf("expected copied WS disconnect map value 4, got %d", state.Runtime.WSDisconnectByReason["timeout"])
	}
}

func TestGetStateReturnsDeepCopyForRuntimeMaps(t *testing.T) {
	mgr := newTestManager(t)

	mgr.SetRuntimeWSDisconnect(3, 1, map[string]uint64{"timeout": 2})
	mgr.RecordSchedulerAssignAttempt("success")
	mgr.RecordSchedulerResourceRejection("hard_limit")

	snapshot := mgr.GetState()
	snapshot.Runtime.WSDisconnectByReason["timeout"] = 999
	snapshot.Runtime.SchedulerAssignAttempts["success"] = 999
	snapshot.Runtime.SchedulerResourceRejections["hard_limit"] = 999

	fresh := mgr.GetState()
	if fresh.Runtime.WSDisconnectByReason["timeout"] != 2 {
		t.Fatalf("expected internal WSDisconnectByReason[timeout]=2, got %d", fresh.Runtime.WSDisconnectByReason["timeout"])
	}
	if fresh.Runtime.SchedulerAssignAttempts["success"] != 1 {
		t.Fatalf("expected internal SchedulerAssignAttempts[success]=1, got %d", fresh.Runtime.SchedulerAssignAttempts["success"])
	}
	if fresh.Runtime.SchedulerResourceRejections["hard_limit"] != 1 {
		t.Fatalf("expected internal SchedulerResourceRejections[hard_limit]=1, got %d", fresh.Runtime.SchedulerResourceRejections["hard_limit"])
	}
}
