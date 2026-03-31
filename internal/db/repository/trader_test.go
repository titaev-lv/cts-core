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

func TestTraderRepository_Create(t *testing.T) {
	// Note: This test requires integration with real DB due to sqlx.NamedExecContext
	// Skip for unit tests - use sqlmock.QueryMatcherEqual or integration tests
	t.Skip("Requires integration test with real MySQL")
}

func TestTraderRepository_GetByID_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_NAME", "CERTIFICATE_CN", "REGION", "STATUS",
		"MAX_TASKS", "DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY", "NOTES",
	}).AddRow(1, "Test Trader", "test.cts.internal", sql.NullString{}, "active",
		10, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{})

	mock.ExpectQuery("SELECT .* FROM TRADER WHERE ID").WithArgs(1).WillReturnRows(rows)

	trader, err := repo.GetByID(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, trader)
	assert.Equal(t, 1, trader.ID)
	assert.Equal(t, "Test Trader", trader.TraderName)
}

func TestTraderRepository_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	mock.ExpectQuery("SELECT .* FROM TRADER WHERE ID").
		WithArgs(999).
		WillReturnError(sql.ErrNoRows)

	trader, err := repo.GetByID(context.Background(), 999)
	assert.NoError(t, err)
	assert.Nil(t, trader)
}

func TestTraderRepository_GetByCertificateCN(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	cn := "prod.cts.internal"
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_NAME", "CERTIFICATE_CN", "REGION", "STATUS",
		"MAX_TASKS", "DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY", "NOTES",
	}).AddRow(5, "Prod Trader", cn, sql.NullString{Valid: true, String: "eu"}, "active",
		20, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{})

	mock.ExpectQuery("SELECT .* FROM TRADER WHERE CERTIFICATE_CN").
		WithArgs(cn).
		WillReturnRows(rows)

	trader, err := repo.GetByCertificateCN(context.Background(), cn)
	assert.NoError(t, err)
	assert.NotNil(t, trader)
	assert.Equal(t, cn, trader.CertificateCN)
}

func TestTraderRepository_GetOrCreateByCertificateCN(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	cn := "trader-auto.cts.internal"

	mock.ExpectExec("INSERT INTO TRADER").
		WithArgs(cn, cn, "unknown", models.TraderStatusPending, 0, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(11, 1))

	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_NAME", "CERTIFICATE_CN", "REGION", "STATUS",
		"MAX_TASKS", "DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY", "NOTES",
	}).AddRow(11, cn, cn, sql.NullString{Valid: true, String: "unknown"}, "pending",
		0, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{})

	mock.ExpectQuery("SELECT .* FROM TRADER WHERE ID").
		WithArgs(11).
		WillReturnRows(rows)

	trader, created, err := repo.GetOrCreateByCertificateCN(context.Background(), cn)
	assert.NoError(t, err)
	assert.NotNil(t, trader)
	assert.True(t, created)
	assert.Equal(t, 11, trader.ID)
	assert.Equal(t, cn, trader.CertificateCN)
	assert.Equal(t, models.TraderStatusPending, trader.Status)
}

func TestTraderRepository_GetOrCreateByCertificateCN_Existing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	cn := "trader-existing.cts.internal"

	mock.ExpectExec("INSERT INTO TRADER").
		WithArgs(cn, cn, "unknown", models.TraderStatusPending, 0, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(15, 0))

	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_NAME", "CERTIFICATE_CN", "REGION", "STATUS",
		"MAX_TASKS", "DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY", "NOTES",
	}).AddRow(15, cn, cn, sql.NullString{Valid: true, String: "unknown"}, "pending",
		0, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{})

	mock.ExpectQuery("SELECT .* FROM TRADER WHERE ID").
		WithArgs(15).
		WillReturnRows(rows)

	trader, created, err := repo.GetOrCreateByCertificateCN(context.Background(), cn)
	assert.NoError(t, err)
	assert.NotNil(t, trader)
	assert.False(t, created)
	assert.Equal(t, 15, trader.ID)
	assert.Equal(t, cn, trader.CertificateCN)
}

func TestTraderRepository_List_NoFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_NAME", "CERTIFICATE_CN", "REGION", "STATUS",
		"MAX_TASKS", "DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY", "NOTES",
	}).
		AddRow(1, "Trader 1", "t1.cts.internal", sql.NullString{}, "active", 10, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{}).
		AddRow(2, "Trader 2", "t2.cts.internal", sql.NullString{}, "active", 15, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{})

	mock.ExpectQuery("SELECT .* FROM TRADER ORDER BY ID").WillReturnRows(rows)

	traders, err := repo.List(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, traders, 2)
}

func TestTraderRepository_List_WithStatusFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	status := models.TraderStatusActive
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_NAME", "CERTIFICATE_CN", "REGION", "STATUS",
		"MAX_TASKS", "DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY", "NOTES",
	}).AddRow(1, "Active Trader", "active.cts.internal", sql.NullString{}, "active",
		10, time.Now(), time.Now(), sql.NullInt32{}, sql.NullInt32{}, sql.NullString{})

	mock.ExpectQuery("SELECT .* FROM TRADER WHERE STATUS").
		WithArgs(status).
		WillReturnRows(rows)

	traders, err := repo.List(context.Background(), &status)
	assert.NoError(t, err)
	assert.Len(t, traders, 1)
	assert.Equal(t, models.TraderStatusActive, traders[0].Status)
}

func TestTraderRepository_Update(t *testing.T) {
	// Note: This test requires integration with real DB due to sqlx.NamedExecContext
	// Skip for unit tests
	t.Skip("Requires integration test with real MySQL")
}

func TestTraderRepository_UpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER SET STATUS").
		WithArgs(models.TraderStatusSuspended, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateStatus(context.Background(), 1, models.TraderStatusSuspended)
	assert.NoError(t, err)
}

func TestTraderRepository_UpdateRelease(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER").
		WithArgs("2.0.2", sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateRelease(context.Background(), 1, "2.0.2")
	assert.NoError(t, err)
}

func TestTraderRepository_Delete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER SET STATUS").
		WithArgs(models.TraderStatusDecommissioned, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Delete(context.Background(), 1)
	assert.NoError(t, err)
}
