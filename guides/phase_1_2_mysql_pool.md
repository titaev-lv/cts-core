# Phase 1.2: MySQL Connection Pool

**Цель:** Создать connection pool с mTLS, retry logic, repository pattern и модели для всех таблиц.

**Время:** ~2 дня (15-20 часов)

**Зависимости:**
- ✅ Phase 0 завершена (таблицы созданы)
- ✅ Phase 1.1 завершена (config, logger готовы)
- MySQL 9.0 запущен
- MySQL client certificates готовы (conf/ssl/client-cert.pem, client-key.pem, ca-cert.pem)

---

## 1.2.1: MySQL Client с mTLS (4 часа)

### Шаг 1: Создать internal/db/mysql.go

```go
package db

import (
    "crypto/tls"
    "crypto/x509"
    "database/sql"
    "fmt"
    "os"
    "time"

    _ "github.com/go-sql-driver/mysql"
    "github.com/rs/zerolog"
)

type MySQLConfig struct {
    Host     string
    Port     int
    User     string
    Password string
    Database string
    CertPath string
    KeyPath  string
    CAPath   string

    // Connection Pool
    MaxOpenConns    int
    MaxIdleConns    int
    ConnMaxLifetime time.Duration
    ConnMaxIdleTime time.Duration
}

type MySQLClient struct {
    db     *sql.DB
    logger *zerolog.Logger
}

// NewMySQLClient creates new MySQL client with mTLS
func NewMySQLClient(cfg MySQLConfig, logger *zerolog.Logger) (*MySQLClient, error) {
    // Load client certificate
    cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load client cert: %w", err)
    }

    // Load CA certificate
    caCert, err := os.ReadFile(cfg.CAPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read CA cert: %w", err)
    }

    caCertPool := x509.NewCertPool()
    if !caCertPool.AppendCertsFromPEM(caCert) {
        return nil, fmt.Errorf("failed to append CA cert")
    }

    // Configure TLS
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      caCertPool,
        ServerName:   cfg.Host, // Important for certificate validation
    }

    // Register TLS config
    err = mysql.RegisterTLSConfig("custom", tlsConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to register TLS config: %w", err)
    }

    // Build DSN (Data Source Name)
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=custom&parseTime=true&loc=UTC",
        cfg.User,
        cfg.Password,
        cfg.Host,
        cfg.Port,
        cfg.Database,
    )

    // Open connection
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to open connection: %w", err)
    }

    // Configure connection pool
    db.SetMaxOpenConns(cfg.MaxOpenConns)       // Max 25 connections
    db.SetMaxIdleConns(cfg.MaxIdleConns)       // Keep 10 idle
    db.SetConnMaxLifetime(cfg.ConnMaxLifetime) // 5 minutes
    db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime) // 2 minutes

    // Test connection
    if err := db.Ping(); err != nil {
        db.Close()
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    logger.Info().Msgf("MySQL connected to %s:%d (mTLS enabled)", cfg.Host, cfg.Port)

    return &MySQLClient{
        db:     db,
        logger: logger,
    }, nil
}

// Close closes database connection
func (c *MySQLClient) Close() error {
    if c.db != nil {
        return c.db.Close()
    }
    return nil
}

// DB returns underlying *sql.DB for advanced usage
func (c *MySQLClient) DB() *sql.DB {
    return c.db
}

// Ping checks if connection is alive
func (c *MySQLClient) Ping() error {
    return c.db.Ping()
}
```

**Время:** 2 часа

### Шаг 2: Добавить retry logic с exponential backoff

Добавить в `internal/db/mysql.go`:

```go
import (
    "context"
    "math"
    "time"
)

// RetryConfig defines retry behavior
type RetryConfig struct {
    MaxAttempts int
    InitialWait time.Duration
    MaxWait     time.Duration
    Multiplier  float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
    return RetryConfig{
        MaxAttempts: 3,
        InitialWait: 100 * time.Millisecond,
        MaxWait:     5 * time.Second,
        Multiplier:  2.0,
    }
}

// WithRetry executes function with exponential backoff retry
func (c *MySQLClient) WithRetry(ctx context.Context, operation func() error) error {
    cfg := DefaultRetryConfig()
    var lastErr error

    for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
        // Execute operation
        err := operation()
        if err == nil {
            return nil
        }

        lastErr = err

        // Check if context is cancelled
        if ctx.Err() != nil {
            return fmt.Errorf("context cancelled: %w", ctx.Err())
        }

        // Don't retry on last attempt
        if attempt == cfg.MaxAttempts {
            break
        }

        // Calculate backoff delay
        wait := time.Duration(float64(cfg.InitialWait) * math.Pow(cfg.Multiplier, float64(attempt-1)))
        if wait > cfg.MaxWait {
            wait = cfg.MaxWait
        }

        c.logger.Warn().
            Err(err).
            Int("attempt", attempt).
            Dur("retry_in", wait).
            Msg("Operation failed, retrying...")

        // Wait before retry
        select {
        case <-time.After(wait):
            // Continue to next attempt
        case <-ctx.Done():
            return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
        }
    }

    return fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}
```

**Время:** 1 час

### Шаг 3: Обновить config для MySQL

Добавить в `internal/config/types.go`:

```go
type DatabaseConfig struct {
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    User     string `yaml:"user"`
    Password string `yaml:"password"`
    Database string `yaml:"database"`
    
    TLS struct {
        CertPath string `yaml:"cert"`
        KeyPath  string `yaml:"key"`
        CAPath   string `yaml:"ca"`
    } `yaml:"tls"`
    
    Pool struct {
        MaxOpenConns    int `yaml:"max_open_conns"`
        MaxIdleConns    int `yaml:"max_idle_conns"`
        ConnMaxLifetime int `yaml:"conn_max_lifetime_minutes"`
        ConnMaxIdleTime int `yaml:"conn_max_idle_time_minutes"`
    } `yaml:"pool"`
}
```

Добавить в `conf/config.yaml`:

```yaml
database:
  host: 127.0.0.1
  port: 3306
  user: cts_user
  password: cts_password
  database: ct_system
  tls:
    cert: conf/ssl/mysql-client-cert.pem
    key: conf/ssl/mysql-client-key.pem
    ca: conf/ssl/ca-cert.pem
  pool:
    max_open_conns: 25
    max_idle_conns: 10
    conn_max_lifetime_minutes: 5
    conn_max_idle_time_minutes: 2
```

**Время:** 30 минут

### Шаг 4: Интегрировать в main.go

Обновить `cmd/daemon/main.go`:

```go
import (
    "github.com/your-org/cts-core/internal/db"
    "time"
)

func main() {
    // ... (после загрузки config)

    // Initialize MySQL
    mysqlCfg := db.MySQLConfig{
        Host:     cfg.Database.Host,
        Port:     cfg.Database.Port,
        User:     cfg.Database.User,
        Password: cfg.Database.Password,
        Database: cfg.Database.Database,
        CertPath: cfg.Database.TLS.CertPath,
        KeyPath:  cfg.Database.TLS.KeyPath,
        CAPath:   cfg.Database.TLS.CAPath,
        MaxOpenConns:    cfg.Database.Pool.MaxOpenConns,
        MaxIdleConns:    cfg.Database.Pool.MaxIdleConns,
        ConnMaxLifetime: time.Duration(cfg.Database.Pool.ConnMaxLifetime) * time.Minute,
        ConnMaxIdleTime: time.Duration(cfg.Database.Pool.ConnMaxIdleTime) * time.Minute,
    }

    dbClient, err := db.NewMySQLClient(mysqlCfg, logger)
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to connect to MySQL")
    }
    defer dbClient.Close()

    logger.Info().Msg("MySQL connection established")

    // ... rest of application
}
```

**Время:** 30 минут

### Верификация 1.2.1

```bash
# Скомпилировать
make build

# Запустить (должен подключиться к MySQL)
./bin/cts-core -config conf/config.yaml

# Ожидаемый вывод:
# {"level":"info","time":"...","message":"MySQL connected to 127.0.0.1:3306 (mTLS enabled)"}
# {"level":"info","time":"...","message":"MySQL connection established"}
```

**Definition of Done:**
- [ ] `internal/db/mysql.go` создан (~150 строк)
- [ ] Retry logic с exponential backoff реализован
- [ ] Config обновлен (database секция)
- [ ] main.go интегрирован
- [ ] `make build` проходит без ошибок
- [ ] Приложение подключается к MySQL с mTLS

---

## 1.2.2: Database Models (3 часа)

### Шаг 1: Создать internal/db/models/trader.go

```go
package models

import (
    "database/sql"
    "time"
)

// Trader represents TRADER table
type Trader struct {
    TraderID          int64          `db:"trader_id"`
    Name              string         `db:"name"`
    IPAddress         string         `db:"ip_address"`
    Port              int            `db:"port"`
    CertificatePath   string         `db:"certificate_path"`
    Status            string         `db:"status"` // active, inactive, error
    LastHeartbeat     sql.NullTime   `db:"last_heartbeat"`
    LoadScore         sql.NullInt32  `db:"load_score"`
    ResourcesJSON     sql.NullString `db:"resources_json"`
    RegisteredAt      time.Time      `db:"registered_at"`
    UpdatedAt         time.Time      `db:"updated_at"`
}

// TraderSession represents TRADER_SESSION table
type TraderSession struct {
    SessionID      int64          `db:"session_id"`
    TraderID       int64          `db:"trader_id"`
    UserID         int64          `db:"user_id"`
    SessionToken   string         `db:"session_token"`
    IPAddress      string         `db:"ip_address"`
    UserAgent      sql.NullString `db:"user_agent"`
    CreatedAt      time.Time      `db:"created_at"`
    ExpiresAt      time.Time      `db:"expires_at"`
    LastActivityAt time.Time      `db:"last_activity_at"`
}
```

**Время:** 30 минут

### Шаг 2: Создать internal/db/models/exchange.go

```go
package models

import (
    "database/sql"
    "time"
)

// ExchangeLimits represents EXCHANGE_LIMITS table
type ExchangeLimits struct {
    LimitID       int64          `db:"limit_id"`
    ExchangeID    int64          `db:"exchange_id"`
    Symbol        string         `db:"symbol"`
    MinQty        sql.NullString `db:"min_qty"`        // DECIMAL as string
    MaxQty        sql.NullString `db:"max_qty"`
    StepSize      sql.NullString `db:"step_size"`
    MinNotional   sql.NullString `db:"min_notional"`
    PricePrecision sql.NullInt32 `db:"price_precision"`
    QtyPrecision   sql.NullInt32 `db:"qty_precision"`
    UpdatedAt      time.Time      `db:"updated_at"`
}

// TraderExchangeResource represents TRADER_EXCHANGE_RESOURCE table
type TraderExchangeResource struct {
    ResourceID     int64          `db:"resource_id"`
    TraderID       int64          `db:"trader_id"`
    ExchangeID     int64          `db:"exchange_id"`
    AccountID      int64          `db:"account_id"`
    ConnectionStatus string       `db:"connection_status"` // connected, disconnected, error
    LastSyncAt     sql.NullTime   `db:"last_sync_at"`
    ErrorMessage   sql.NullString `db:"error_message"`
    CreatedAt      time.Time      `db:"created_at"`
    UpdatedAt      time.Time      `db:"updated_at"`
}
```

**Время:** 30 минут

### Шаг 3: Создать internal/db/models/order.go

```go
package models

import (
    "database/sql"
    "time"
)

// ArbitrageOrder represents ARBITRAGE_ORDER table
type ArbitrageOrder struct {
    OrderID        int64          `db:"order_id"`
    TransID        int64          `db:"trans_id"`
    Side           string         `db:"side"` // buy, sell
    ExchangeID     int64          `db:"exchange_id"`
    AccountID      int64          `db:"account_id"`
    Symbol         string         `db:"symbol"`
    Quantity       string         `db:"quantity"` // DECIMAL as string
    Price          sql.NullString `db:"price"`
    Status         string         `db:"status"` // pending, filled, partially_filled, cancelled, failed
    ExchangeOrderID sql.NullString `db:"exchange_order_id"`
    FilledQty      sql.NullString `db:"filled_qty"`
    AvgPrice       sql.NullString `db:"avg_price"`
    Commission     sql.NullString `db:"commission"`
    CommissionAsset sql.NullString `db:"commission_asset"`
    ErrorMessage   sql.NullString `db:"error_message"`
    CreatedAt      time.Time      `db:"created_at"`
    UpdatedAt      time.Time      `db:"updated_at"`
}

// OrderTransaction represents ORDER_TRANSACTION table
type OrderTransaction struct {
    TxnID         int64          `db:"txn_id"`
    OrderID       int64          `db:"order_id"`
    TransID       int64          `db:"trans_id"`
    TradeID       sql.NullString `db:"trade_id"`
    Quantity      string         `db:"quantity"`
    Price         string         `db:"price"`
    Commission    sql.NullString `db:"commission"`
    CommissionAsset sql.NullString `db:"commission_asset"`
    TradeTime     sql.NullTime   `db:"trade_time"`
    CreatedAt     time.Time      `db:"created_at"`
}
```

**Время:** 30 минут

### Шаг 4: Создать internal/db/models/audit.go

```go
package models

import (
    "database/sql"
    "time"
)

// AuditLog represents AUDIT_LOG table
type AuditLog struct {
    LogID      int64          `db:"log_id"`
    UserID     sql.NullInt64  `db:"user_id"`
    TraderID   sql.NullInt64  `db:"trader_id"`
    Action     string         `db:"action"`
    Resource   sql.NullString `db:"resource"`
    ResourceID sql.NullInt64  `db:"resource_id"`
    IPAddress  sql.NullString `db:"ip_address"`
    Details    sql.NullString `db:"details"` // JSON
    CreatedAt  time.Time      `db:"created_at"`
}
```

**Время:** 15 минут

### Шаг 5: Создать internal/db/models/hsm.go

```go
package models

import (
    "time"
)

// ReencryptionJob represents REENCRYPTION_JOBS table
type ReencryptionJob struct {
    JobID           int64  `db:"job_id"`
    OldKeyVersion   int    `db:"old_key_version"`
    NewKeyVersion   int    `db:"new_key_version"`
    Status          string `db:"status"` // pending, in_progress, completed, failed
    TotalRecords    int    `db:"total_records"`
    ProcessedRecords int   `db:"processed_records"`
    FailedRecords   int    `db:"failed_records"`
    StartedAt       *time.Time `db:"started_at"`
    CompletedAt     *time.Time `db:"completed_at"`
    CreatedAt       time.Time  `db:"created_at"`
}

// ReencryptionProgress represents REENCRYPTION_PROGRESS table
type ReencryptionProgress struct {
    ProgressID int64  `db:"progress_id"`
    JobID      int64  `db:"job_id"`
    TableName  string `db:"table_name"`
    RecordID   int64  `db:"record_id"`
    Status     string `db:"status"` // success, failed
    Error      *string `db:"error"`
    ProcessedAt time.Time `db:"processed_at"`
}

// SchedulerTask represents SCHEDULER_TASKS table
type SchedulerTask struct {
    TaskID      int64      `db:"task_id"`
    TaskName    string     `db:"task_name"`
    TaskType    string     `db:"task_type"`
    Schedule    string     `db:"schedule"` // cron expression
    Enabled     bool       `db:"enabled"`
    LastRunAt   *time.Time `db:"last_run_at"`
    NextRunAt   *time.Time `db:"next_run_at"`
    Status      string     `db:"status"` // idle, running, failed
    CreatedAt   time.Time  `db:"created_at"`
    UpdatedAt   time.Time  `db:"updated_at"`
}
```

**Время:** 30 минutes

### Верификация 1.2.2

```bash
# Скомпилировать
make build

# Проверить что нет ошибок импорта
go list -m all
```

**Definition of Done:**
- [ ] 5 файлов моделей созданы (trader, exchange, order, audit, hsm)
- [ ] Все 11 таблиц Phase 1 покрыты
- [ ] `sql.Null*` типы использованы для nullable полей
- [ ] DECIMAL поля как string (для точности)
- [ ] `make build` проходит без ошибок

---

## 1.2.3: Repository Pattern (4 часа)

### Шаг 1: Создать internal/db/repository.go

```go
package db

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/your-org/cts-core/internal/db/models"
)

// Repository provides database operations
type Repository struct {
    client *MySQLClient
}

// NewRepository creates new repository
func NewRepository(client *MySQLClient) *Repository {
    return &Repository{client: client}
}

// ==================== TRADER ====================

// InsertTrader inserts new trader
func (r *Repository) InsertTrader(ctx context.Context, trader *models.Trader) (int64, error) {
    query := `
        INSERT INTO TRADER (name, ip_address, port, certificate_path, status, registered_at, updated_at)
        VALUES (?, ?, ?, ?, ?, NOW(), NOW())
    `

    var result sql.Result
    err := r.client.WithRetry(ctx, func() error {
        var execErr error
        result, execErr = r.client.db.ExecContext(ctx, query,
            trader.Name,
            trader.IPAddress,
            trader.Port,
            trader.CertificatePath,
            trader.Status,
        )
        return execErr
    })

    if err != nil {
        return 0, fmt.Errorf("failed to insert trader: %w", err)
    }

    return result.LastInsertId()
}

// GetTraderByID retrieves trader by ID
func (r *Repository) GetTraderByID(ctx context.Context, traderID int64) (*models.Trader, error) {
    query := `
        SELECT trader_id, name, ip_address, port, certificate_path, status,
               last_heartbeat, load_score, resources_json, registered_at, updated_at
        FROM TRADER
        WHERE trader_id = ?
    `

    trader := &models.Trader{}
    err := r.client.WithRetry(ctx, func() error {
        return r.client.db.QueryRowContext(ctx, query, traderID).Scan(
            &trader.TraderID,
            &trader.Name,
            &trader.IPAddress,
            &trader.Port,
            &trader.CertificatePath,
            &trader.Status,
            &trader.LastHeartbeat,
            &trader.LoadScore,
            &trader.ResourcesJSON,
            &trader.RegisteredAt,
            &trader.UpdatedAt,
        )
    })

    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("trader not found: %d", traderID)
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get trader: %w", err)
    }

    return trader, nil
}

// UpdateTraderStatus updates trader status
func (r *Repository) UpdateTraderStatus(ctx context.Context, traderID int64, status string) error {
    query := `
        UPDATE TRADER
        SET status = ?, updated_at = NOW()
        WHERE trader_id = ?
    `

    return r.client.WithRetry(ctx, func() error {
        result, err := r.client.db.ExecContext(ctx, query, status, traderID)
        if err != nil {
            return err
        }

        rows, err := result.RowsAffected()
        if err != nil {
            return err
        }

        if rows == 0 {
            return fmt.Errorf("trader not found: %d", traderID)
        }

        return nil
    })
}

// ListActiveTraders returns all active traders
func (r *Repository) ListActiveTraders(ctx context.Context) ([]*models.Trader, error) {
    query := `
        SELECT trader_id, name, ip_address, port, certificate_path, status,
               last_heartbeat, load_score, resources_json, registered_at, updated_at
        FROM TRADER
        WHERE status = 'active'
        ORDER BY load_score ASC
    `

    var traders []*models.Trader
    err := r.client.WithRetry(ctx, func() error {
        rows, err := r.client.db.QueryContext(ctx, query)
        if err != nil {
            return err
        }
        defer rows.Close()

        traders = make([]*models.Trader, 0)
        for rows.Next() {
            trader := &models.Trader{}
            err := rows.Scan(
                &trader.TraderID,
                &trader.Name,
                &trader.IPAddress,
                &trader.Port,
                &trader.CertificatePath,
                &trader.Status,
                &trader.LastHeartbeat,
                &trader.LoadScore,
                &trader.ResourcesJSON,
                &trader.RegisteredAt,
                &trader.UpdatedAt,
            )
            if err != nil {
                return err
            }
            traders = append(traders, trader)
        }

        return rows.Err()
    })

    if err != nil {
        return nil, fmt.Errorf("failed to list traders: %w", err)
    }

    return traders, nil
}

// UpdateTraderHeartbeat updates last_heartbeat and load_score
func (r *Repository) UpdateTraderHeartbeat(ctx context.Context, traderID int64, loadScore int32, resourcesJSON string) error {
    query := `
        UPDATE TRADER
        SET last_heartbeat = NOW(),
            load_score = ?,
            resources_json = ?,
            updated_at = NOW()
        WHERE trader_id = ?
    `

    return r.client.WithRetry(ctx, func() error {
        _, err := r.client.db.ExecContext(ctx, query, loadScore, resourcesJSON, traderID)
        return err
    })
}

// ==================== TRADER_SESSION ====================

// InsertTraderSession creates new session
func (r *Repository) InsertTraderSession(ctx context.Context, session *models.TraderSession) (int64, error) {
    query := `
        INSERT INTO TRADER_SESSION (trader_id, user_id, session_token, ip_address, user_agent, created_at, expires_at, last_activity_at)
        VALUES (?, ?, ?, ?, ?, NOW(), ?, NOW())
    `

    var result sql.Result
    err := r.client.WithRetry(ctx, func() error {
        var execErr error
        result, execErr = r.client.db.ExecContext(ctx, query,
            session.TraderID,
            session.UserID,
            session.SessionToken,
            session.IPAddress,
            session.UserAgent,
            session.ExpiresAt,
        )
        return execErr
    })

    if err != nil {
        return 0, fmt.Errorf("failed to insert session: %w", err)
    }

    return result.LastInsertId()
}

// GetTraderSessionByToken retrieves session by token
func (r *Repository) GetTraderSessionByToken(ctx context.Context, token string) (*models.TraderSession, error) {
    query := `
        SELECT session_id, trader_id, user_id, session_token, ip_address, user_agent, created_at, expires_at, last_activity_at
        FROM TRADER_SESSION
        WHERE session_token = ? AND expires_at > NOW()
    `

    session := &models.TraderSession{}
    err := r.client.WithRetry(ctx, func() error {
        return r.client.db.QueryRowContext(ctx, query, token).Scan(
            &session.SessionID,
            &session.TraderID,
            &session.UserID,
            &session.SessionToken,
            &session.IPAddress,
            &session.UserAgent,
            &session.CreatedAt,
            &session.ExpiresAt,
            &session.LastActivityAt,
        )
    })

    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("session not found or expired")
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get session: %w", err)
    }

    return session, nil
}

// UpdateSessionActivity updates last_activity_at
func (r *Repository) UpdateSessionActivity(ctx context.Context, sessionID int64) error {
    query := `UPDATE TRADER_SESSION SET last_activity_at = NOW() WHERE session_id = ?`

    return r.client.WithRetry(ctx, func() error {
        _, err := r.client.db.ExecContext(ctx, query, sessionID)
        return err
    })
}

// ==================== ARBITRAGE_ORDER ====================

// InsertArbitrageOrder creates new order
func (r *Repository) InsertArbitrageOrder(ctx context.Context, order *models.ArbitrageOrder) (int64, error) {
    query := `
        INSERT INTO ARBITRAGE_ORDER (trans_id, side, exchange_id, account_id, symbol, quantity, price, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
    `

    var result sql.Result
    err := r.client.WithRetry(ctx, func() error {
        var execErr error
        result, execErr = r.client.db.ExecContext(ctx, query,
            order.TransID,
            order.Side,
            order.ExchangeID,
            order.AccountID,
            order.Symbol,
            order.Quantity,
            order.Price,
            order.Status,
        )
        return execErr
    })

    if err != nil {
        return 0, fmt.Errorf("failed to insert order: %w", err)
    }

    return result.LastInsertId()
}

// UpdateOrderStatus updates order status and exchange_order_id
func (r *Repository) UpdateOrderStatus(ctx context.Context, orderID int64, status string, exchangeOrderID *string, errorMsg *string) error {
    query := `
        UPDATE ARBITRAGE_ORDER
        SET status = ?, exchange_order_id = ?, error_message = ?, updated_at = NOW()
        WHERE order_id = ?
    `

    return r.client.WithRetry(ctx, func() error {
        _, err := r.client.db.ExecContext(ctx, query, status, exchangeOrderID, errorMsg, orderID)
        return err
    })
}

// ==================== AUDIT_LOG ====================

// InsertAuditLog creates audit log entry
func (r *Repository) InsertAuditLog(ctx context.Context, log *models.AuditLog) error {
    query := `
        INSERT INTO AUDIT_LOG (user_id, trader_id, action, resource, resource_id, ip_address, details, created_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, NOW())
    `

    return r.client.WithRetry(ctx, func() error {
        _, err := r.client.db.ExecContext(ctx, query,
            log.UserID,
            log.TraderID,
            log.Action,
            log.Resource,
            log.ResourceID,
            log.IPAddress,
            log.Details,
        )
        return err
    })
}
```

**Время:** 3 часа

### Шаг 2: Создать internal/db/repository_test.go

```go
package db

import (
    "context"
    "database/sql"
    "testing"
    "time"

    "github.com/your-org/cts-core/internal/db/models"
    "github.com/rs/zerolog"
)

// NOTE: These are integration tests - require real MySQL connection
// Run with: go test -tags=integration ./internal/db/...

func setupTestDB(t *testing.T) *Repository {
    logger := zerolog.Nop()

    cfg := MySQLConfig{
        Host:     "127.0.0.1",
        Port:     3306,
        User:     "cts_user",
        Password: "cts_password",
        Database: "ct_system",
        CertPath: "../../conf/ssl/mysql-client-cert.pem",
        KeyPath:  "../../conf/ssl/mysql-client-key.pem",
        CAPath:   "../../conf/ssl/ca-cert.pem",
        MaxOpenConns:    10,
        MaxIdleConns:    5,
        ConnMaxLifetime: 5 * time.Minute,
        ConnMaxIdleTime: 2 * time.Minute,
    }

    client, err := NewMySQLClient(cfg, &logger)
    if err != nil {
        t.Fatalf("Failed to connect to MySQL: %v", err)
    }

    return NewRepository(client)
}

func TestInsertAndGetTrader(t *testing.T) {
    repo := setupTestDB(t)
    ctx := context.Background()

    trader := &models.Trader{
        Name:            "TestTrader",
        IPAddress:       "192.168.1.100",
        Port:            8443,
        CertificatePath: "/path/to/cert.pem",
        Status:          "active",
    }

    // Insert
    traderID, err := repo.InsertTrader(ctx, trader)
    if err != nil {
        t.Fatalf("Failed to insert trader: %v", err)
    }

    if traderID == 0 {
        t.Fatal("Expected non-zero trader ID")
    }

    // Get
    retrieved, err := repo.GetTraderByID(ctx, traderID)
    if err != nil {
        t.Fatalf("Failed to get trader: %v", err)
    }

    if retrieved.Name != trader.Name {
        t.Errorf("Expected name %s, got %s", trader.Name, retrieved.Name)
    }

    // Cleanup
    // Note: Add DELETE method in repository for cleanup
}

func TestListActiveTraders(t *testing.T) {
    repo := setupTestDB(t)
    ctx := context.Background()

    traders, err := repo.ListActiveTraders(ctx)
    if err != nil {
        t.Fatalf("Failed to list traders: %v", err)
    }

    // Should return at least test trader from previous test
    if len(traders) == 0 {
        t.Log("No active traders found (this is OK if database is empty)")
    }

    for _, trader := range traders {
        if trader.Status != "active" {
            t.Errorf("Expected active trader, got status: %s", trader.Status)
        }
    }
}

func TestInsertAndGetSession(t *testing.T) {
    repo := setupTestDB(t)
    ctx := context.Background()

    session := &models.TraderSession{
        TraderID:     1, // Assume trader exists
        UserID:       1, // Assume user exists
        SessionToken: "test-token-123",
        IPAddress:    "192.168.1.1",
        UserAgent:    sql.NullString{String: "TestAgent/1.0", Valid: true},
        ExpiresAt:    time.Now().Add(24 * time.Hour),
    }

    // Insert
    sessionID, err := repo.InsertTraderSession(ctx, session)
    if err != nil {
        t.Fatalf("Failed to insert session: %v", err)
    }

    if sessionID == 0 {
        t.Fatal("Expected non-zero session ID")
    }

    // Get by token
    retrieved, err := repo.GetTraderSessionByToken(ctx, session.SessionToken)
    if err != nil {
        t.Fatalf("Failed to get session: %v", err)
    }

    if retrieved.TraderID != session.TraderID {
        t.Errorf("Expected trader ID %d, got %d", session.TraderID, retrieved.TraderID)
    }
}
```

**Время:** 1 час

### Верификация 1.2.3

```bash
# Скомпилировать
make build

# Запустить тесты (integration)
go test -v -tags=integration ./internal/db/...

# Ожидаемый вывод:
# === RUN   TestInsertAndGetTrader
# --- PASS: TestInsertAndGetTrader (0.05s)
# === RUN   TestListActiveTraders
# --- PASS: TestListActiveTraders (0.02s)
# === RUN   TestInsertAndGetSession
# --- PASS: TestInsertAndGetSession (0.03s)
# PASS
```

**Definition of Done:**
- [ ] `internal/db/repository.go` создан (~400 строк)
- [ ] CRUD методы для TRADER, TRADER_SESSION, ARBITRAGE_ORDER, AUDIT_LOG
- [ ] Retry logic используется во всех методах
- [ ] `internal/db/repository_test.go` создан (integration tests)
- [ ] Tests проходят с реальной MySQL

---

## 1.2.4: Обновить main.go и добавить health check (1 час)

### Шаг 1: Интегрировать Repository в main.go

Обновить `cmd/daemon/main.go`:

```go
import (
    "github.com/your-org/cts-core/internal/db"
)

func main() {
    // ... (после dbClient)

    // Initialize repository
    repo := db.NewRepository(dbClient)

    // Test database connection
    ctx := context.Background()
    traders, err := repo.ListActiveTraders(ctx)
    if err != nil {
        logger.Warn().Err(err).Msg("Failed to list traders (database may be empty)")
    } else {
        logger.Info().Msgf("Found %d active traders", len(traders))
    }

    // ... rest of application
}
```

### Шаг 2: Добавить health check endpoint

Создать `internal/api/rest/health.go`:

```go
package rest

import (
    "context"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/your-org/cts-core/internal/db"
)

type HealthHandler struct {
    dbClient *db.MySQLClient
}

func NewHealthHandler(dbClient *db.MySQLClient) *HealthHandler {
    return &HealthHandler{dbClient: dbClient}
}

func (h *HealthHandler) Health(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
    defer cancel()

    // Check database
    dbStatus := "ok"
    err := h.dbClient.Ping()
    if err != nil {
        dbStatus = "error: " + err.Error()
    }

    response := gin.H{
        "status": "ok",
        "components": gin.H{
            "database": dbStatus,
        },
        "timestamp": time.Now().Unix(),
    }

    // If any component is down, return 503
    if dbStatus != "ok" {
        c.JSON(http.StatusServiceUnavailable, response)
        return
    }

    c.JSON(http.StatusOK, response)
}
```

**Время:** 30 минут

### Шаг 3: Обновить Makefile

Добавить в `Makefile`:

```makefile
# Database operations
.PHONY: db-ping
db-ping:
	@mysql -h 127.0.0.1 -u cts_user -pcts_password -e "SELECT 1;" ct_system

.PHONY: db-test
db-test:
	@go test -v -tags=integration ./internal/db/...
```

**Время:** 10 минут

### Верификация 1.2.4

```bash
# Ping database
make db-ping

# Run integration tests
make db-test

# Build and run
make build
./bin/cts-core -config conf/config.yaml

# Test health endpoint (if server is running)
curl -k https://localhost:8443/health
# Expected: {"status":"ok","components":{"database":"ok"},"timestamp":...}
```

**Definition of Done:**
- [ ] Repository интегрирован в main.go
- [ ] Health check endpoint создан
- [ ] Makefile обновлен (db-ping, db-test)
- [ ] Health endpoint возвращает database status

---

## Troubleshooting

### Проблема: "Error 1045: Access denied"

**Причина:** Неверные credentials или отсутствие пользователя.

**Решение:**
```sql
-- Создать пользователя
CREATE USER 'cts_user'@'%' IDENTIFIED BY 'cts_password';
GRANT ALL PRIVILEGES ON ct_system.* TO 'cts_user'@'%';
FLUSH PRIVILEGES;
```

### Проблема: "x509: certificate signed by unknown authority"

**Причина:** CA certificate не найден или неверный.

**Решение:**
1. Проверить путь к CA cert в config.yaml
2. Verify CA cert:
   ```bash
   openssl x509 -in conf/ssl/ca-cert.pem -text -noout
   ```

### Проблема: "connection refused"

**Причина:** MySQL не запущен или неверный host/port.

**Решение:**
```bash
# Проверить MySQL
systemctl status mysql

# Проверить порт
netstat -tlnp | grep 3306

# Проверить доступ
mysql -h 127.0.0.1 -u root -p -e "SELECT 1;"
```

### Проблема: "sql: database is closed"

**Причина:** Connection pool closed или max lifetime expired.

**Решение:**
- Увеличить `conn_max_lifetime_minutes` в config
- Check connection leaks (не закрытые rows)

### Проблема: Integration tests fail

**Причина:** Тесты требуют реальную MySQL connection.

**Решение:**
1. Убедиться что MySQL запущен
2. Создать test database:
   ```sql
   CREATE DATABASE ct_system_test;
   ```
3. Update test config to use `ct_system_test`

---

## FAQ

**Q: Почему DECIMAL хранится как string?**
A: В Go нет native decimal типа. Использование string или библиотеки как `shopspring/decimal` обеспечивает точность для финансовых операций. Не используйте float64 для денег!

**Q: Зачем retry logic?**
A: Network могут быть временные сбои. Retry с exponential backoff делает систему более resilient к transient errors.

**Q: Как работает connection pool?**
A: MySQL driver автоматически управляет pool. `MaxOpenConns` ограничивает максимум соединений, `MaxIdleConns` - сколько держать открытыми в idle. `ConnMaxLifetime` - когда переоткрывать соединение.

**Q: Почему использован repository pattern?**
A: Repository абстрагирует database logic от business logic. Упрощает testing (можно mock repository) и делает код более maintainable.

**Q: Как добавить транзакции?**
A: Добавить метод в Repository:
```go
func (r *Repository) WithTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
    tx, err := r.client.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    
    if err := fn(tx); err != nil {
        tx.Rollback()
        return err
    }
    
    return tx.Commit()
}
```

**Q: Что делать с connection leaks?**
A: Всегда закрывать `rows.Close()` в defer. Использовать `defer` сразу после `Query()`.

---

## Summary Phase 1.2

**Созданные файлы:**
- `internal/db/mysql.go` (~200 строк)
- `internal/db/repository.go` (~400 строк)
- `internal/db/repository_test.go` (~150 строк)
- `internal/db/models/trader.go` (~30 строк)
- `internal/db/models/exchange.go` (~50 строк)
- `internal/db/models/order.go` (~60 строк)
- `internal/db/models/audit.go` (~20 строк)
- `internal/db/models/hsm.go` (~50 строк)
- `internal/api/rest/health.go` (~40 строк)

**Total LOC:** ~1000 строк

**Обновленные файлы:**
- `internal/config/types.go` (добавлен DatabaseConfig)
- `conf/config.yaml` (добавлена database секция)
- `cmd/daemon/main.go` (добавлен MySQL + Repository)
- `Makefile` (db-ping, db-test targets)

**Deliverables:**
✅ MySQL connection pool с mTLS  
✅ Retry logic (exponential backoff)  
✅ Repository pattern (CRUD для 5 core tables)  
✅ 11 database models  
✅ Integration tests  
✅ Health check endpoint  

**Next Phase:** Phase 1.3 - HSM Client

---

## Definition of Done - Phase 1.2

- [ ] Все файлы созданы и скомпилированы
- [ ] `make build` проходит без ошибок
- [ ] `make db-test` проходит (integration tests)
- [ ] MySQL connection с mTLS работает
- [ ] Repository CRUD методы протестированы
- [ ] Health endpoint возвращает database status
- [ ] Закоммичено в git:
  ```bash
  git add internal/db/ internal/api/rest/health.go conf/config.yaml cmd/daemon/main.go Makefile
  git commit -m "Phase 1.2: MySQL connection pool and repository pattern"
  ```
- [ ] `guides/phase_1_2_mysql_pool.md` удален (после завершения фазы)
