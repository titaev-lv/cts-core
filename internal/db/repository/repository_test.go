package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := New(sqlxDB)

	assert.NotNil(t, repo)
	assert.NotNil(t, repo.Trader())
	assert.NotNil(t, repo.TraderSession())
	assert.NotNil(t, repo.TraderResource())
	assert.NotNil(t, repo.ArbitrageOrder())
	assert.NotNil(t, repo.AuditLog())
	assert.NotNil(t, repo.ReencryptionJob())
	assert.NotNil(t, repo.ReencryptionProgress())
	assert.NotNil(t, repo.SchedulerTask())
}

func TestRepository_BeginTx(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := New(sqlxDB)

	mock.ExpectBegin()
	mock.ExpectRollback()

	tx, err := repo.BeginTx(context.Background(), nil)
	assert.NoError(t, err)
	assert.NotNil(t, tx)

	// Verify transaction has access to all repositories
	assert.NotNil(t, tx.Trader())
	assert.NotNil(t, tx.TraderSession())
	assert.NotNil(t, tx.TraderResource())
	assert.NotNil(t, tx.ArbitrageOrder())
	assert.NotNil(t, tx.AuditLog())
	assert.NotNil(t, tx.ReencryptionJob())
	assert.NotNil(t, tx.ReencryptionProgress())
	assert.NotNil(t, tx.SchedulerTask())

	err = tx.Rollback()
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_WithTx_Commit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := New(sqlxDB)

	mock.ExpectBegin()
	mock.ExpectCommit()

	var executed bool
	err = repo.WithTx(context.Background(), func(tx Tx) error {
		executed = true
		assert.NotNil(t, tx.Trader())
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, executed)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_WithTx_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := New(sqlxDB)

	mock.ExpectBegin()
	mock.ExpectRollback()

	testErr := sql.ErrNoRows
	err = repo.WithTx(context.Background(), func(tx Tx) error {
		return testErr
	})

	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRepository_WithTx_Panic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := New(sqlxDB)

	mock.ExpectBegin()
	mock.ExpectRollback()

	assert.Panics(t, func() {
		_ = repo.WithTx(context.Background(), func(tx Tx) error {
			panic("test panic")
		})
	})

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTransaction_Commit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	mock.ExpectBegin()
	mock.ExpectCommit()

	sqlxTx, err := sqlxDB.Beginx()
	require.NoError(t, err)

	tx := &transaction{tx: sqlxTx}
	err = tx.Commit()
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTransaction_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	mock.ExpectBegin()
	mock.ExpectRollback()

	sqlxTx, err := sqlxDB.Beginx()
	require.NoError(t, err)

	tx := &transaction{tx: sqlxTx}
	err = tx.Rollback()
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
