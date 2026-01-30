package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// reencryptionProgressRepository implements ReencryptionProgressRepository interface
type reencryptionProgressRepository struct {
	db sqlx.ExtContext
}

// NewReencryptionProgressRepository creates a new re-encryption progress repository
func NewReencryptionProgressRepository(db *sqlx.DB) ReencryptionProgressRepository {
	return &reencryptionProgressRepository{db: db}
}

// NewReencryptionProgressRepositoryWithTx creates a re-encryption progress repository with transaction context
func NewReencryptionProgressRepositoryWithTx(tx *sqlx.Tx) ReencryptionProgressRepository {
	return &reencryptionProgressRepository{db: tx}
}

// CreateBatch inserts progress records in batch
func (r *reencryptionProgressRepository) CreateBatch(ctx context.Context, records []*models.ReencryptionProgress) error {
	if len(records) == 0 {
		return nil
	}

	// Build bulk insert query
	query := `
		INSERT INTO REENCRYPTION_PROGRESS (
			JOB_ID, TABLE_NAME, RECORD_ID, STATUS,
			ATTEMPT_COUNT, ERROR_MESSAGE, PROCESSED_AT, DATE_CREATE
		) VALUES
	`

	valuePlaceholders := make([]string, len(records))
	args := make([]interface{}, 0, len(records)*8)

	for i, rec := range records {
		valuePlaceholders[i] = "(?, ?, ?, ?, ?, ?, ?, ?)"
		args = append(args,
			rec.JobID,
			rec.TableName,
			rec.RecordID,
			rec.Status,
			rec.AttemptCount,
			rec.ErrorMessage,
			rec.ProcessedAt,
			rec.DateCreate,
		)
	}

	query += strings.Join(valuePlaceholders, ", ")

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to create batch progress records: %w", err)
	}

	return nil
}

// GetPendingBatch retrieves pending records for processing
func (r *reencryptionProgressRepository) GetPendingBatch(ctx context.Context, jobID, batchSize int) ([]*models.ReencryptionProgress, error) {
	query := `
		SELECT * FROM REENCRYPTION_PROGRESS
		WHERE JOB_ID = ?
		  AND STATUS = 'pending'
		  AND ATTEMPT_COUNT < 3
		LIMIT ?
	`

	var records []*models.ReencryptionProgress
	err := sqlx.SelectContext(ctx, r.db, &records, query, jobID, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending batch: %w", err)
	}

	return records, nil
}

// UpdateStatus updates record status
func (r *reencryptionProgressRepository) UpdateStatus(ctx context.Context, id int64, status models.ReencryptionProgressStatus, errorMsg *string) error {
	query := `
		UPDATE REENCRYPTION_PROGRESS
		SET STATUS = ?,
		    ERROR_MESSAGE = ?,
		    PROCESSED_AT = CASE WHEN ? = 'completed' THEN NOW() ELSE PROCESSED_AT END
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, status, errorMsg, status, id)
	if err != nil {
		return fmt.Errorf("failed to update progress status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("reencryption progress not found: %d", id)
	}

	return nil
}

// IncrementAttempts increments processing attempts counter
func (r *reencryptionProgressRepository) IncrementAttempts(ctx context.Context, id int64) error {
	query := `
		UPDATE REENCRYPTION_PROGRESS
		SET ATTEMPT_COUNT = ATTEMPT_COUNT + 1
		WHERE ID = ?
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to increment attempts: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("reencryption progress not found: %d", id)
	}

	return nil
}

// GetFailedRecords retrieves failed records for retry
func (r *reencryptionProgressRepository) GetFailedRecords(ctx context.Context, jobID, maxAttempts int) ([]*models.ReencryptionProgress, error) {
	query := `
		SELECT * FROM REENCRYPTION_PROGRESS
		WHERE JOB_ID = ?
		  AND STATUS = 'failed'
		  AND ATTEMPT_COUNT < ?
		ORDER BY ID
	`

	var records []*models.ReencryptionProgress
	err := sqlx.SelectContext(ctx, r.db, &records, query, jobID, maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to get failed records: %w", err)
	}

	return records, nil
}
