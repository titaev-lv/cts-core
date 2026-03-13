package state

import (
	"encoding/json"
	"fmt"
	"log/slog"
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

func (m *Manager) GetState() DaemonState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.state
}
