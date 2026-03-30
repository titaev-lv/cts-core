package repository

import (
	"context"
	"database/sql"

	"github.com/titaev-lv/cts-core/internal/db/models"
)

// TraderRepository defines operations for Trader management
type TraderRepository interface {
	// Create inserts a new trader
	Create(ctx context.Context, trader *models.Trader) error
	// GetByID retrieves a trader by ID
	GetByID(ctx context.Context, id int) (*models.Trader, error)
	// GetByCertificateCN retrieves a trader by certificate Common Name (for mTLS auth)
	GetByCertificateCN(ctx context.Context, cn string) (*models.Trader, error)
	// GetOrCreateByCertificateCN returns existing trader by certificate CN or creates pending trader atomically.
	// The created flag is true only when a new trader row was inserted.
	GetOrCreateByCertificateCN(ctx context.Context, cn string) (*models.Trader, bool, error)
	// List retrieves all traders with optional status filter
	List(ctx context.Context, status *models.TraderStatus) ([]*models.Trader, error)
	// Update updates trader fields
	Update(ctx context.Context, trader *models.Trader) error
	// UpdateStatus changes trader status
	UpdateStatus(ctx context.Context, id int, status models.TraderStatus) error
	// Delete soft-deletes a trader (sets status to decommissioned)
	Delete(ctx context.Context, id int) error
}

// TraderSessionRepository defines operations for Trader sessions
type TraderSessionRepository interface {
	// Create starts a new session
	Create(ctx context.Context, session *models.TraderSession) error
	// GetByID retrieves a session by ID
	GetByID(ctx context.Context, id int64) (*models.TraderSession, error)
	// GetBySessionID retrieves a session by UUID
	GetBySessionID(ctx context.Context, sessionID string) (*models.TraderSession, error)
	// GetActiveByTraderID retrieves active session for a trader
	GetActiveByTraderID(ctx context.Context, traderID int) (*models.TraderSession, error)
	// ListActive retrieves all active sessions
	ListActive(ctx context.Context) ([]*models.TraderSession, error)
	// UpdateHeartbeat updates last heartbeat timestamp
	UpdateHeartbeat(ctx context.Context, sessionID string) error
	// EndSession marks session as ended
	EndSession(ctx context.Context, sessionID string, reason models.DisconnectReason, errorMsg *string) error
	// CleanupOldSessions deletes sessions older than retention period
	CleanupOldSessions(ctx context.Context, retentionDays int) (int64, error)
}

// TraderResourceRepository defines operations for exchange resource tracking
type TraderResourceRepository interface {
	// Upsert creates or updates resource usage
	Upsert(ctx context.Context, resource *models.TraderExchangeResource) error
	// GetByTraderAndExchange retrieves resources for trader on specific exchange
	GetByTraderAndExchange(ctx context.Context, traderID, exchangeID int, accountID *int32) ([]*models.TraderExchangeResource, error)
	// IncrementUsage atomically increments current value
	IncrementUsage(ctx context.Context, id int, increment int) error
	// ResetUsage sets current value to 0
	ResetUsage(ctx context.Context, id int) error
	// GetAvailable returns resources with available capacity
	GetAvailable(ctx context.Context, exchangeID int, resourceType models.ExchangeResourceType, minAvailable int) ([]*models.TraderExchangeResource, error)
}

// ArbitrageOrderRepository defines operations for arbitrage orders
type ArbitrageOrderRepository interface {
	// Create inserts a new arbitrage order
	Create(ctx context.Context, order *models.ArbitrageOrder) error
	// GetByID retrieves an order by ID
	GetByID(ctx context.Context, id int64) (*models.ArbitrageOrder, error)
	// GetByExchangeOrderID retrieves order by exchange order ID (idempotency)
	GetByExchangeOrderID(ctx context.Context, exchangeID int, exchangeOrderID string) (*models.ArbitrageOrder, error)
	// ListByArbitrageTransID retrieves all orders for arbitrage transaction
	ListByArbitrageTransID(ctx context.Context, arbitrageTransID int64) ([]*models.ArbitrageOrder, error)
	// UpdateStatus updates order status and fills
	UpdateStatus(ctx context.Context, id int64, status models.OrderStatus, filledQty, avgPrice, totalFee *float64, feeCurrency *string) error
	// AddFill creates a fill record (ArbitrageOrderTrans)
	AddFill(ctx context.Context, fill *models.ArbitrageOrderTrans) error
	// GetFills retrieves all fills for an order
	GetFills(ctx context.Context, orderID int64) ([]*models.ArbitrageOrderTrans, error)
}

// AuditLogRepository defines operations for audit logging
type AuditLogRepository interface {
	// Create inserts a new audit log entry
	Create(ctx context.Context, log *models.AuditLog) error
	// List retrieves audit logs with filters
	List(ctx context.Context, filters AuditLogFilters, limit, offset int) ([]*models.AuditLog, error)
	// Count returns total count matching filters
	Count(ctx context.Context, filters AuditLogFilters) (int64, error)
	// CleanupOld deletes audit logs older than retention period
	CleanupOld(ctx context.Context, retentionDays int) (int64, error)
}

// AuditLogFilters defines filters for audit log queries
type AuditLogFilters struct {
	UID          *int32
	Action       *string
	ResourceType *string
	ResourceID   *string
	Success      *bool
	FromDate     *string
	ToDate       *string
}

// ReencryptionJobRepository defines operations for re-encryption jobs
type ReencryptionJobRepository interface {
	// Create inserts a new job
	Create(ctx context.Context, job *models.ReencryptionJob) error
	// GetByID retrieves a job by ID
	GetByID(ctx context.Context, id int) (*models.ReencryptionJob, error)
	// ListPending retrieves all pending jobs
	ListPending(ctx context.Context) ([]*models.ReencryptionJob, error)
	// UpdateStatus updates job status
	UpdateStatus(ctx context.Context, id int, status models.ReencryptionJobStatus) error
	// UpdateProgress updates job progress counters
	UpdateProgress(ctx context.Context, id int, processed, failed int) error
	// Complete marks job as completed
	Complete(ctx context.Context, id int) error
	// Fail marks job as failed with error message
	Fail(ctx context.Context, id int, errorMsg string) error
}

// ReencryptionProgressRepository defines operations for re-encryption progress tracking
type ReencryptionProgressRepository interface {
	// Create inserts progress records in batch
	CreateBatch(ctx context.Context, records []*models.ReencryptionProgress) error
	// GetPendingBatch retrieves pending records for processing
	GetPendingBatch(ctx context.Context, jobID, batchSize int) ([]*models.ReencryptionProgress, error)
	// UpdateStatus updates record status
	UpdateStatus(ctx context.Context, id int64, status models.ReencryptionProgressStatus, errorMsg *string) error
	// IncrementAttempts increments processing attempts counter
	IncrementAttempts(ctx context.Context, id int64) error
	// GetFailedRecords retrieves failed records for retry
	GetFailedRecords(ctx context.Context, jobID, maxAttempts int) ([]*models.ReencryptionProgress, error)
}

// SchedulerTaskRepository defines operations for scheduler tasks
type SchedulerTaskRepository interface {
	// Create inserts a new task
	Create(ctx context.Context, task *models.SchedulerTask) error
	// GetByID retrieves a task by ID
	GetByID(ctx context.Context, id int) (*models.SchedulerTask, error)
	// GetByName retrieves a task by unique name
	GetByName(ctx context.Context, name string) (*models.SchedulerTask, error)
	// List retrieves all tasks with optional status filter
	List(ctx context.Context, enabled *bool) ([]*models.SchedulerTask, error)
	// GetDueTasks retrieves tasks that should run now
	GetDueTasks(ctx context.Context) ([]*models.SchedulerTask, error)
	// Update updates task configuration
	Update(ctx context.Context, task *models.SchedulerTask) error
	// UpdateStatus updates task status
	UpdateStatus(ctx context.Context, id int, status models.TaskStatus) error
	// RecordExecution records task execution result
	RecordExecution(ctx context.Context, id int, durationMS int32, status models.TaskRunStatus, errorMsg *string) error
	// UpdateNextRun calculates and updates next run time
	UpdateNextRun(ctx context.Context, id int) error
	// Delete removes a task
	Delete(ctx context.Context, id int) error
}

// ExchangeInfo describes a distinct exchange used by active tasks.
type ExchangeInfo struct {
	ExchangeID   int    `db:"exchange_id"`
	ExchangeName string `db:"exchange_name"`
}

// ExchangeRequirementsRepository defines operations for active exchange sets.
type ExchangeRequirementsRepository interface {
	// ListTradeExchanges returns distinct active exchanges used by active TRADE tasks.
	ListTradeExchanges(ctx context.Context) ([]ExchangeInfo, error)
	// ListMonitorExchanges returns distinct active exchanges used by active MONITORING tasks.
	ListMonitorExchanges(ctx context.Context) ([]ExchangeInfo, error)
}

// Transactor defines transaction support
type Transactor interface {
	// BeginTx starts a new transaction
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
	// WithTx executes a function within a transaction
	WithTx(ctx context.Context, fn func(tx Tx) error) error
}

// Tx represents a database transaction
type Tx interface {
	Commit() error
	Rollback() error

	// Repository accessors with transaction context
	Trader() TraderRepository
	TraderSession() TraderSessionRepository
	TraderResource() TraderResourceRepository
	ArbitrageOrder() ArbitrageOrderRepository
	AuditLog() AuditLogRepository
	ReencryptionJob() ReencryptionJobRepository
	ReencryptionProgress() ReencryptionProgressRepository
	SchedulerTask() SchedulerTaskRepository
	ExchangeRequirements() ExchangeRequirementsRepository
}
