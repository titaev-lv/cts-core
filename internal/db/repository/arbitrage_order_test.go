package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

func TestArbitrageOrderRepository_Create(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext)")
}

func TestArbitrageOrderRepository_GetByID_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewArbitrageOrderRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "ARBITRAGE_TRANS_ID", "EXCHANGE_ID", "EXCHANGE_ACCOUNT_ID", "EXCHANGE_ORDER_ID",
		"TRADER_ID", "SIDE", "ORDER_TYPE", "PAIR_ID", "REQUESTED_QUANTITY",
		"FILLED_QUANTITY", "AVG_PRICE", "TOTAL_COST", "TOTAL_FEE", "FEE_CURRENCY",
		"STATUS", "ERROR_MESSAGE", "DATE_CREATE",
	}).AddRow(
		1, 100, 2, 123, "exchange-order-123",
		1, "BUY", "LIMIT", 42, 1.5,
		1.5, 50000.0, 75000.0, 7.5, "USDT",
		models.OrderStatusFilled, nil, now,
	)

	mock.ExpectQuery("SELECT .* FROM ARBITRAGE_ORDER WHERE ID").WithArgs(int64(1)).WillReturnRows(rows)

	order, err := repo.GetByID(context.Background(), 1)

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, int64(1), order.ID)
	assert.Equal(t, int64(100), order.ArbitrageTransID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArbitrageOrderRepository_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewArbitrageOrderRepository(sqlxDB)

	mock.ExpectQuery("SELECT .* FROM ARBITRAGE_ORDER WHERE ID").
		WithArgs(int64(999)).
		WillReturnError(sql.ErrNoRows)

	order, err := repo.GetByID(context.Background(), 999)

	assert.NoError(t, err)
	assert.Nil(t, order)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArbitrageOrderRepository_GetByExchangeOrderID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewArbitrageOrderRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "ARBITRAGE_TRANS_ID", "EXCHANGE_ID", "EXCHANGE_ACCOUNT_ID", "EXCHANGE_ORDER_ID",
		"TRADER_ID", "SIDE", "ORDER_TYPE", "PAIR_ID", "REQUESTED_QUANTITY",
		"FILLED_QUANTITY", "AVG_PRICE", "TOTAL_COST", "TOTAL_FEE", "FEE_CURRENCY",
		"STATUS", "ERROR_MESSAGE", "DATE_CREATE",
	}).AddRow(
		1, 100, 2, 123, "exchange-order-123",
		1, "BUY", "LIMIT", 42, 1.5,
		1.5, 50000.0, 75000.0, 7.5, "USDT",
		models.OrderStatusFilled, nil, now,
	)

	mock.ExpectQuery("SELECT .* FROM ARBITRAGE_ORDER WHERE EXCHANGE_ID = .* AND EXCHANGE_ORDER_ID = .*").
		WithArgs(2, "exchange-order-123").
		WillReturnRows(rows)

	order, err := repo.GetByExchangeOrderID(context.Background(), 2, "exchange-order-123")

	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, "exchange-order-123", order.ExchangeOrderID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArbitrageOrderRepository_ListByArbitrageTransID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewArbitrageOrderRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "ARBITRAGE_TRANS_ID", "EXCHANGE_ID", "EXCHANGE_ACCOUNT_ID", "EXCHANGE_ORDER_ID",
		"TRADER_ID", "SIDE", "ORDER_TYPE", "PAIR_ID", "REQUESTED_QUANTITY",
		"FILLED_QUANTITY", "AVG_PRICE", "TOTAL_COST", "TOTAL_FEE", "FEE_CURRENCY",
		"STATUS", "ERROR_MESSAGE", "DATE_CREATE",
	}).
		AddRow(1, 100, 2, 123, "order-1", 1, "BUY", "LIMIT", 42, 1.5, 1.5, 50000.0, 75000.0, 7.5, "USDT", models.OrderStatusFilled, nil, now).
		AddRow(2, 100, 3, 124, "order-2", 1, "SELL", "LIMIT", 42, 1.5, 1.5, 51000.0, 76500.0, 7.65, "USDT", models.OrderStatusFilled, nil, now)

	mock.ExpectQuery("SELECT .* FROM ARBITRAGE_ORDER WHERE ARBITRAGE_TRANS_ID = .* ORDER BY ID").
		WithArgs(int64(100)).
		WillReturnRows(rows)

	orders, err := repo.ListByArbitrageTransID(context.Background(), 100)

	assert.NoError(t, err)
	assert.Len(t, orders, 2)
	assert.Equal(t, int64(100), orders[0].ArbitrageTransID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArbitrageOrderRepository_UpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewArbitrageOrderRepository(sqlxDB)

	filledQty := 1.5
	avgPrice := 50000.0
	totalFee := 7.5
	feeCurrency := "USDT"

	mock.ExpectExec("UPDATE ARBITRAGE_ORDER SET STATUS = .*").
		WithArgs(models.OrderStatusFilled, &filledQty, &avgPrice, &totalFee, &feeCurrency, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateStatus(context.Background(), 1, models.OrderStatusFilled, &filledQty, &avgPrice, &totalFee, &feeCurrency)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestArbitrageOrderRepository_AddFill(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext)")
}

func TestArbitrageOrderRepository_GetFills(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewArbitrageOrderRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "ARBITRAGE_ORDER_ID", "EXCHANGE_TRANSACTION_ID", "QUANTITY", "PRICE", "COST",
		"FEE", "FEE_CURRENCY", "TIMESTAMP",
	}).
		AddRow(1, 100, "txn-1", "0.5", "50000.0", "25000.0", "2.5", "USDT", now).
		AddRow(2, 100, "txn-2", "1.0", "50100.0", "50100.0", "5.0", "USDT", now.Add(time.Second))

	mock.ExpectQuery("SELECT .* FROM ARBITRAGE_ORDER_TRANS WHERE ORDER_ID = .* ORDER BY EXEC_TIMESTAMP").
		WithArgs(int64(100)).
		WillReturnRows(rows)

	fills, err := repo.GetFills(context.Background(), 100)

	assert.NoError(t, err)
	assert.Len(t, fills, 2)
	assert.Equal(t, "0.5", fills[0].Quantity)
	assert.Equal(t, "1.0", fills[1].Quantity)
	assert.NoError(t, mock.ExpectationsWereMet())
}
