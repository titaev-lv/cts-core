package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

func TestTraderResourceRepository_Upsert(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext with ON DUPLICATE KEY)")
}

func TestTraderResourceRepository_GetByTraderAndExchange_NoAccountID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderResourceRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "EXCHANGE_ID", "EXCHANGE_ACCOUNT_ID", "RESOURCE_TYPE",
		"USED_VALUE", "MAX_VALUE", "LAST_UPDATED", "RESET_AT",
	}).AddRow(1, 1, 2, nil, models.ResourceTypeAPIRequestsMinute, "10", "100", now, now.Add(time.Hour))

	mock.ExpectQuery("SELECT .* FROM TRADER_EXCHANGE_RESOURCE WHERE TRADER_ID = .* AND EXCHANGE_ID = .* AND EXCHANGE_ACCOUNT_ID IS NULL").
		WithArgs(1, 2).
		WillReturnRows(rows)

	resources, err := repo.GetByTraderAndExchange(context.Background(), 1, 2, nil)

	assert.NoError(t, err)
	assert.Len(t, resources, 1)
	assert.Equal(t, int64(1), resources[0].ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderResourceRepository_GetByTraderAndExchange_WithAccountID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderResourceRepository(sqlxDB)

	now := time.Now()
	accountID := int32(123)
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "EXCHANGE_ID", "EXCHANGE_ACCOUNT_ID", "RESOURCE_TYPE",
		"USED_VALUE", "MAX_VALUE", "LAST_UPDATED", "RESET_AT",
	}).AddRow(1, 1, 2, 123, models.ResourceTypeAPIRequestsMinute, "10", "100", now, now.Add(time.Hour))

	mock.ExpectQuery("SELECT .* FROM TRADER_EXCHANGE_RESOURCE WHERE TRADER_ID = .* AND EXCHANGE_ID = .* AND EXCHANGE_ACCOUNT_ID = .*").
		WithArgs(1, 2, accountID).
		WillReturnRows(rows)

	resources, err := repo.GetByTraderAndExchange(context.Background(), 1, 2, &accountID)

	assert.NoError(t, err)
	assert.Len(t, resources, 1)
	assert.True(t, resources[0].ExchangeAccountID.Valid)
	assert.Equal(t, int32(123), resources[0].ExchangeAccountID.Int32)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderResourceRepository_IncrementUsage(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderResourceRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER_EXCHANGE_RESOURCE SET USED_VALUE = USED_VALUE \\+ .*").
		WithArgs(5, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.IncrementUsage(context.Background(), 1, 5)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderResourceRepository_ResetUsage(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderResourceRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER_EXCHANGE_RESOURCE SET USED_VALUE = 0").
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.ResetUsage(context.Background(), 1)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderResourceRepository_GetAvailable(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderResourceRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "EXCHANGE_ID", "EXCHANGE_ACCOUNT_ID", "RESOURCE_TYPE",
		"USED_VALUE", "MAX_VALUE", "LAST_UPDATED", "RESET_AT",
	}).
		AddRow(1, 1, 2, nil, models.ResourceTypeAPIRequestsMinute, "10", "100", now, now.Add(time.Hour)).
		AddRow(2, 2, 2, nil, models.ResourceTypeAPIRequestsMinute, "20", "100", now, now.Add(time.Hour))

	mock.ExpectQuery("SELECT .* FROM TRADER_EXCHANGE_RESOURCE WHERE EXCHANGE_ID = .* AND RESOURCE_TYPE = .* AND \\(MAX_VALUE - USED_VALUE\\) >= .* AND RESET_AT > NOW\\(\\)").
		WithArgs(2, models.ResourceTypeAPIRequestsMinute, 50).
		WillReturnRows(rows)

	resources, err := repo.GetAvailable(context.Background(), 2, models.ResourceTypeAPIRequestsMinute, 50)

	assert.NoError(t, err)
	assert.Len(t, resources, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}
