package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// reencryptionJobRepository implements ReencryptionJobRepository interface
type reencryptionJobRepository struct {
	db sqlx.ExtContext
}

// NewReencryptionJobRepository creates a new re-encryption job repository
func NewReencryptionJobRepository(db *sqlx.DB) ReencryptionJobRepository {
	return &reencryptionJobRepository{db: db}
}

// NewReencryptionJobRepositoryWithTx creates a re-encryption job repository with transaction context
func NewReencryptionJobRepositoryWithTx(tx *sqlx.Tx) ReencryptionJobRepository {
	return &reencryptionJobRepository{db: tx}
}

// Create inserts a new job
func (r *reencryptionJobRepository) Create(ctx context.Context, job *models.ReencryptionJob) error {
	query := `
		INSERT INTO REENCRYPTION_JOBS (
			JOB_TYPE, OLD_KEY_VERSION, NEW_KEY_VERSION, CONTEXT,
			STATUS, TOTAL_RECORDS, PROCESSED_RECORDS, FAILED_RECORDS,
			BATCH_SIZE, STARTED_AT, COMPLETED_AT, LAST_PROCESSED_AT,
			LAST_ERROR, DATE_CREATE, USER_CREATED
		) VALUES (
			:JOB_TYPE, :OLD_KEY_VERSION, :NEW_KEY_VERSION, :CONTEXT,
			:STATUS, :TOTAL_RECORDS, :PROCESSED_RECORDS, :FAILED_RECORDS,
			:BATCH_SIZE, :STARTED_AT, :COMPLETED_AT, :LAST_PROCESSED_AT,
			:LAST_ERROR, :DATE_CREATE, :USER_CREATED
		)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, job)
	if err != nil {
		return fmt.Errorf("failed to create reencryption job: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	job.ID = int(id)
	return nil
}

// GetByID retrieves a job by ID
func (r *reencryptionJobRepository) GetByID(ctx context.Context, id int) (*models.ReencryptionJob, error) {
	query := `SELECT * FROM REENCRYPTION_JOBS WHERE ID = ?`

	var job models.ReencryptionJob
	err := sqlx.GetContext(ctx, r.db, &job, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reencryption job: %w", err)
	}

	return &job, nil
}

// ListPending retrieves all pending jobs
func (r *reencryptionJobRepository) ListPending(ctx context.Context) ([]*models.ReencryptionJob, error) {
	query := `
		SELECT * FROM REENCRYPTION_JOBS
		WHERE STATUS IN ('pending', 'in_progress')
		ORDER BY ID
	`

	var jobs []*models.ReencryptionJob
	err := sqlx.SelectContext(ctx, r.db, &jobs, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending jobs: %w", err)
	}

	return jobs, nil
}

// UpdateStatus updates job status
func (r *reencryptionJobRepository) UpdateStatus(ctx context.Context, id int, status models.ReencryptionJobStatus) error {
	query := `
		UPDATE REENCRYPTION_JOBS
		SET STATUS = ?,
		    STARTED_AT = CASE WHEN ? = 'in_progress' AND STARTED_AT IS NULL THEN NOW() ELSE STARTED_AT END
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, status, id)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("reencryption job not found: %d", id)
	}

	return nil
}

// UpdateProgress updates job progress counters
func (r *reencryptionJobRepository) UpdateProgress(ctx context.Context, id int, processed, failed int) error {
	query := `
		UPDATE REENCRYPTION_JOBS
		SET PROCESSED_RECORDS = PROCESSED_RECORDS + ?,
		    FAILED_RECORDS = FAILED_RECORDS + ?,
		    LAST_PROCESSED_AT = NOW()
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, processed, failed, id)
	if err != nil {
		return fmt.Errorf("failed to update job progress: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("reencryption job not found: %d", id)
	}

	return nil
}

// Complete marks job as completed
func (r *reencryptionJobRepository) Complete(ctx context.Context, id int) error {
	query := `
		UPDATE REENCRYPTION_JOBS
		SET STATUS = 'completed',
		    COMPLETED_AT = NOW()
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("reencryption job not found: %d", id)
	}

	return nil
}

// Fail marks job as failed with error message
func (r *reencryptionJobRepository) Fail(ctx context.Context, id int, errorMsg string) error {
	query := `
		UPDATE REENCRYPTION_JOBS
		SET STATUS = 'failed',
		    LAST_ERROR = ?,
		    COMPLETED_AT = NOW()
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, errorMsg, id)
	if err != nil {
		return fmt.Errorf("failed to mark job as failed: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("reencryption job not found: %d", id)
	}

	return nil
}
