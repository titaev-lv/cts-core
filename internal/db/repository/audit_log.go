package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// auditLogRepository implements AuditLogRepository interface
type auditLogRepository struct {
	db sqlx.ExtContext
}

// NewAuditLogRepository creates a new audit log repository
func NewAuditLogRepository(db *sqlx.DB) AuditLogRepository {
	return &auditLogRepository{db: db}
}

// NewAuditLogRepositoryWithTx creates an audit log repository with transaction context
func NewAuditLogRepositoryWithTx(tx *sqlx.Tx) AuditLogRepository {
	return &auditLogRepository{db: tx}
}

// Create inserts a new audit log entry
func (r *auditLogRepository) Create(ctx context.Context, log *models.AuditLog) error {
	// Set timestamp with microsecond precision
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	query := `
		INSERT INTO AUDIT_LOG (
			TIMESTAMP, UID, ACTION, RESOURCE_TYPE, RESOURCE_ID,
			OLD_VALUE, NEW_VALUE, IP_ADDRESS, USER_AGENT, SUCCESS, ERROR_MESSAGE
		) VALUES (
			:timestamp, :uid, :action, :resource_type, :resource_id,
			:old_value, :new_value, :ip_address, :user_agent, :success, :error_message
		)
	`

	result, err := sqlx.NamedExecContext(ctx, r.db, query, log)
	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	log.ID = id
	return nil
}

// List retrieves audit logs with filters
func (r *auditLogRepository) List(ctx context.Context, filters AuditLogFilters, limit, offset int) ([]*models.AuditLog, error) {
	query := `
		SELECT ID, TIMESTAMP, UID, ACTION, RESOURCE_TYPE, RESOURCE_ID,
		       OLD_VALUE, NEW_VALUE, IP_ADDRESS, USER_AGENT, SUCCESS, ERROR_MESSAGE
		FROM AUDIT_LOG
		WHERE 1=1
	`
	args := []interface{}{}

	// Apply filters
	if filters.UID != nil {
		query += " AND UID = ?"
		args = append(args, *filters.UID)
	}
	if filters.Action != nil {
		query += " AND ACTION = ?"
		args = append(args, *filters.Action)
	}
	if filters.ResourceType != nil {
		query += " AND RESOURCE_TYPE = ?"
		args = append(args, *filters.ResourceType)
	}
	if filters.ResourceID != nil {
		query += " AND RESOURCE_ID = ?"
		args = append(args, *filters.ResourceID)
	}
	if filters.Success != nil {
		query += " AND SUCCESS = ?"
		args = append(args, *filters.Success)
	}
	if filters.FromDate != nil {
		query += " AND TIMESTAMP >= ?"
		args = append(args, *filters.FromDate)
	}
	if filters.ToDate != nil {
		query += " AND TIMESTAMP <= ?"
		args = append(args, *filters.ToDate)
	}

	query += " ORDER BY TIMESTAMP DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var logs []*models.AuditLog
	err := sqlx.SelectContext(ctx, r.db, &logs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}

	return logs, nil
}

// Count returns total count matching filters
func (r *auditLogRepository) Count(ctx context.Context, filters AuditLogFilters) (int64, error) {
	query := "SELECT COUNT(*) FROM AUDIT_LOG WHERE 1=1"
	args := []interface{}{}

	// Apply same filters as List
	if filters.UID != nil {
		query += " AND UID = ?"
		args = append(args, *filters.UID)
	}
	if filters.Action != nil {
		query += " AND ACTION = ?"
		args = append(args, *filters.Action)
	}
	if filters.ResourceType != nil {
		query += " AND RESOURCE_TYPE = ?"
		args = append(args, *filters.ResourceType)
	}
	if filters.ResourceID != nil {
		query += " AND RESOURCE_ID = ?"
		args = append(args, *filters.ResourceID)
	}
	if filters.Success != nil {
		query += " AND SUCCESS = ?"
		args = append(args, *filters.Success)
	}
	if filters.FromDate != nil {
		query += " AND TIMESTAMP >= ?"
		args = append(args, *filters.FromDate)
	}
	if filters.ToDate != nil {
		query += " AND TIMESTAMP <= ?"
		args = append(args, *filters.ToDate)
	}

	var count int64
	err := sqlx.GetContext(ctx, r.db, &count, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}

	return count, nil
}

// CleanupOld deletes audit logs older than retention period
func (r *auditLogRepository) CleanupOld(ctx context.Context, retentionDays int) (int64, error) {
	query := `
		DELETE FROM AUDIT_LOG
		WHERE TIMESTAMP < DATE_SUB(NOW(), INTERVAL ? DAY)
	`

	result, err := r.db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old audit logs: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rows, nil
}

// Helper function to build WHERE clause from filters
func buildWhereClause(filters AuditLogFilters) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if filters.UID != nil {
		conditions = append(conditions, "UID = ?")
		args = append(args, *filters.UID)
	}
	if filters.Action != nil {
		conditions = append(conditions, "ACTION = ?")
		args = append(args, *filters.Action)
	}
	if filters.ResourceType != nil {
		conditions = append(conditions, "RESOURCE_TYPE = ?")
		args = append(args, *filters.ResourceType)
	}
	if filters.ResourceID != nil {
		conditions = append(conditions, "RESOURCE_ID = ?")
		args = append(args, *filters.ResourceID)
	}
	if filters.Success != nil {
		conditions = append(conditions, "SUCCESS = ?")
		args = append(args, *filters.Success)
	}
	if filters.FromDate != nil {
		conditions = append(conditions, "TIMESTAMP >= ?")
		args = append(args, *filters.FromDate)
	}
	if filters.ToDate != nil {
		conditions = append(conditions, "TIMESTAMP <= ?")
		args = append(args, *filters.ToDate)
	}

	if len(conditions) == 0 {
		return "", args
	}

	return " WHERE " + strings.Join(conditions, " AND "), args
}
