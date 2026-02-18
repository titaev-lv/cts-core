# Phase 1.4: State Management

**Цель:** Реализовать persistent state management (daemon.state file + MySQL sync).

**Время:** ~2 дня (12-16 часов)

**Зависимости:**
- ✅ Phase 1.1 завершена (config, logger готовы)
- ✅ Phase 1.2 завершена (MySQL repository готов)
- state/ директория создана

---

## 1.4.1: State Data Structures (2 часа)

### Шаг 1: Создать internal/state/types.go

```go
package state

import (
    "time"
)

// DaemonState represents complete daemon state
type DaemonState struct {
    Version   string     `json:"version"`   // State format version (e.g., "1.0")
    UpdatedAt time.Time  `json:"updated_at"`
    
    // Core components
    Server      ServerState       `json:"server"`
    Traders     []TraderState     `json:"traders"`
    Sessions    []SessionState    `json:"sessions"`
    Orders      []OrderState      `json:"orders"`
}

// ServerState represents CTS-Core server state
type ServerState struct {
    StartedAt      time.Time `json:"started_at"`
    Status         string    `json:"status"` // running, stopping, stopped
    ActiveSessions int       `json:"active_sessions"`
    TotalOrders    int64     `json:"total_orders"`
    LastHeartbeat  time.Time `json:"last_heartbeat"`
}

// TraderState represents trader daemon state
type TraderState struct {
    TraderID       int64     `json:"trader_id"`
    Name           string    `json:"name"`
    IPAddress      string    `json:"ip_address"`
    Port           int       `json:"port"`
    Status         string    `json:"status"` // active, inactive, error
    LastHeartbeat  time.Time `json:"last_heartbeat"`
    LoadScore      int32     `json:"load_score"`
    ConnectionTime time.Time `json:"connection_time"` // When trader connected
}

// SessionState represents active WebSocket session
type SessionState struct {
    SessionID      int64     `json:"session_id"`
    TraderID       int64     `json:"trader_id"`
    UserID         int64     `json:"user_id"`
    SessionToken   string    `json:"session_token"`
    IPAddress      string    `json:"ip_address"`
    ConnectedAt    time.Time `json:"connected_at"`
    LastActivityAt time.Time `json:"last_activity_at"`
}

// OrderState represents in-flight arbitrage order
type OrderState struct {
    OrderID    int64     `json:"order_id"`
    TransID    int64     `json:"trans_id"`
    Side       string    `json:"side"` // buy, sell
    ExchangeID int64     `json:"exchange_id"`
    Symbol     string    `json:"symbol"`
    Status     string    `json:"status"`
    CreatedAt  time.Time `json:"created_at"`
}

// NewDaemonState creates empty daemon state
func NewDaemonState() *DaemonState {
    return &DaemonState{
        Version:   "1.0",
        UpdatedAt: time.Now(),
        Server: ServerState{
            StartedAt:      time.Now(),
            Status:         "starting",
            ActiveSessions: 0,
            TotalOrders:    0,
            LastHeartbeat:  time.Now(),
        },
        Traders:  make([]TraderState, 0),
        Sessions: make([]SessionState, 0),
        Orders:   make([]OrderState, 0),
    }
}
```

**Время:** 1 час

### Шаг 2: Создать internal/state/manager.go

```go
package state

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "log/slog"
)

type Manager struct {
    state      *DaemonState
    mu         sync.RWMutex
    logger     *slog.Logger
    
    // Configuration
    stateFile   string
    backupDir   string
    maxBackups  int
    syncInterval time.Duration
    
    // Background sync
    stopChan chan struct{}
    wg       sync.WaitGroup
}

type ManagerConfig struct {
    StateFile    string        // Path to daemon.state
    BackupDir    string        // Path to backups directory
    MaxBackups   int           // Max number of backup files to keep
    SyncInterval time.Duration // How often to sync to MySQL
}

// NewManager creates new state manager
func NewManager(cfg ManagerConfig, logger *slog.Logger) (*Manager, error) {
    // Ensure directories exist
    if err := os.MkdirAll(filepath.Dir(cfg.StateFile), 0755); err != nil {
        return nil, fmt.Errorf("failed to create state directory: %w", err)
    }
    
    if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create backup directory: %w", err)
    }
    
    return &Manager{
        state:        NewDaemonState(),
        logger:       logger,
        stateFile:    cfg.StateFile,
        backupDir:    cfg.BackupDir,
        maxBackups:   cfg.MaxBackups,
        syncInterval: cfg.SyncInterval,
        stopChan:     make(chan struct{}),
    }, nil
}

// Load loads state from disk
func (m *Manager) Load() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    data, err := os.ReadFile(m.stateFile)
    if err != nil {
        if os.IsNotExist(err) {
            m.logger.Info().Msg("State file not found, starting with empty state")
            return nil
        }
        return fmt.Errorf("failed to read state file: %w", err)
    }
    
    var state DaemonState
    if err := json.Unmarshal(data, &state); err != nil {
        return fmt.Errorf("failed to parse state file: %w", err)
    }
    
    m.state = &state
    m.logger.Info().
        Str("version", state.Version).
        Time("updated_at", state.UpdatedAt).
        Int("traders", len(state.Traders)).
        Int("sessions", len(state.Sessions)).
        Msg("State loaded successfully")
    
    return nil
}

// Save saves state to disk
func (m *Manager) Save() error {
    m.mu.RLock()
    m.state.UpdatedAt = time.Now()
    data, err := json.MarshalIndent(m.state, "", "  ")
    m.mu.RUnlock()
    
    if err != nil {
        return fmt.Errorf("failed to marshal state: %w", err)
    }
    
    // Write to temp file first (atomic write)
    tempFile := m.stateFile + ".tmp"
    if err := os.WriteFile(tempFile, data, 0600); err != nil {
        return fmt.Errorf("failed to write temp state file: %w", err)
    }
    
    // Atomic rename
    if err := os.Rename(tempFile, m.stateFile); err != nil {
        return fmt.Errorf("failed to rename state file: %w", err)
    }
    
    m.logger.Debug().Msg("State saved to disk")
    return nil
}

// Backup creates backup of current state file
func (m *Manager) Backup() error {
    // Check if state file exists
    if _, err := os.Stat(m.stateFile); os.IsNotExist(err) {
        return nil // Nothing to backup
    }
    
    // Read current state
    data, err := os.ReadFile(m.stateFile)
    if err != nil {
        return fmt.Errorf("failed to read state file for backup: %w", err)
    }
    
    // Create backup filename with timestamp
    timestamp := time.Now().Format("20060102_150405")
    backupFile := filepath.Join(m.backupDir, fmt.Sprintf("daemon.state.%s", timestamp))
    
    // Write backup
    if err := os.WriteFile(backupFile, data, 0600); err != nil {
        return fmt.Errorf("failed to write backup file: %w", err)
    }
    
    m.logger.Info().Str("backup", backupFile).Msg("State backup created")
    
    // Cleanup old backups
    return m.cleanupOldBackups()
}

// cleanupOldBackups removes old backup files
func (m *Manager) cleanupOldBackups() error {
    files, err := filepath.Glob(filepath.Join(m.backupDir, "daemon.state.*"))
    if err != nil {
        return fmt.Errorf("failed to list backups: %w", err)
    }
    
    // Keep only last N backups
    if len(files) > m.maxBackups {
        // Sort by filename (timestamp in name ensures chronological order)
        // Delete oldest files
        for i := 0; i < len(files)-m.maxBackups; i++ {
            if err := os.Remove(files[i]); err != nil {
                m.logger.Warn().Err(err).Str("file", files[i]).Msg("Failed to delete old backup")
            } else {
                m.logger.Debug().Str("file", files[i]).Msg("Old backup deleted")
            }
        }
    }
    
    return nil
}

// GetState returns copy of current state (read-only)
func (m *Manager) GetState() DaemonState {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    // Return a copy to prevent external modification
    return *m.state
}

// UpdateServerStatus updates server status
func (m *Manager) UpdateServerStatus(status string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.state.Server.Status = status
    m.state.Server.LastHeartbeat = time.Now()
}

// Close stops background sync and saves final state
func (m *Manager) Close() error {
    // Stop background sync
    close(m.stopChan)
    m.wg.Wait()
    
    // Save final state
    if err := m.Save(); err != nil {
        return fmt.Errorf("failed to save final state: %w", err)
    }
    
    // Create final backup
    if err := m.Backup(); err != nil {
        m.logger.Warn().Err(err).Msg("Failed to create final backup")
    }
    
    m.logger.Info().Msg("State manager closed")
    return nil
}
```

**Время:** 1 час

### Верификация 1.4.1

```bash
# Скомпилировать
make build

# Ожидаемый результат: нет ошибок компиляции
```

**Definition of Done:**
- [ ] `internal/state/types.go` создан (~80 строк)
- [ ] `internal/state/manager.go` создан (~200 строк)
- [ ] Data structures определены
- [ ] Load/Save/Backup методы реализованы
- [ ] `make build` проходит без ошибок

---

## 1.4.2: Trader State Operations (2 часа)

### Шаг 1: Добавить trader operations в manager.go

Добавить в `internal/state/manager.go`:

```go
// AddTrader adds or updates trader state
func (m *Manager) AddTrader(trader TraderState) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Check if trader already exists
    for i, t := range m.state.Traders {
        if t.TraderID == trader.TraderID {
            // Update existing
            m.state.Traders[i] = trader
            m.logger.Debug().Int64("trader_id", trader.TraderID).Msg("Trader state updated")
            return
        }
    }
    
    // Add new
    m.state.Traders = append(m.state.Traders, trader)
    m.logger.Info().Int64("trader_id", trader.TraderID).Str("name", trader.Name).Msg("Trader added to state")
}

// RemoveTrader removes trader from state
func (m *Manager) RemoveTrader(traderID int64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for i, t := range m.state.Traders {
        if t.TraderID == traderID {
            // Remove by replacing with last element and truncating
            m.state.Traders[i] = m.state.Traders[len(m.state.Traders)-1]
            m.state.Traders = m.state.Traders[:len(m.state.Traders)-1]
            m.logger.Info().Int64("trader_id", traderID).Msg("Trader removed from state")
            return
        }
    }
}

// GetTrader returns trader state by ID
func (m *Manager) GetTrader(traderID int64) (TraderState, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    for _, t := range m.state.Traders {
        if t.TraderID == traderID {
            return t, true
        }
    }
    
    return TraderState{}, false
}

// GetActiveTraders returns all active traders
func (m *Manager) GetActiveTraders() []TraderState {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    active := make([]TraderState, 0)
    for _, t := range m.state.Traders {
        if t.Status == "active" {
            active = append(active, t)
        }
    }
    
    return active
}

// UpdateTraderHeartbeat updates trader's last heartbeat and load score
func (m *Manager) UpdateTraderHeartbeat(traderID int64, loadScore int32) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for i, t := range m.state.Traders {
        if t.TraderID == traderID {
            m.state.Traders[i].LastHeartbeat = time.Now()
            m.state.Traders[i].LoadScore = loadScore
            m.logger.Debug().
                Int64("trader_id", traderID).
                Int32("load_score", loadScore).
                Msg("Trader heartbeat updated")
            return
        }
    }
}
```

**Время:** 1 час

### Шаг 2: Добавить session operations

```go
// AddSession adds or updates session state
func (m *Manager) AddSession(session SessionState) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // Check if session already exists
    for i, s := range m.state.Sessions {
        if s.SessionID == session.SessionID {
            m.state.Sessions[i] = session
            return
        }
    }
    
    // Add new
    m.state.Sessions = append(m.state.Sessions, session)
    m.state.Server.ActiveSessions = len(m.state.Sessions)
    
    m.logger.Info().
        Int64("session_id", session.SessionID).
        Int64("user_id", session.UserID).
        Msg("Session added to state")
}

// RemoveSession removes session from state
func (m *Manager) RemoveSession(sessionID int64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for i, s := range m.state.Sessions {
        if s.SessionID == sessionID {
            m.state.Sessions[i] = m.state.Sessions[len(m.state.Sessions)-1]
            m.state.Sessions = m.state.Sessions[:len(m.state.Sessions)-1]
            m.state.Server.ActiveSessions = len(m.state.Sessions)
            
            m.logger.Info().Int64("session_id", sessionID).Msg("Session removed from state")
            return
        }
    }
}

// UpdateSessionActivity updates session's last activity time
func (m *Manager) UpdateSessionActivity(sessionID int64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for i, s := range m.state.Sessions {
        if s.SessionID == sessionID {
            m.state.Sessions[i].LastActivityAt = time.Now()
            return
        }
    }
}

// GetSessionCount returns number of active sessions
func (m *Manager) GetSessionCount() int {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    return len(m.state.Sessions)
}
```

**Время:** 30 минут

### Шаг 3: Добавить order operations

```go
// AddOrder adds order to in-flight state
func (m *Manager) AddOrder(order OrderState) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.state.Orders = append(m.state.Orders, order)
    m.state.Server.TotalOrders++
    
    m.logger.Debug().
        Int64("order_id", order.OrderID).
        Int64("trans_id", order.TransID).
        Msg("Order added to state")
}

// RemoveOrder removes order from in-flight state
func (m *Manager) RemoveOrder(orderID int64) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for i, o := range m.state.Orders {
        if o.OrderID == orderID {
            m.state.Orders[i] = m.state.Orders[len(m.state.Orders)-1]
            m.state.Orders = m.state.Orders[:len(m.state.Orders)-1]
            
            m.logger.Debug().Int64("order_id", orderID).Msg("Order removed from state")
            return
        }
    }
}

// UpdateOrderStatus updates order status
func (m *Manager) UpdateOrderStatus(orderID int64, status string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    for i, o := range m.state.Orders {
        if o.OrderID == orderID {
            m.state.Orders[i].Status = status
            m.logger.Debug().
                Int64("order_id", orderID).
                Str("status", status).
                Msg("Order status updated")
            return
        }
    }
}

// GetInFlightOrders returns all in-flight orders
func (m *Manager) GetInFlightOrders() []OrderState {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    // Return a copy
    orders := make([]OrderState, len(m.state.Orders))
    copy(orders, m.state.Orders)
    
    return orders
}
```

**Время:** 30 минут

### Верификация 1.4.2

```bash
# Скомпилировать
make build

# Ожидаемый результат: нет ошибок компиляции
```

**Definition of Done:**
- [ ] Trader operations (Add, Remove, Update, Get) реализованы
- [ ] Session operations реализованы
- [ ] Order operations реализованы
- [ ] `make build` проходит без ошибок

---

## 1.4.3: MySQL Sync (3 часа)

### Шаг 1: Создать internal/state/sync.go

```go
package state

import (
    "context"
    "time"

    "github.com/your-org/cts-core/internal/db"
    "github.com/your-org/cts-core/internal/db/models"
)

// Syncer handles MySQL synchronization
type Syncer struct {
    manager *Manager
    repo    *db.Repository
}

// NewSyncer creates new syncer
func NewSyncer(manager *Manager, repo *db.Repository) *Syncer {
    return &Syncer{
        manager: manager,
        repo:    repo,
    }
}

// SyncToMySQL syncs current state to MySQL
func (s *Syncer) SyncToMySQL(ctx context.Context) error {
    state := s.manager.GetState()
    
    // Sync traders
    if err := s.syncTraders(ctx, state.Traders); err != nil {
        return err
    }
    
    // Sync sessions
    if err := s.syncSessions(ctx, state.Sessions); err != nil {
        return err
    }
    
    // Note: Orders are already in MySQL (inserted when created)
    // State only tracks in-flight orders for recovery
    
    s.manager.logger.Debug().Msg("State synced to MySQL")
    return nil
}

// syncTraders syncs trader heartbeats to MySQL
func (s *Syncer) syncTraders(ctx context.Context, traders []TraderState) error {
    for _, trader := range traders {
        // Build resources JSON (simplified)
        resourcesJSON := ""
        
        err := s.repo.UpdateTraderHeartbeat(ctx, trader.TraderID, trader.LoadScore, resourcesJSON)
        if err != nil {
            s.manager.logger.Warn().
                Err(err).
                Int64("trader_id", trader.TraderID).
                Msg("Failed to sync trader heartbeat")
            // Continue with other traders
        }
    }
    
    return nil
}

// syncSessions syncs session activity to MySQL
func (s *Syncer) syncSessions(ctx context.Context, sessions []SessionState) error {
    for _, session := range sessions {
        err := s.repo.UpdateSessionActivity(ctx, session.SessionID)
        if err != nil {
            s.manager.logger.Warn().
                Err(err).
                Int64("session_id", session.SessionID).
                Msg("Failed to sync session activity")
            // Continue with other sessions
        }
    }
    
    return nil
}

// LoadFromMySQL loads state from MySQL (recovery)
func (s *Syncer) LoadFromMySQL(ctx context.Context) error {
    s.manager.logger.Info().Msg("Loading state from MySQL...")
    
    // Load active traders
    traders, err := s.repo.ListActiveTraders(ctx)
    if err != nil {
        return err
    }
    
    s.manager.mu.Lock()
    s.manager.state.Traders = make([]TraderState, 0, len(traders))
    for _, trader := range traders {
        s.manager.state.Traders = append(s.manager.state.Traders, TraderState{
            TraderID:       trader.TraderID,
            Name:           trader.Name,
            IPAddress:      trader.IPAddress,
            Port:           trader.Port,
            Status:         trader.Status,
            LastHeartbeat:  trader.LastHeartbeat.Time,
            LoadScore:      trader.LoadScore.Int32,
            ConnectionTime: trader.RegisteredAt,
        })
    }
    s.manager.mu.Unlock()
    
    s.manager.logger.Info().
        Int("traders", len(traders)).
        Msg("State loaded from MySQL")
    
    return nil
}
```

**Время:** 1.5 часа

### Шаг 2: Добавить background sync в manager.go

Добавить в `internal/state/manager.go`:

```go
// StartBackgroundSync starts background sync goroutine
func (m *Manager) StartBackgroundSync(repo *db.Repository) {
    syncer := NewSyncer(m, repo)
    
    m.wg.Add(1)
    go func() {
        defer m.wg.Done()
        
        ticker := time.NewTicker(m.syncInterval)
        defer ticker.Stop()
        
        m.logger.Info().
            Dur("interval", m.syncInterval).
            Msg("Background sync started")
        
        for {
            select {
            case <-ticker.C:
                // Sync to MySQL
                ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
                if err := syncer.SyncToMySQL(ctx); err != nil {
                    m.logger.Error().Err(err).Msg("Failed to sync state to MySQL")
                }
                cancel()
                
                // Save to disk
                if err := m.Save(); err != nil {
                    m.logger.Error().Err(err).Msg("Failed to save state to disk")
                }
                
            case <-m.stopChan:
                m.logger.Info().Msg("Background sync stopped")
                return
            }
        }
    }()
}
```

**Время:** 1 hour

### Шаг 3: Добавить state config

Добавить в `internal/config/types.go`:

```go
type StateConfig struct {
    File         string `yaml:"file"`           // daemon.state path
    BackupDir    string `yaml:"backup_dir"`     // backups directory
    MaxBackups   int    `yaml:"max_backups"`    // max backup files
    SyncInterval int    `yaml:"sync_interval_seconds"` // sync to MySQL interval
}
```

Обновить `internal/config/config.go`:

```go
type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Database DatabaseConfig `yaml:"database"`
    HSM      HSMConfig      `yaml:"hsm"`
    State    StateConfig    `yaml:"state"` // <-- ADD THIS
    Logging  LoggingConfig  `yaml:"logging"`
}
```

Добавить в `conf/config.yaml`:

```yaml
state:
  file: state/daemon.state
  backup_dir: state/backups
  max_backups: 3
  sync_interval_seconds: 30
```

**Время:** 30 минут

### Верификация 1.4.3

```bash
# Скомпилировать
make build

# Ожидаемый результат: нет ошибок компиляции
```

**Definition of Done:**
- [ ] `internal/state/sync.go` создан (~120 строк)
- [ ] Background sync реализован
- [ ] StateConfig добавлен
- [ ] config.yaml обновлен
- [ ] `make build` проходит без ошибок

---

## 1.4.4: Integration в main.go (2 часа)

### Шаг 1: Интегрировать state manager в main.go

Обновить `cmd/daemon/main.go`:

```go
import (
    "github.com/your-org/cts-core/internal/state"
    "time"
)

func main() {
    // ... (после repo)

    // Initialize state manager
    stateCfg := state.ManagerConfig{
        StateFile:    cfg.State.File,
        BackupDir:    cfg.State.BackupDir,
        MaxBackups:   cfg.State.MaxBackups,
        SyncInterval: time.Duration(cfg.State.SyncInterval) * time.Second,
    }

    stateManager, err := state.NewManager(stateCfg, logger)
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to create state manager")
    }
    defer stateManager.Close()

    // Try to load existing state
    if err := stateManager.Load(); err != nil {
        logger.Warn().Err(err).Msg("Failed to load state, starting fresh")
    }

    // Start background sync
    stateManager.StartBackgroundSync(repo)

    // Update server status
    stateManager.UpdateServerStatus("running")

    logger.Info().Msg("State manager initialized")

    // ... rest of application
}
```

**Время:** 30 минут

### Шаг 2: Создать state recovery helper

Создать `internal/state/recovery.go`:

```go
package state

import (
    "context"
    "fmt"
)

// RecoverState attempts to recover state from disk or MySQL
func (m *Manager) RecoverState(syncer *Syncer) error {
    // Try to load from disk first
    if err := m.Load(); err != nil {
        m.logger.Warn().Err(err).Msg("Failed to load state from disk")
        
        // Try to load from MySQL
        ctx := context.Background()
        if err := syncer.LoadFromMySQL(ctx); err != nil {
            return fmt.Errorf("failed to recover state from MySQL: %w", err)
        }
        
        m.logger.Info().Msg("State recovered from MySQL")
        
        // Save recovered state to disk
        if err := m.Save(); err != nil {
            m.logger.Warn().Err(err).Msg("Failed to save recovered state to disk")
        }
        
        return nil
    }
    
    m.logger.Info().Msg("State loaded from disk")
    return nil
}

// ValidateState validates state consistency
func (m *Manager) ValidateState() []string {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    issues := make([]string, 0)
    
    // Check version
    if m.state.Version != "1.0" {
        issues = append(issues, fmt.Sprintf("Unknown state version: %s", m.state.Version))
    }
    
    // Check trader duplicates
    traderIDs := make(map[int64]bool)
    for _, trader := range m.state.Traders {
        if traderIDs[trader.TraderID] {
            issues = append(issues, fmt.Sprintf("Duplicate trader ID: %d", trader.TraderID))
        }
        traderIDs[trader.TraderID] = true
    }
    
    // Check session duplicates
    sessionIDs := make(map[int64]bool)
    for _, session := range m.state.Sessions {
        if sessionIDs[session.SessionID] {
            issues = append(issues, fmt.Sprintf("Duplicate session ID: %d", session.SessionID))
        }
        sessionIDs[session.SessionID] = true
    }
    
    // Validate server status
    validStatuses := map[string]bool{
        "starting": true,
        "running":  true,
        "stopping": true,
        "stopped":  true,
    }
    if !validStatuses[m.state.Server.Status] {
        issues = append(issues, fmt.Sprintf("Invalid server status: %s", m.state.Server.Status))
    }
    
    return issues
}
```

**Время:** 30 минут

### Шаг 3: Добавить state tests

Создать `internal/state/manager_test.go`:

```go
package state

import (
    "io"
    "os"
    "path/filepath"
    "testing"
    "time"

    "log/slog"
)

func TestStateManagerBasic(t *testing.T) {
    logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
    
    // Create temp directory
    tmpDir := t.TempDir()
    
    cfg := ManagerConfig{
        StateFile:    filepath.Join(tmpDir, "daemon.state"),
        BackupDir:    filepath.Join(tmpDir, "backups"),
        MaxBackups:   3,
        SyncInterval: 30 * time.Second,
    }
    
    manager, err := NewManager(cfg, logger)
    if err != nil {
        t.Fatalf("Failed to create manager: %v", err)
    }
    defer manager.Close()
    
    // Test: Add trader
    trader := TraderState{
        TraderID:  1,
        Name:      "TestTrader",
        IPAddress: "192.168.1.100",
        Port:      8443,
        Status:    "active",
        LastHeartbeat: time.Now(),
    }
    
    manager.AddTrader(trader)
    
    // Test: Get trader
    retrieved, found := manager.GetTrader(1)
    if !found {
        t.Fatal("Trader not found")
    }
    
    if retrieved.Name != trader.Name {
        t.Errorf("Expected %s, got %s", trader.Name, retrieved.Name)
    }
    
    // Test: Save state
    if err := manager.Save(); err != nil {
        t.Fatalf("Failed to save state: %v", err)
    }
    
    // Verify file exists
    if _, err := os.Stat(cfg.StateFile); os.IsNotExist(err) {
        t.Fatal("State file not created")
    }
}

func TestStateLoadSave(t *testing.T) {
    logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
    tmpDir := t.TempDir()
    
    cfg := ManagerConfig{
        StateFile:    filepath.Join(tmpDir, "daemon.state"),
        BackupDir:    filepath.Join(tmpDir, "backups"),
        MaxBackups:   3,
        SyncInterval: 30 * time.Second,
    }
    
    // Create manager and add data
    manager1, _ := NewManager(cfg, logger)
    
    trader := TraderState{
        TraderID: 1,
        Name:     "TestTrader",
    }
    manager1.AddTrader(trader)
    
    session := SessionState{
        SessionID: 100,
        UserID:    1,
    }
    manager1.AddSession(session)
    
    // Save
    if err := manager1.Save(); err != nil {
        t.Fatalf("Save failed: %v", err)
    }
    manager1.Close()
    
    // Create new manager and load
    manager2, _ := NewManager(cfg, &logger)
    if err := manager2.Load(); err != nil {
        t.Fatalf("Load failed: %v", err)
    }
    defer manager2.Close()
    
    // Verify loaded data
    if len(manager2.GetState().Traders) != 1 {
        t.Errorf("Expected 1 trader, got %d", len(manager2.GetState().Traders))
    }
    
    if len(manager2.GetState().Sessions) != 1 {
        t.Errorf("Expected 1 session, got %d", len(manager2.GetState().Sessions))
    }
}

func TestBackupRotation(t *testing.T) {
    logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
    tmpDir := t.TempDir()
    
    cfg := ManagerConfig{
        StateFile:    filepath.Join(tmpDir, "daemon.state"),
        BackupDir:    filepath.Join(tmpDir, "backups"),
        MaxBackups:   3,
        SyncInterval: 30 * time.Second,
    }
    
    manager, _ := NewManager(cfg, logger)
    defer manager.Close()
    
    // Create initial state
    manager.Save()
    
    // Create 5 backups
    for i := 0; i < 5; i++ {
        time.Sleep(10 * time.Millisecond) // Ensure unique timestamps
        manager.Backup()
    }
    
    // Check that only 3 backups remain
    files, _ := filepath.Glob(filepath.Join(cfg.BackupDir, "daemon.state.*"))
    if len(files) != 3 {
        t.Errorf("Expected 3 backups, got %d", len(files))
    }
}

func TestValidateState(t *testing.T) {
    logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
    tmpDir := t.TempDir()
    
    cfg := ManagerConfig{
        StateFile:    filepath.Join(tmpDir, "daemon.state"),
        BackupDir:    filepath.Join(tmpDir, "backups"),
        MaxBackups:   3,
        SyncInterval: 30 * time.Second,
    }
    
    manager, _ := NewManager(cfg, logger)
    defer manager.Close()
    
    // Add valid data
    manager.AddTrader(TraderState{TraderID: 1, Name: "T1"})
    
    // Validate
    issues := manager.ValidateState()
    if len(issues) > 0 {
        t.Errorf("Expected no issues, got: %v", issues)
    }
    
    // Add duplicate trader (manually to bypass AddTrader logic)
    manager.mu.Lock()
    manager.state.Traders = append(manager.state.Traders, TraderState{TraderID: 1, Name: "T2"})
    manager.mu.Unlock()
    
    // Validate again
    issues = manager.ValidateState()
    if len(issues) == 0 {
        t.Error("Expected validation issues for duplicate trader")
    }
}
```

**Время:** 1 час

### Верификация 1.4.4

```bash
# Run unit tests
go test -v ./internal/state/...

# Ожидаемый вывод:
# === RUN   TestStateManagerBasic
# --- PASS: TestStateManagerBasic (0.01s)
# === RUN   TestStateLoadSave
# --- PASS: TestStateLoadSave (0.01s)
# === RUN   TestBackupRotation
# --- PASS: TestBackupRotation (0.06s)
# === RUN   TestValidateState
# --- PASS: TestValidateState (0.00s)
# PASS

# Build and run
make build
./bin/cts-core -config conf/config.yaml

# Ожидаемый вывод:
# {"level":"info","message":"State manager initialized"}
# {"level":"info","message":"Background sync started","interval":"30s"}

# Check state file created
ls -la state/
# Expected: daemon.state file exists

# Wait 30 seconds and check again
# Expected: daemon.state updated (check mtime)
```

**Definition of Done:**
- [ ] State manager интегрирован в main.go
- [ ] Recovery helper создан
- [ ] Unit tests написаны и проходят
- [ ] State file создается при запуске
- [ ] Background sync работает (проверить mtime)

---

## 1.4.5: Monitoring и Health Check (1 час)

### Шаг 1: Добавить state metrics

Создать `internal/state/metrics.go`:

```go
package state

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    stateActiveTraders = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "cts_state_active_traders",
        Help: "Number of active traders in state",
    })
    
    stateActiveSessions = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "cts_state_active_sessions",
        Help: "Number of active sessions in state",
    })
    
    stateInFlightOrders = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "cts_state_inflight_orders",
        Help: "Number of in-flight orders in state",
    })
    
    stateSaveErrors = promauto.NewCounter(prometheus.CounterOpts{
        Name: "cts_state_save_errors_total",
        Help: "Total number of state save errors",
    })
    
    stateSyncErrors = promauto.NewCounter(prometheus.CounterOpts{
        Name: "cts_state_sync_errors_total",
        Help: "Total number of MySQL sync errors",
    })
)

// UpdateMetrics updates Prometheus metrics
func (m *Manager) UpdateMetrics() {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    stateActiveTraders.Set(float64(len(m.state.Traders)))
    stateActiveSessions.Set(float64(len(m.state.Sessions)))
    stateInFlightOrders.Set(float64(len(m.state.Orders)))
}
```

Обновить background sync в `manager.go`:

```go
// In StartBackgroundSync, add after Save():
m.UpdateMetrics()
```

**Время:** 20 минут

### Шаг 2: Обновить health check

Обновить `internal/api/rest/health.go`:

```go
import (
    "github.com/your-org/cts-core/internal/state"
)

type HealthHandler struct {
    dbClient     *db.MySQLClient
    hsmClient    *hsm.Client
    stateManager *state.Manager
}

func NewHealthHandler(dbClient *db.MySQLClient, hsmClient *hsm.Client, stateManager *state.Manager) *HealthHandler {
    return &HealthHandler{
        dbClient:     dbClient,
        hsmClient:    hsmClient,
        stateManager: stateManager,
    }
}

func (h *HealthHandler) Health(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
    defer cancel()

    // ... (existing database and HSM checks)

    // Check state
    stateStatus := "ok"
    stateData := h.stateManager.GetState()
    issues := h.stateManager.ValidateState()
    if len(issues) > 0 {
        stateStatus = fmt.Sprintf("validation issues: %d", len(issues))
    }

    response := gin.H{
        "status": "ok",
        "components": gin.H{
            "database": dbStatus,
            "hsm":      hsmStatus,
            "state":    stateStatus,
        },
        "state_info": gin.H{
            "traders":  len(stateData.Traders),
            "sessions": len(stateData.Sessions),
            "orders":   len(stateData.Orders),
            "updated":  stateData.UpdatedAt.Unix(),
        },
        "timestamp": time.Now().Unix(),
    }

    // If any component is down, return 503
    if dbStatus != "ok" || hsmStatus != "ok" || stateStatus != "ok" {
        c.JSON(http.StatusServiceUnavailable, response)
        return
    }

    c.JSON(http.StatusOK, response)
}
```

**Время:** 20 минут

### Шаг 3: Добавить state debug endpoint

Создать `internal/api/rest/state.go`:

```go
package rest

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/your-org/cts-core/internal/state"
)

type StateHandler struct {
    stateManager *state.Manager
}

func NewStateHandler(stateManager *state.Manager) *StateHandler {
    return &StateHandler{stateManager: stateManager}
}

// GetState returns current daemon state (debug endpoint)
func (h *StateHandler) GetState(c *gin.Context) {
    state := h.stateManager.GetState()
    
    c.JSON(http.StatusOK, state)
}

// GetStateValidation returns state validation issues
func (h *StateHandler) GetStateValidation(c *gin.Context) {
    issues := h.stateManager.ValidateState()
    
    response := gin.H{
        "valid":  len(issues) == 0,
        "issues": issues,
    }
    
    c.JSON(http.StatusOK, response)
}
```

**Время:** 20 минут

### Верификация 1.4.5

```bash
# Build and run
make build
./bin/cts-core -config conf/config.yaml

# Test health endpoint
curl -k https://localhost:8443/health
# Expected: {"status":"ok","components":{"database":"ok","hsm":"ok","state":"ok"},"state_info":{...}}

# Test metrics endpoint
curl -k https://localhost:9090/metrics | grep cts_state
# Expected:
# cts_state_active_traders 0
# cts_state_active_sessions 0
# cts_state_inflight_orders 0

# Test state debug endpoint (if registered in router)
curl -k https://localhost:8443/debug/state
# Expected: JSON with full state

# Test state validation endpoint
curl -k https://localhost:8443/debug/state/validation
# Expected: {"valid":true,"issues":[]}
```

**Definition of Done:**
- [ ] Prometheus metrics добавлены
- [ ] Health check обновлен (включает state info)
- [ ] State debug endpoints созданы
- [ ] Metrics endpoint показывает state metrics
- [ ] Health endpoint возвращает state status

---

## Troubleshooting

### Проблема: "Failed to create state directory"

**Причина:** Нет прав на запись в state/ директорию.

**Решение:**
```bash
# Создать директорию
mkdir -p state/backups

# Установить права
chmod 755 state
chmod 755 state/backups
```

### Проблема: State file corrupted (JSON parse error)

**Причина:** daemon.state file поврежден.

**Решение:**
```bash
# Восстановить из backup
ls -lt state/backups/
cp state/backups/daemon.state.20260128_120000 state/daemon.state

# Или начать с пустого state
rm state/daemon.state
# Daemon создаст новый при запуске
```

### Проблема: Background sync не работает

**Причина:** stopChan закрыт или sync interval слишком большой.

**Решение:**
1. Проверить logs:
   ```bash
   tail -f logs/cts-core.log | grep sync
   ```
2. Уменьшить sync interval в config.yaml:
   ```yaml
   state:
     sync_interval_seconds: 10  # test with 10 seconds
   ```

### Проблема: "Duplicate trader ID" validation error

**Причина:** State содержит дубликаты (corruption или bug).

**Решение:**
```bash
# Load state from MySQL instead
rm state/daemon.state
# Daemon will recover from MySQL
```

### Проблема: State file grows too large

**Причина:** Слишком много in-flight orders или sessions не удаляются.

**Решение:**
1. Check state size:
   ```bash
   ls -lh state/daemon.state
   ```
2. Inspect state:
   ```bash
   cat state/daemon.state | jq '.orders | length'
   cat state/daemon.state | jq '.sessions | length'
   ```
3. Cleanup completed orders from state (should be done automatically)

---

## FAQ

**Q: Зачем state file + MySQL sync?**
A: State file обеспечивает быстрый recovery после restart (не нужно запрашивать все данные из MySQL). MySQL sync обеспечивает durability и backup.

**Q: Что хранится в state?**
A: Только runtime state - активные traders, sessions, in-flight orders. Persistent data (order history, user data) только в MySQL.

**Q: Как часто sync в MySQL?**
A: По умолчанию каждые 30 секунд. Этого достаточно для большинства случаев. Критичные данные (новые orders) пишутся в MySQL immediately.

**Q: Что если daemon крашнется между syncs?**
A: Потеряется max 30 секунд runtime state (heartbeats, last activity). Критичные данные (orders) уже в MySQL, так что не теряются.

**Q: Сколько backups держать?**
A: По умолчанию 3. Это покрывает последние ~1.5 минуты (если backup каждые 30 секунд).

**Q: Как тестировать recovery?**
A:
```bash
# Start daemon
./bin/cts-core

# Wait for some state to accumulate
# Kill daemon (Ctrl+C)

# Restart
./bin/cts-core
# Should load state from disk

# Or delete state file to test MySQL recovery
rm state/daemon.state
./bin/cts-core
# Should recover from MySQL
```

**Q: Thread-safe ли state manager?**
A: Да, все operations защищены с `sync.RWMutex`. Read operations используют RLock, write operations - Lock.

**Q: Производительность - как быстро Save()?**
A: Save() - atomic write (~1ms для small state file). Background sync делает это каждые 30s, так что нет impact на latency.

---

## Summary Phase 1.4

**Созданные файлы:**
- `internal/state/types.go` (~80 строк)
- `internal/state/manager.go` (~400 строк)
- `internal/state/sync.go` (~120 строк)
- `internal/state/recovery.go` (~80 строк)
- `internal/state/metrics.go` (~50 строк)
- `internal/state/manager_test.go` (~150 строк)
- `internal/api/rest/state.go` (~40 строк)

**Total LOC:** ~920 строк

**Обновленные файлы:**
- `internal/config/types.go` (добавлен StateConfig)
- `conf/config.yaml` (добавлена state секция)
- `cmd/daemon/main.go` (добавлен state manager)
- `internal/api/rest/health.go` (добавлен state check)

**Deliverables:**
✅ State data structures (Traders, Sessions, Orders)  
✅ State manager (Load/Save/Backup)  
✅ MySQL sync (every 30 seconds)  
✅ Background sync goroutine  
✅ Recovery from disk or MySQL  
✅ State validation  
✅ Prometheus metrics  
✅ Health check с state info  
✅ Unit tests  

**Next Phase:** Phase 1.5 - REST API Server

---

## Definition of Done - Phase 1.4

- [ ] Все файлы созданы и скомпилированы
- [ ] `make build` проходит без ошибок
- [ ] Unit tests проходят (`go test ./internal/state/...`)
- [ ] State file создается при запуске
- [ ] Background sync работает (check mtime каждые 30s)
- [ ] Backup rotation работает (max 3 backups)
- [ ] Recovery из disk работает
- [ ] Recovery из MySQL работает (delete state file, restart)
- [ ] Health endpoint показывает state info
- [ ] Metrics endpoint показывает state metrics
- [ ] Закоммичено в git:
  ```bash
  git add internal/state/ internal/config/ conf/config.yaml cmd/daemon/main.go internal/api/rest/ state/
  git commit -m "Phase 1.4: State management with MySQL sync"
  ```
- [ ] `guides/phase_1_4_state_management.md` удален (после завершения фазы)
