package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// arbitrageOrderRepository implements ArbitrageOrderRepository interface
type arbitrageOrderRepository struct {
	db sqlx.ExtContext
}

// NewArbitrageOrderRepository creates a new arbitrage order repository
func NewArbitrageOrderRepository(db *sqlx.DB) ArbitrageOrderRepository {
	return &arbitrageOrderRepository{db: db}
}

// NewArbitrageOrderRepositoryWithTx creates an arbitrage order repository with transaction context
func NewArbitrageOrderRepositoryWithTx(tx *sqlx.Tx) ArbitrageOrderRepository {
	return &arbitrageOrderRepository{db: tx}
}

// Create inserts a new arbitrage order
func (r *arbitrageOrderRepository) Create(ctx context.Context, order *models.ArbitrageOrder) error {
	query := `
		INSERT INTO ARBITRAGE_ORDER (
			ARBITRAGE_TRANS_ID, EXCHANGE_ID, EXCHANGE_ACCOUNT_ID, EXCHANGE_ORDER_ID,
			TRADER_ID, SIDE, ORDER_TYPE, PAIR_ID, REQUESTED_QUANTITY,
			FILLED_QUANTITY, AVG_PRICE, TOTAL_COST, TOTAL_FEE, FEE_CURRENCY,
			STATUS, ERROR_MESSAGE, DATE_CREATE
		) VALUES (
			:ARBITRAGE_TRANS_ID, :EXCHANGE_ID, :EXCHANGE_ACCOUNT_ID, :EXCHANGE_ORDER_ID,
			:TRADER_ID, :SIDE, :ORDER_TYPE, :PAIR_ID, :REQUESTED_QUANTITY,
			:FILLED_QUANTITY, :AVG_PRICE, :TOTAL_COST, :TOTAL_FEE, :FEE_CURRENCY,
			:STATUS, :ERROR_MESSAGE, :DATE_CREATE
		)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, order)
	if err != nil {
		return fmt.Errorf("failed to create arbitrage order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	order.ID = id
	return nil
}

// GetByID retrieves an order by ID
func (r *arbitrageOrderRepository) GetByID(ctx context.Context, id int64) (*models.ArbitrageOrder, error) {
	query := `SELECT * FROM ARBITRAGE_ORDER WHERE ID = ?`

	var order models.ArbitrageOrder
	err := sqlx.GetContext(ctx, r.db, &order, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get arbitrage order: %w", err)
	}

	return &order, nil
}

// GetByExchangeOrderID retrieves order by exchange order ID (idempotency)
func (r *arbitrageOrderRepository) GetByExchangeOrderID(ctx context.Context, exchangeID int, exchangeOrderID string) (*models.ArbitrageOrder, error) {
	query := `
		SELECT * FROM ARBITRAGE_ORDER
		WHERE EXCHANGE_ID = ? AND EXCHANGE_ORDER_ID = ?
	`

	var order models.ArbitrageOrder
	err := sqlx.GetContext(ctx, r.db, &order, query, exchangeID, exchangeOrderID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get arbitrage order by exchange order id: %w", err)
	}

	return &order, nil
}

// ListByArbitrageTransID retrieves all orders for arbitrage transaction
func (r *arbitrageOrderRepository) ListByArbitrageTransID(ctx context.Context, arbitrageTransID int64) ([]*models.ArbitrageOrder, error) {
	query := `
		SELECT * FROM ARBITRAGE_ORDER
		WHERE ARBITRAGE_TRANS_ID = ?
		ORDER BY ID
	`

	var orders []*models.ArbitrageOrder
	err := sqlx.SelectContext(ctx, r.db, &orders, query, arbitrageTransID)
	if err != nil {
		return nil, fmt.Errorf("failed to list arbitrage orders: %w", err)
	}

	return orders, nil
}

// UpdateStatus updates order status and fills
func (r *arbitrageOrderRepository) UpdateStatus(ctx context.Context, id int64, status models.OrderStatus, filledQty, avgPrice, totalFee *float64, feeCurrency *string) error {
	query := `
		UPDATE ARBITRAGE_ORDER
		SET STATUS = ?,
		    FILLED_QUANTITY = COALESCE(?, FILLED_QUANTITY),
		    AVG_PRICE = COALESCE(?, AVG_PRICE),
		    TOTAL_FEE = COALESCE(?, TOTAL_FEE),
		    FEE_CURRENCY = COALESCE(?, FEE_CURRENCY)
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, filledQty, avgPrice, totalFee, feeCurrency, id)
	if err != nil {
		return fmt.Errorf("failed to update arbitrage order status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("arbitrage order not found: %d", id)
	}

	return nil
}

// AddFill creates a fill record (ArbitrageOrderTrans)
func (r *arbitrageOrderRepository) AddFill(ctx context.Context, fill *models.ArbitrageOrderTrans) error {
	query := `
		INSERT INTO ARBITRAGE_ORDER_TRANS (
			ORDER_ID, EXEC_QUANTITY, EXEC_PRICE, EXEC_COST,
			FEE_AMOUNT, FEE_CURRENCY, EXEC_TIMESTAMP
		) VALUES (
			:ORDER_ID, :EXEC_QUANTITY, :EXEC_PRICE, :EXEC_COST,
			:FEE_AMOUNT, :FEE_CURRENCY, :EXEC_TIMESTAMP
		)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, fill)
	if err != nil {
		return fmt.Errorf("failed to add fill: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	fill.ID = id
	return nil
}

// GetFills retrieves all fills for an order
func (r *arbitrageOrderRepository) GetFills(ctx context.Context, orderID int64) ([]*models.ArbitrageOrderTrans, error) {
	query := `
		SELECT * FROM ARBITRAGE_ORDER_TRANS
		WHERE ORDER_ID = ?
		ORDER BY EXEC_TIMESTAMP
	`

	var fills []*models.ArbitrageOrderTrans
	err := sqlx.SelectContext(ctx, r.db, &fills, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fills: %w", err)
	}

	return fills, nil
}
