package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Repository provides access to all repositories
type Repository struct {
	db *sqlx.DB

	trader               TraderRepository
	traderSession        TraderSessionRepository
	traderResource       TraderResourceRepository
	arbitrageOrder       ArbitrageOrderRepository
	auditLog             AuditLogRepository
	reencryptionJob      ReencryptionJobRepository
	reencryptionProgress ReencryptionProgressRepository
	schedulerTask        SchedulerTaskRepository
}

// New creates a new repository instance
func New(db *sqlx.DB) *Repository {
	return &Repository{
		db:                   db,
		trader:               NewTraderRepository(db),
		traderSession:        NewTraderSessionRepository(db),
		traderResource:       NewTraderResourceRepository(db),
		arbitrageOrder:       NewArbitrageOrderRepository(db),
		auditLog:             NewAuditLogRepository(db),
		reencryptionJob:      NewReencryptionJobRepository(db),
		reencryptionProgress: NewReencryptionProgressRepository(db),
		schedulerTask:        NewSchedulerTaskRepository(db),
	}
}

// Trader returns the trader repository
func (r *Repository) Trader() TraderRepository {
	return r.trader
}

// TraderSession returns the trader session repository
func (r *Repository) TraderSession() TraderSessionRepository {
	return r.traderSession
}

// TraderResource returns the trader resource repository
func (r *Repository) TraderResource() TraderResourceRepository {
	return r.traderResource
}

// ArbitrageOrder returns the arbitrage order repository
func (r *Repository) ArbitrageOrder() ArbitrageOrderRepository {
	return r.arbitrageOrder
}

// AuditLog returns the audit log repository
func (r *Repository) AuditLog() AuditLogRepository {
	return r.auditLog
}

// ReencryptionJob returns the re-encryption job repository
func (r *Repository) ReencryptionJob() ReencryptionJobRepository {
	return r.reencryptionJob
}

// ReencryptionProgress returns the re-encryption progress repository
func (r *Repository) ReencryptionProgress() ReencryptionProgressRepository {
	return r.reencryptionProgress
}

// SchedulerTask returns the scheduler task repository
func (r *Repository) SchedulerTask() SchedulerTaskRepository {
	return r.schedulerTask
}

// BeginTx starts a new transaction
func (r *Repository) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := r.db.BeginTxx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &transaction{
		tx: tx,
		db: r.db,
	}, nil
}

// WithTx executes a function within a transaction
func (r *Repository) WithTx(ctx context.Context, fn func(tx Tx) error) error {
	tx, err := r.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}

// transaction implements Tx interface
type transaction struct {
	tx *sqlx.Tx
	db *sqlx.DB
}

// Commit commits the transaction
func (t *transaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *transaction) Rollback() error {
	return t.tx.Rollback()
}

// Trader returns trader repository with transaction context
func (t *transaction) Trader() TraderRepository {
	return NewTraderRepositoryWithTx(t.tx)
}

// TraderSession returns trader session repository with transaction context
func (t *transaction) TraderSession() TraderSessionRepository {
	return NewTraderSessionRepositoryWithTx(t.tx)
}

// TraderResource returns trader resource repository with transaction context
func (t *transaction) TraderResource() TraderResourceRepository {
	return NewTraderResourceRepositoryWithTx(t.tx)
}

// ArbitrageOrder returns arbitrage order repository with transaction context
func (t *transaction) ArbitrageOrder() ArbitrageOrderRepository {
	return NewArbitrageOrderRepositoryWithTx(t.tx)
}

// AuditLog returns audit log repository with transaction context
func (t *transaction) AuditLog() AuditLogRepository {
	return NewAuditLogRepositoryWithTx(t.tx)
}

// ReencryptionJob returns re-encryption job repository with transaction context
func (t *transaction) ReencryptionJob() ReencryptionJobRepository {
	return NewReencryptionJobRepositoryWithTx(t.tx)
}

// ReencryptionProgress returns re-encryption progress repository with transaction context
func (t *transaction) ReencryptionProgress() ReencryptionProgressRepository {
	return NewReencryptionProgressRepositoryWithTx(t.tx)
}

// SchedulerTask returns scheduler task repository with transaction context
func (t *transaction) SchedulerTask() SchedulerTaskRepository {
	return NewSchedulerTaskRepositoryWithTx(t.tx)
}
