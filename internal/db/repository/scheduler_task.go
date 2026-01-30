package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// schedulerTaskRepository implements SchedulerTaskRepository interface
type schedulerTaskRepository struct {
	db sqlx.ExtContext
}

// NewSchedulerTaskRepository creates a new scheduler task repository
func NewSchedulerTaskRepository(db *sqlx.DB) SchedulerTaskRepository {
	return &schedulerTaskRepository{db: db}
}

// NewSchedulerTaskRepositoryWithTx creates a scheduler task repository with transaction context
func NewSchedulerTaskRepositoryWithTx(tx *sqlx.Tx) SchedulerTaskRepository {
	return &schedulerTaskRepository{db: tx}
}

// Create inserts a new task
func (r *schedulerTaskRepository) Create(ctx context.Context, task *models.SchedulerTask) error {
	query := `
		INSERT INTO SCHEDULER_TASKS (
			TASK_NAME, TASK_TYPE, SCHEDULE_CRON, SCHEDULE_INTERVAL_SEC,
			ENABLED, STATUS, CONFIG, DATE_CREATE, DATE_MODIFY,
			USER_CREATED, USER_MODIFY
		) VALUES (
			:task_name, :task_type, :schedule_cron, :schedule_interval_sec,
			:enabled, :status, :config, :date_create, :date_modify,
			:user_created, :user_modify
		)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, task)
	if err != nil {
		return fmt.Errorf("failed to create scheduler task: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	task.ID = int(id)
	return nil
}

// GetByID retrieves a task by ID
func (r *schedulerTaskRepository) GetByID(ctx context.Context, id int) (*models.SchedulerTask, error) {
	query := `
		SELECT ID, TASK_NAME, TASK_TYPE, SCHEDULE_CRON, SCHEDULE_INTERVAL_SEC,
		       ENABLED, STATUS, LAST_RUN_AT, LAST_RUN_DURATION_MS, LAST_RUN_STATUS,
		       LAST_ERROR, NEXT_RUN_AT, RUN_COUNT, ERROR_COUNT, CONFIG,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY
		FROM SCHEDULER_TASKS
		WHERE ID = ?
	`

	var task models.SchedulerTask
	err := sqlx.GetContext(ctx, r.db, &task, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduler task by id: %w", err)
	}

	return &task, nil
}

// GetByName retrieves a task by unique name
func (r *schedulerTaskRepository) GetByName(ctx context.Context, name string) (*models.SchedulerTask, error) {
	query := `
		SELECT ID, TASK_NAME, TASK_TYPE, SCHEDULE_CRON, SCHEDULE_INTERVAL_SEC,
		       ENABLED, STATUS, LAST_RUN_AT, LAST_RUN_DURATION_MS, LAST_RUN_STATUS,
		       LAST_ERROR, NEXT_RUN_AT, RUN_COUNT, ERROR_COUNT, CONFIG,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY
		FROM SCHEDULER_TASKS
		WHERE TASK_NAME = ?
	`

	var task models.SchedulerTask
	err := sqlx.GetContext(ctx, r.db, &task, query, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduler task by name: %w", err)
	}

	return &task, nil
}

// List retrieves all tasks with optional enabled filter
func (r *schedulerTaskRepository) List(ctx context.Context, enabled *bool) ([]*models.SchedulerTask, error) {
	query := `
		SELECT ID, TASK_NAME, TASK_TYPE, SCHEDULE_CRON, SCHEDULE_INTERVAL_SEC,
		       ENABLED, STATUS, LAST_RUN_AT, LAST_RUN_DURATION_MS, LAST_RUN_STATUS,
		       LAST_ERROR, NEXT_RUN_AT, RUN_COUNT, ERROR_COUNT, CONFIG,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY
		FROM SCHEDULER_TASKS
	`
	args := []interface{}{}

	if enabled != nil {
		query += " WHERE ENABLED = ?"
		args = append(args, *enabled)
	}

	query += " ORDER BY ID"

	var tasks []*models.SchedulerTask
	err := sqlx.SelectContext(ctx, r.db, &tasks, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list scheduler tasks: %w", err)
	}

	return tasks, nil
}

// GetDueTasks retrieves tasks that should run now
func (r *schedulerTaskRepository) GetDueTasks(ctx context.Context) ([]*models.SchedulerTask, error) {
	query := `
		SELECT ID, TASK_NAME, TASK_TYPE, SCHEDULE_CRON, SCHEDULE_INTERVAL_SEC,
		       ENABLED, STATUS, LAST_RUN_AT, LAST_RUN_DURATION_MS, LAST_RUN_STATUS,
		       LAST_ERROR, NEXT_RUN_AT, RUN_COUNT, ERROR_COUNT, CONFIG,
		       DATE_CREATE, DATE_MODIFY, USER_CREATED, USER_MODIFY
		FROM SCHEDULER_TASKS
		WHERE ENABLED = TRUE
		  AND STATUS != ?
		  AND (NEXT_RUN_AT IS NULL OR NEXT_RUN_AT <= ?)
		ORDER BY NEXT_RUN_AT
	`

	var tasks []*models.SchedulerTask
	err := sqlx.SelectContext(ctx, r.db, &tasks, query, models.TaskStatusDisabled, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get due tasks: %w", err)
	}

	return tasks, nil
}

// Update updates task configuration
func (r *schedulerTaskRepository) Update(ctx context.Context, task *models.SchedulerTask) error {
	task.DateModify = time.Now()

	query := `
		UPDATE SCHEDULER_TASKS
		SET TASK_TYPE = :task_type,
		    SCHEDULE_CRON = :schedule_cron,
		    SCHEDULE_INTERVAL_SEC = :schedule_interval_sec,
		    ENABLED = :enabled,
		    CONFIG = :config,
		    DATE_MODIFY = :date_modify,
		    USER_MODIFY = :user_modify
		WHERE ID = :id
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, task)
	if err != nil {
		return fmt.Errorf("failed to update scheduler task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scheduler task not found: id=%d", task.ID)
	}

	return nil
}

// UpdateStatus updates task status
func (r *schedulerTaskRepository) UpdateStatus(ctx context.Context, id int, status models.TaskStatus) error {
	query := `
		UPDATE SCHEDULER_TASKS
		SET STATUS = ?,
		    DATE_MODIFY = ?
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update scheduler task status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scheduler task not found: id=%d", id)
	}

	return nil
}

// RecordExecution records task execution result
func (r *schedulerTaskRepository) RecordExecution(ctx context.Context, id int, durationMS int32, status models.TaskRunStatus, errorMsg *string) error {
	query := `
		UPDATE SCHEDULER_TASKS
		SET LAST_RUN_AT = ?,
		    LAST_RUN_DURATION_MS = ?,
		    LAST_RUN_STATUS = ?,
		    LAST_ERROR = ?,
		    RUN_COUNT = RUN_COUNT + 1,
		    ERROR_COUNT = ERROR_COUNT + CASE WHEN ? = ? THEN 1 ELSE 0 END,
		    STATUS = CASE WHEN ? = ? THEN ? ELSE ? END,
		    DATE_MODIFY = ?
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		time.Now(),
		durationMS,
		status,
		errorMsg,
		status, models.TaskRunStatusError,
		status, models.TaskRunStatusError, models.TaskStatusFailed, models.TaskStatusIdle,
		time.Now(),
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to record scheduler task execution: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scheduler task not found: id=%d", id)
	}

	return nil
}

// UpdateNextRun calculates and updates next run time
func (r *schedulerTaskRepository) UpdateNextRun(ctx context.Context, id int) error {
	// TODO: Implement cron/interval calculation
	// For now, just clear NEXT_RUN_AT
	query := `
		UPDATE SCHEDULER_TASKS
		SET NEXT_RUN_AT = NULL,
		    DATE_MODIFY = ?
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update next run: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scheduler task not found: id=%d", id)
	}

	return nil
}

// Delete removes a task
func (r *schedulerTaskRepository) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM SCHEDULER_TASKS WHERE ID = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete scheduler task: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("scheduler task not found: id=%d", id)
	}

	return nil
}
