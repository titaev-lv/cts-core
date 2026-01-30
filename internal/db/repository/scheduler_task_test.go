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

func TestSchedulerTaskRepository_Create(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext)")
}

func TestSchedulerTaskRepository_GetByID_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TASK_NAME", "TASK_TYPE", "SCHEDULE_CRON", "SCHEDULE_INTERVAL_SEC",
		"ENABLED", "STATUS", "LAST_RUN_AT", "LAST_RUN_DURATION_MS", "LAST_RUN_STATUS",
		"LAST_ERROR", "NEXT_RUN_AT", "RUN_COUNT", "ERROR_COUNT", "CONFIG",
		"DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY",
	}).AddRow(
		1, "cleanup_sessions", models.TaskTypeCleanup, "0 2 * * *", nil,
		true, models.TaskStatusIdle, nil, nil, nil,
		nil, now, 0, 0, `{"retention_days": 30}`,
		now, now, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM SCHEDULER_TASKS WHERE ID").WithArgs(1).WillReturnRows(rows)

	task, err := repo.GetByID(context.Background(), 1)

	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, 1, task.ID)
	assert.Equal(t, "cleanup_sessions", task.TaskName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	mock.ExpectQuery("SELECT .* FROM SCHEDULER_TASKS WHERE ID").
		WithArgs(999).
		WillReturnError(sql.ErrNoRows)

	task, err := repo.GetByID(context.Background(), 999)

	assert.NoError(t, err)
	assert.Nil(t, task)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_GetByName(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TASK_NAME", "TASK_TYPE", "SCHEDULE_CRON", "SCHEDULE_INTERVAL_SEC",
		"ENABLED", "STATUS", "LAST_RUN_AT", "LAST_RUN_DURATION_MS", "LAST_RUN_STATUS",
		"LAST_ERROR", "NEXT_RUN_AT", "RUN_COUNT", "ERROR_COUNT", "CONFIG",
		"DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY",
	}).AddRow(
		1, "cleanup_sessions", models.TaskTypeCleanup, "0 2 * * *", nil,
		true, models.TaskStatusIdle, nil, nil, nil,
		nil, now, 0, 0, `{"retention_days": 30}`,
		now, now, nil, nil,
	)

	mock.ExpectQuery("SELECT .* FROM SCHEDULER_TASKS WHERE TASK_NAME").
		WithArgs("cleanup_sessions").
		WillReturnRows(rows)

	task, err := repo.GetByName(context.Background(), "cleanup_sessions")

	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, "cleanup_sessions", task.TaskName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_List_NoFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TASK_NAME", "TASK_TYPE", "SCHEDULE_CRON", "SCHEDULE_INTERVAL_SEC",
		"ENABLED", "STATUS", "LAST_RUN_AT", "LAST_RUN_DURATION_MS", "LAST_RUN_STATUS",
		"LAST_ERROR", "NEXT_RUN_AT", "RUN_COUNT", "ERROR_COUNT", "CONFIG",
		"DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY",
	}).
		AddRow(1, "task1", models.TaskTypeCleanup, "0 2 * * *", nil, true, models.TaskStatusIdle, nil, nil, nil, nil, now, 0, 0, `{}`, now, now, nil, nil).
		AddRow(2, "task2", models.TaskTypeMonitoring, nil, 60, false, models.TaskStatusDisabled, nil, nil, nil, nil, nil, 0, 0, `{}`, now, now, nil, nil)

	mock.ExpectQuery("SELECT .* FROM SCHEDULER_TASKS ORDER BY ID").WillReturnRows(rows)

	tasks, err := repo.List(context.Background(), nil)

	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_List_EnabledFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	now := time.Now()
	enabled := true
	rows := sqlmock.NewRows([]string{
		"ID", "TASK_NAME", "TASK_TYPE", "SCHEDULE_CRON", "SCHEDULE_INTERVAL_SEC",
		"ENABLED", "STATUS", "LAST_RUN_AT", "LAST_RUN_DURATION_MS", "LAST_RUN_STATUS",
		"LAST_ERROR", "NEXT_RUN_AT", "RUN_COUNT", "ERROR_COUNT", "CONFIG",
		"DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY",
	}).AddRow(1, "task1", models.TaskTypeCleanup, "0 2 * * *", nil, true, models.TaskStatusIdle, nil, nil, nil, nil, now, 0, 0, `{}`, now, now, nil, nil)

	mock.ExpectQuery("SELECT .* FROM SCHEDULER_TASKS WHERE ENABLED = .* ORDER BY ID").
		WithArgs(enabled).
		WillReturnRows(rows)

	tasks, err := repo.List(context.Background(), &enabled)

	assert.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.True(t, tasks[0].Enabled)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_GetDueTasks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TASK_NAME", "TASK_TYPE", "SCHEDULE_CRON", "SCHEDULE_INTERVAL_SEC",
		"ENABLED", "STATUS", "LAST_RUN_AT", "LAST_RUN_DURATION_MS", "LAST_RUN_STATUS",
		"LAST_ERROR", "NEXT_RUN_AT", "RUN_COUNT", "ERROR_COUNT", "CONFIG",
		"DATE_CREATE", "DATE_MODIFY", "USER_CREATED", "USER_MODIFY",
	}).AddRow(1, "task1", models.TaskTypeCleanup, "0 2 * * *", nil, true, models.TaskStatusIdle, nil, nil, nil, nil, now.Add(-time.Hour), 0, 0, `{}`, now, now, nil, nil)

	mock.ExpectQuery("SELECT .* FROM SCHEDULER_TASKS WHERE ENABLED = TRUE AND STATUS != .* AND \\(NEXT_RUN_AT IS NULL OR NEXT_RUN_AT <= .*\\)").
		WithArgs(models.TaskStatusDisabled, sqlmock.AnyArg()).
		WillReturnRows(rows)

	tasks, err := repo.GetDueTasks(context.Background())

	assert.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_Update(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext)")
}

func TestSchedulerTaskRepository_UpdateStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	mock.ExpectExec("UPDATE SCHEDULER_TASKS SET STATUS").
		WithArgs(models.TaskStatusRunning, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateStatus(context.Background(), 1, models.TaskStatusRunning)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_RecordExecution(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (complex UPDATE with CASE)")
}

func TestSchedulerTaskRepository_UpdateNextRun(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	mock.ExpectExec("UPDATE SCHEDULER_TASKS SET NEXT_RUN_AT").
		WithArgs(sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateNextRun(context.Background(), 1)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSchedulerTaskRepository_Delete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewSchedulerTaskRepository(sqlxDB)

	mock.ExpectExec("DELETE FROM SCHEDULER_TASKS WHERE ID").
		WithArgs(1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Delete(context.Background(), 1)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
