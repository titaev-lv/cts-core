package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExchangeRequirementsRepository_ListTradeExchanges(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewExchangeRequirementsRepository(sqlxDB)

	rows := sqlmock.NewRows([]string{"exchange_id", "exchange_name"}).
		AddRow(2, "kucoin").
		AddRow(5, "binance")

	mock.ExpectQuery("SELECT DISTINCT[\\s\\S]*FROM[\\s\\S]*TRADE t[\\s\\S]*ORDER BY[\\s\\S]*exchange_name").
		WillReturnRows(rows)

	items, err := repo.ListTradeExchanges(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 2, items[0].ExchangeID)
	assert.Equal(t, "kucoin", items[0].ExchangeName)
	assert.Equal(t, 5, items[1].ExchangeID)
	assert.Equal(t, "binance", items[1].ExchangeName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestExchangeRequirementsRepository_ListMonitorExchanges(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewExchangeRequirementsRepository(sqlxDB)

	rows := sqlmock.NewRows([]string{"exchange_id", "exchange_name"}).
		AddRow(2, "kucoin")

	mock.ExpectQuery("SELECT DISTINCT[\\s\\S]*FROM[\\s\\S]*MONITORING m[\\s\\S]*ORDER BY[\\s\\S]*exchange_name").
		WillReturnRows(rows)

	items, err := repo.ListMonitorExchanges(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 2, items[0].ExchangeID)
	assert.Equal(t, "kucoin", items[0].ExchangeName)
	assert.NoError(t, mock.ExpectationsWereMet())
}
