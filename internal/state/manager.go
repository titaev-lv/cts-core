package state

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ManagerConfig struct {
	StateFile    string
	BackupDir    string
	MaxBackups   int
	SyncInterval time.Duration
}

type Manager struct {
	mu           sync.RWMutex
	state        *DaemonState
	logger       *slog.Logger
	stateFile    string
	backupDir    string
	maxBackups   int
	syncInterval time.Duration
	stopCh       chan struct{}
	closeOnce    sync.Once
	wg           sync.WaitGroup
}

func NewManager(cfg ManagerConfig, logger *slog.Logger) (*Manager, error) {
	if cfg.StateFile == "" {
		return nil, fmt.Errorf("state file path is required")
	}

	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 30 * time.Second
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 3
	}
	if cfg.BackupDir == "" {
		cfg.BackupDir = filepath.Join(filepath.Dir(cfg.StateFile), "backups")
	}
	if logger == nil {
		logger = slog.Default()
	}

	if err := os.MkdirAll(filepath.Dir(cfg.StateFile), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	if err := os.MkdirAll(cfg.BackupDir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	return &Manager{
		state:        NewDaemonState(),
		logger:       logger,
		stateFile:    cfg.StateFile,
		backupDir:    cfg.BackupDir,
		maxBackups:   cfg.MaxBackups,
		syncInterval: cfg.SyncInterval,
		stopCh:       make(chan struct{}),
	}, nil
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			m.logger.Info("state file not found, using empty state", "path", m.stateFile)
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	var loaded DaemonState
	if err := json.Unmarshal(data, &loaded); err != nil {
		m.logger.Warn("state file is corrupted, fallback to empty state", "path", m.stateFile, "error", err)
		m.state = NewDaemonState()
		return nil
	}

	if loaded.Version == "" {
		loaded.Version = "1.0"
	}
	if loaded.Runtime.SchedulerAssignAttempts == nil {
		loaded.Runtime.SchedulerAssignAttempts = map[string]int64{}
	}
	if loaded.Runtime.SchedulerResourceRejections == nil {
		loaded.Runtime.SchedulerResourceRejections = map[string]int64{}
	}
	if loaded.Runtime.WSDisconnectByReason == nil {
		loaded.Runtime.WSDisconnectByReason = map[string]int64{}
	}
	m.state = &loaded
	m.logger.Info("state loaded", "path", m.stateFile, "version", loaded.Version)
	return nil
}

func (m *Manager) Save() error {
	m.mu.Lock()
	m.state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(m.state, "", "  ")
	m.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := m.writeAtomically(data); err != nil {
		return err
	}
	return nil
}

func (m *Manager) StartBackgroundSync() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := m.Save(); err != nil {
					m.logger.Warn("background state save failed", "error", err)
					continue
				}
				if err := m.Backup(); err != nil {
					m.logger.Warn("background state backup failed", "error", err)
				}
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *Manager) Close() error {
	var closeErr error
	m.closeOnce.Do(func() {
		close(m.stopCh)
		m.wg.Wait()
		if err := m.Save(); err != nil {
			closeErr = fmt.Errorf("save state on close: %w", err)
			return
		}
		if err := m.Backup(); err != nil {
			closeErr = fmt.Errorf("backup state on close: %w", err)
			return
		}
		m.logger.Info("state manager closed")
	})
	return closeErr
}

func (m *Manager) SetServerStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Server.Status = status
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetRuntimeWS(active int64, lastConnectUnix int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.ActiveWSConnections = active
	m.state.Runtime.LastWSConnectUnix = lastConnectUnix
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetRuntimeWSPing(lastPingUnix int64, lastPongUnix int64, lastRTTMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lastPingUnix < 0 {
		lastPingUnix = 0
	}
	if lastPongUnix < 0 {
		lastPongUnix = 0
	}
	if lastRTTMs < 0 {
		lastRTTMs = 0
	}
	m.state.Runtime.LastWSPingUnix = lastPingUnix
	m.state.Runtime.LastWSPongUnix = lastPongUnix
	m.state.Runtime.LastWSPingRTTMs = lastRTTMs
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetRuntimeWSHeartbeat(lastHeartbeatUnix int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.LastWSHeartbeatUnix = lastHeartbeatUnix
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) IncrementRuntimeWSTimeout(lastTimeoutUnix int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.LastWSTimeoutUnix = lastTimeoutUnix
	m.state.Runtime.WSTimeoutCount++
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetRuntimeWSDisconnect(total uint64, close4009 uint64, byReason map[string]uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state.Runtime.WSDisconnectTotal = clampUint64ToInt64(total)
	m.state.Runtime.WSDisconnectClose4009 = clampUint64ToInt64(close4009)

	reasons := make(map[string]int64, len(byReason))
	for reason, count := range byReason {
		reasons[reason] = clampUint64ToInt64(count)
	}
	m.state.Runtime.WSDisconnectByReason = reasons
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetRuntimeScheduler(cycleCount int64, lastCandidateCount int64, lastRunUnix int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.SchedulerCycleCount = cycleCount
	m.state.Runtime.SchedulerLastCandidateCount = lastCandidateCount
	m.state.Runtime.SchedulerLastRunUnix = lastRunUnix
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) RecordSchedulerAssignAttempt(result string) {
	if result == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Runtime.SchedulerAssignAttempts == nil {
		m.state.Runtime.SchedulerAssignAttempts = map[string]int64{}
	}
	m.state.Runtime.SchedulerAssignAttempts[result]++
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetSchedulerAssignLatencyMs(latencyMs float64) {
	if latencyMs < 0 {
		latencyMs = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.SchedulerAssignLatencyMs = latencyMs
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetSchedulerScoreDistribution(p50 float64, p95 float64) {
	if p50 < 0 {
		p50 = 0
	}
	if p95 < 0 {
		p95 = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.SchedulerScoreP50 = p50
	m.state.Runtime.SchedulerScoreP95 = p95
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) SetSchedulerLastAssignStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Runtime.SchedulerLastAssignStatus = status
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) RecordSchedulerResourceRejection(reason string) {
	if reason == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Runtime.SchedulerResourceRejections == nil {
		m.state.Runtime.SchedulerResourceRejections = map[string]int64{}
	}
	m.state.Runtime.SchedulerResourceRejections[reason]++
	m.state.UpdatedAt = time.Now().UTC()
}

func (m *Manager) GetState() DaemonState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copyState := *m.state
	copyState.Runtime = cloneRuntimeState(copyState.Runtime)
	return copyState
}

func clampUint64ToInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func cloneRuntimeState(src RuntimeState) RuntimeState {
	out := src
	out.WSDisconnectByReason = cloneStringInt64Map(src.WSDisconnectByReason)
	out.SchedulerAssignAttempts = cloneStringInt64Map(src.SchedulerAssignAttempts)
	out.SchedulerResourceRejections = cloneStringInt64Map(src.SchedulerResourceRejections)
	return out
}

func cloneStringInt64Map(src map[string]int64) map[string]int64 {
	if src == nil {
		return nil
	}
	out := make(map[string]int64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
