package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// traderRepository implements TraderRepository interface
type traderRepository struct {
	db sqlx.ExtContext
}

// NewTraderRepository creates a new trader repository
func NewTraderRepository(db *sqlx.DB) TraderRepository {
	return &traderRepository{db: db}
}

// NewTraderRepositoryWithTx creates a trader repository with transaction context
func NewTraderRepositoryWithTx(tx *sqlx.Tx) TraderRepository {
	return &traderRepository{db: tx}
}

// Create inserts a new trader
func (r *traderRepository) Create(ctx context.Context, trader *models.Trader) error {
	query := `
		INSERT INTO TRADER (
			TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS,
			DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY, NOTES
		) VALUES (
			:trader_name, :certificate_cn, :region, :status, :max_tasks,
			:date_create, :date_modify, :user_created, :user_modify, :notes
		)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, trader)
	if err != nil {
		return fmt.Errorf("failed to create trader: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	trader.ID = int(id)
	return nil
}

// GetByID retrieves a trader by ID
func (r *traderRepository) GetByID(ctx context.Context, id int) (*models.Trader, error) {
	query := `
		SELECT ID, TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY, NOTES
		FROM TRADER
		WHERE ID = ?
	`

	var trader models.Trader
	err := sqlx.GetContext(ctx, r.db, &trader, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trader by id: %w", err)
	}

	return &trader, nil
}

// GetByCertificateCN retrieves a trader by certificate Common Name
func (r *traderRepository) GetByCertificateCN(ctx context.Context, cn string) (*models.Trader, error) {
	query := `
		SELECT ID, TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY, NOTES
		FROM TRADER
		WHERE CERTIFICATE_CN = ?
	`

	var trader models.Trader
	err := sqlx.GetContext(ctx, r.db, &trader, query, cn)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trader by certificate CN: %w", err)
	}

	return &trader, nil
}

// GetOrCreateByCertificateCN retrieves existing trader by CN or creates a pending trader atomically.
// created=true means a new trader row was inserted.
func (r *traderRepository) GetOrCreateByCertificateCN(ctx context.Context, cn string) (*models.Trader, bool, error) {
	cn = strings.TrimSpace(cn)
	if cn == "" {
		return nil, false, fmt.Errorf("certificate CN is required")
	}

	query := `
		INSERT INTO TRADER (
			TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS,
			DATE_CREATE, DATE_MODIFY
		) VALUES (
			?, ?, ?, ?, ?, ?, ?
		)
		ON DUPLICATE KEY UPDATE
			ID = LAST_INSERT_ID(ID),
			DATE_MODIFY = DATE_MODIFY
	`

	now := time.Now().UTC()
	result, err := r.db.ExecContext(
		ctx,
		query,
		cn,
		cn,
		"unknown",
		models.TraderStatusPending,
		0,
		now,
		now,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get-or-create trader by certificate CN: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("failed to read rows affected after get-or-create: %w", err)
	}
	created := rowsAffected == 1

	id, err := result.LastInsertId()
	if err != nil {
		return nil, false, fmt.Errorf("failed to read trader id after get-or-create: %w", err)
	}

	trader, err := r.GetByID(ctx, int(id))
	if err != nil {
		return nil, false, err
	}
	if trader == nil {
		return nil, false, fmt.Errorf("trader id not found after get-or-create: id=%d", id)
	}

	return trader, created, nil
}

// List retrieves all traders with optional status filter
func (r *traderRepository) List(ctx context.Context, status *models.TraderStatus) ([]*models.Trader, error) {
	query := `
		SELECT ID, TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY, NOTES
		FROM TRADER
	`
	args := []interface{}{}

	if status != nil {
		query += " WHERE STATUS = ?"
		args = append(args, *status)
	}

	query += " ORDER BY ID"

	var traders []*models.Trader
	err := sqlx.SelectContext(ctx, r.db, &traders, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list traders: %w", err)
	}

	return traders, nil
}

// Update updates trader fields
func (r *traderRepository) Update(ctx context.Context, trader *models.Trader) error {
	trader.DateModify = time.Now()

	query := `
		UPDATE TRADER
		SET TRADER_NAME = :trader_name,
		    CERTIFICATE_CN = :certificate_cn,
		    REGION = :region,
		    STATUS = :status,
		    MAX_TASKS = :max_tasks,
		    DATE_MODIFY = :date_modify,
		    USER_MODIFY = :user_modify,
		    NOTES = :notes
		WHERE ID = :id
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, trader)
	if err != nil {
		return fmt.Errorf("failed to update trader: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("trader not found: id=%d", trader.ID)
	}

	return nil
}

// UpdateStatus changes trader status
func (r *traderRepository) UpdateStatus(ctx context.Context, id int, status models.TraderStatus) error {
	query := `
		UPDATE TRADER
		SET STATUS = ?,
		    DATE_MODIFY = ?
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update trader status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("trader not found: id=%d", id)
	}

	return nil
}

// Delete soft-deletes a trader (sets status to decommissioned)
func (r *traderRepository) Delete(ctx context.Context, id int) error {
	return r.UpdateStatus(ctx, id, models.TraderStatusDecommissioned)
}
