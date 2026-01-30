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

func TestReencryptionProgressRepository_CreateBatch(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (bulk INSERT with dynamic placeholders)")
}

func TestReencryptionProgressRepository_GetPendingBatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionProgressRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "JOB_ID", "TABLE_NAME", "RECORD_ID", "STATUS",
		"ATTEMPT_COUNT", "ERROR_MESSAGE", "PROCESSED_AT", "DATE_CREATE",
	}).
		AddRow(1, 1, "TRADER", 100, models.ReencryptionProgressStatusPending, 0, nil, nil, now).
		AddRow(2, 1, "TRADER", 101, models.ReencryptionProgressStatusPending, 0, nil, nil, now).
		AddRow(3, 1, "TRADER", 102, models.ReencryptionProgressStatusPending, 1, "temp error", nil, now)

	mock.ExpectQuery("SELECT .* FROM REENCRYPTION_PROGRESS WHERE JOB_ID = .* AND STATUS = 'pending' AND ATTEMPT_COUNT < 3").
		WithArgs(1, 50).
		WillReturnRows(rows)

	records, err := repo.GetPendingBatch(context.Background(), 1, 50)

	assert.NoError(t, err)
	assert.Len(t, records, 3)
	assert.Equal(t, models.ReencryptionProgressStatusPending, records[0].Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionProgressRepository_UpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionProgressRepository(sqlxDB)

	mock.ExpectExec("UPDATE REENCRYPTION_PROGRESS SET STATUS = .*").
		WithArgs(models.ReencryptionProgressStatusCompleted, nil, models.ReencryptionProgressStatusCompleted, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateStatus(context.Background(), 1, models.ReencryptionProgressStatusCompleted, nil)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionProgressRepository_UpdateStatus_WithError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionProgressRepository(sqlxDB)

	errorMsg := "decryption failed"
	mock.ExpectExec("UPDATE REENCRYPTION_PROGRESS SET STATUS = .*").
		WithArgs(models.ReencryptionProgressStatusFailed, &errorMsg, models.ReencryptionProgressStatusFailed, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateStatus(context.Background(), 1, models.ReencryptionProgressStatusFailed, &errorMsg)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionProgressRepository_IncrementAttempts(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionProgressRepository(sqlxDB)

	mock.ExpectExec("UPDATE REENCRYPTION_PROGRESS SET ATTEMPT_COUNT = ATTEMPT_COUNT \\+ 1").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.IncrementAttempts(context.Background(), 1)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionProgressRepository_GetFailedRecords(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionProgressRepository(sqlxDB)

	now := time.Now()
	errorMsg := "temporary HSM error"
	rows := sqlmock.NewRows([]string{
		"ID", "JOB_ID", "TABLE_NAME", "RECORD_ID", "STATUS",
		"ATTEMPT_COUNT", "ERROR_MESSAGE", "PROCESSED_AT", "DATE_CREATE",
	}).
		AddRow(1, 1, "TRADER", 100, models.ReencryptionProgressStatusFailed, 1, errorMsg, nil, now).
		AddRow(2, 1, "TRADER", 101, models.ReencryptionProgressStatusFailed, 2, errorMsg, nil, now)

	mock.ExpectQuery("SELECT .* FROM REENCRYPTION_PROGRESS WHERE JOB_ID = .* AND STATUS = 'failed' AND ATTEMPT_COUNT < .*").
		WithArgs(1, 3).
		WillReturnRows(rows)

	records, err := repo.GetFailedRecords(context.Background(), 1, 3)

	assert.NoError(t, err)
	assert.Len(t, records, 2)
	assert.Equal(t, models.ReencryptionProgressStatusFailed, records[0].Status)
	assert.Equal(t, 1, records[0].AttemptCount)
	assert.Equal(t, 2, records[1].AttemptCount)
	assert.NoError(t, mock.ExpectationsWereMet())
}
