package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// traderResourceRepository implements TraderResourceRepository interface
type traderResourceRepository struct {
	db sqlx.ExtContext
}

// NewTraderResourceRepository creates a new trader resource repository
func NewTraderResourceRepository(db *sqlx.DB) TraderResourceRepository {
	return &traderResourceRepository{db: db}
}

// NewTraderResourceRepositoryWithTx creates a trader resource repository with transaction context
func NewTraderResourceRepositoryWithTx(tx *sqlx.Tx) TraderResourceRepository {
	return &traderResourceRepository{db: tx}
}

// Upsert creates or updates resource usage
func (r *traderResourceRepository) Upsert(ctx context.Context, resource *models.TraderExchangeResource) error {
	query := `
		INSERT INTO TRADER_EXCHANGE_RESOURCE (
			TRADER_ID, EXCHANGE_ID, EXCHANGE_ACCOUNT_ID, RESOURCE_TYPE,
			USED_VALUE, MAX_VALUE, LAST_UPDATED, RESET_AT
		) VALUES (
			:TRADER_ID, :EXCHANGE_ID, :EXCHANGE_ACCOUNT_ID, :RESOURCE_TYPE,
			:USED_VALUE, :MAX_VALUE, :LAST_UPDATED, :RESET_AT
		)
		ON DUPLICATE KEY UPDATE
			USED_VALUE = VALUES(USED_VALUE),
			MAX_VALUE = VALUES(MAX_VALUE),
			LAST_UPDATED = VALUES(LAST_UPDATED),
			RESET_AT = VALUES(RESET_AT)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, resource)
	if err != nil {
		return fmt.Errorf("failed to upsert trader resource: %w", err)
	}

	// If INSERT, set ID
	if resource.ID == 0 {
		id, _ := result.LastInsertId()
		resource.ID = id
	}

	return nil
}

// GetByTraderAndExchange retrieves resources for trader on specific exchange
func (r *traderResourceRepository) GetByTraderAndExchange(ctx context.Context, traderID, exchangeID int, accountID *int32) ([]*models.TraderExchangeResource, error) {
	var query string
	var args []interface{}

	if accountID == nil {
		query = `
			SELECT * FROM TRADER_EXCHANGE_RESOURCE
			WHERE TRADER_ID = ? AND EXCHANGE_ID = ? AND EXCHANGE_ACCOUNT_ID IS NULL
		`
		args = []interface{}{traderID, exchangeID}
	} else {
		query = `
			SELECT * FROM TRADER_EXCHANGE_RESOURCE
			WHERE TRADER_ID = ? AND EXCHANGE_ID = ? AND EXCHANGE_ACCOUNT_ID = ?
		`
		args = []interface{}{traderID, exchangeID, *accountID}
	}

	var resources []*models.TraderExchangeResource
	err := sqlx.SelectContext(ctx, r.db, &resources, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get trader resources: %w", err)
	}

	return resources, nil
}

// IncrementUsage atomically increments current value
func (r *traderResourceRepository) IncrementUsage(ctx context.Context, id int, increment int) error {
	query := `
		UPDATE TRADER_EXCHANGE_RESOURCE
		SET USED_VALUE = USED_VALUE + ?,
		    LAST_UPDATED = NOW()
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, increment, id)
	if err != nil {
		return fmt.Errorf("failed to increment resource usage: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("trader resource not found: %d", id)
	}

	return nil
}

// ResetUsage sets current value to 0
func (r *traderResourceRepository) ResetUsage(ctx context.Context, id int) error {
	query := `
		UPDATE TRADER_EXCHANGE_RESOURCE
		SET USED_VALUE = 0,
		    LAST_UPDATED = NOW()
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to reset resource usage: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("trader resource not found: %d", id)
	}

	return nil
}

// GetAvailable returns resources with available capacity
func (r *traderResourceRepository) GetAvailable(ctx context.Context, exchangeID int, resourceType models.ExchangeResourceType, minAvailable int) ([]*models.TraderExchangeResource, error) {
	query := `
		SELECT * FROM TRADER_EXCHANGE_RESOURCE
		WHERE EXCHANGE_ID = ?
		  AND RESOURCE_TYPE = ?
		  AND (MAX_VALUE - USED_VALUE) >= ?
		  AND RESET_AT > NOW()
		ORDER BY (MAX_VALUE - USED_VALUE) DESC
	`

	var resources []*models.TraderExchangeResource
	err := sqlx.SelectContext(ctx, r.db, &resources, query, exchangeID, resourceType, minAvailable)
	if err != nil {
		return nil, fmt.Errorf("failed to get available resources: %w", err)
	}

	return resources, nil
}
