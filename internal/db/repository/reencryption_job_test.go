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

func TestReencryptionJobRepository_Create(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext)")
}

func TestReencryptionJobRepository_GetByID_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "JOB_TYPE", "OLD_KEY_VERSION", "NEW_KEY_VERSION", "CONTEXT",
		"STATUS", "TOTAL_RECORDS", "PROCESSED_RECORDS", "FAILED_RECORDS",
		"BATCH_SIZE", "STARTED_AT", "COMPLETED_AT", "LAST_PROCESSED_AT",
		"ERROR_MESSAGE", "DATE_CREATE",
	}).AddRow(
		1, "full_rotation", 1, 2, "all_api_keys",
		models.ReencryptionJobStatusPending, 1000, 0, 0,
		100, nil, nil, nil,
		nil, now,
	)

	mock.ExpectQuery("SELECT .* FROM REENCRYPTION_JOBS WHERE ID").WithArgs(1).WillReturnRows(rows)

	job, err := repo.GetByID(context.Background(), 1)

	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, 1, job.ID)
	assert.Equal(t, models.ReencryptionJobStatusPending, job.Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionJobRepository_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	mock.ExpectQuery("SELECT .* FROM REENCRYPTION_JOBS WHERE ID").
		WithArgs(999).
		WillReturnError(sql.ErrNoRows)

	job, err := repo.GetByID(context.Background(), 999)

	assert.NoError(t, err)
	assert.Nil(t, job)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionJobRepository_ListPending(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "JOB_TYPE", "OLD_KEY_VERSION", "NEW_KEY_VERSION", "CONTEXT",
		"STATUS", "TOTAL_RECORDS", "PROCESSED_RECORDS", "FAILED_RECORDS",
		"BATCH_SIZE", "STARTED_AT", "COMPLETED_AT", "LAST_PROCESSED_AT",
		"ERROR_MESSAGE", "DATE_CREATE",
	}).
		AddRow(1, "full_rotation", 1, 2, "all", models.ReencryptionJobStatusPending, 1000, 0, 0, 100, nil, nil, nil, nil, now).
		AddRow(2, "partial_rotation", 1, 2, "api_keys", models.ReencryptionJobStatusInProgress, 500, 250, 0, 50, now, nil, now, nil, now)

	mock.ExpectQuery("SELECT .* FROM REENCRYPTION_JOBS WHERE STATUS IN \\('pending', 'in_progress'\\)").
		WillReturnRows(rows)

	jobs, err := repo.ListPending(context.Background())

	assert.NoError(t, err)
	assert.Len(t, jobs, 2)
	assert.Equal(t, models.ReencryptionJobStatusPending, jobs[0].Status)
	assert.Equal(t, models.ReencryptionJobStatusInProgress, jobs[1].Status)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionJobRepository_UpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	mock.ExpectExec("UPDATE REENCRYPTION_JOBS SET STATUS = .*").
		WithArgs(models.ReencryptionJobStatusInProgress, models.ReencryptionJobStatusInProgress, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateStatus(context.Background(), 1, models.ReencryptionJobStatusInProgress)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionJobRepository_UpdateProgress(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	mock.ExpectExec("UPDATE REENCRYPTION_JOBS SET PROCESSED_RECORDS = PROCESSED_RECORDS \\+ .*").
		WithArgs(50, 2, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateProgress(context.Background(), 1, 50, 2)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionJobRepository_Complete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	mock.ExpectExec("UPDATE REENCRYPTION_JOBS SET STATUS = 'completed'").
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Complete(context.Background(), 1)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReencryptionJobRepository_Fail(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewReencryptionJobRepository(sqlxDB)

	errorMsg := "HSM connection timeout"
	mock.ExpectExec("UPDATE REENCRYPTION_JOBS SET STATUS = 'failed'").
		WithArgs(errorMsg, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Fail(context.Background(), 1, errorMsg)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
