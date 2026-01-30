package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogRepository_Create(t *testing.T) {
	t.Skip("Requires integration test with real MySQL (NamedExecContext)")
}

func TestAuditLogRepository_List_NoFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewAuditLogRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TIMESTAMP", "UID", "ACTION", "RESOURCE_TYPE", "RESOURCE_ID",
		"OLD_VALUE", "NEW_VALUE", "IP_ADDRESS", "USER_AGENT", "SUCCESS", "ERROR_MESSAGE",
	}).
		AddRow(1, now, 1, "CREATE", "trader", "123", nil, `{"name":"Test"}`, "192.168.1.1", "Mozilla", true, nil).
		AddRow(2, now, 1, "UPDATE", "trader", "123", `{"name":"Test"}`, `{"name":"Test2"}`, "192.168.1.1", "Mozilla", true, nil)

	mock.ExpectQuery("SELECT .* FROM AUDIT_LOG WHERE 1=1 ORDER BY TIMESTAMP DESC").
		WithArgs(10, 0).
		WillReturnRows(rows)

	logs, err := repo.List(context.Background(), AuditLogFilters{}, 10, 0)

	assert.NoError(t, err)
	assert.Len(t, logs, 2)
	assert.Equal(t, "CREATE", logs[0].Action)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogRepository_List_WithFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewAuditLogRepository(sqlxDB)

	now := time.Now()
	uid := int32(1)
	action := "CREATE"
	resourceType := "trader"
	
	rows := sqlmock.NewRows([]string{
		"ID", "TIMESTAMP", "UID", "ACTION", "RESOURCE_TYPE", "RESOURCE_ID",
		"OLD_VALUE", "NEW_VALUE", "IP_ADDRESS", "USER_AGENT", "SUCCESS", "ERROR_MESSAGE",
	}).AddRow(1, now, 1, "CREATE", "trader", "123", nil, `{"name":"Test"}`, "192.168.1.1", "Mozilla", true, nil)

	mock.ExpectQuery("SELECT .* FROM AUDIT_LOG WHERE 1=1 AND UID = .* AND ACTION = .* AND RESOURCE_TYPE = .* ORDER BY TIMESTAMP DESC").
		WithArgs(uid, action, resourceType, 10, 0).
		WillReturnRows(rows)

	filters := AuditLogFilters{
		UID:          &uid,
		Action:       &action,
		ResourceType: &resourceType,
	}
	logs, err := repo.List(context.Background(), filters, 10, 0)

	assert.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, "CREATE", logs[0].Action)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogRepository_Count_NoFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewAuditLogRepository(sqlxDB)

	rows := sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(42)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM AUDIT_LOG WHERE 1=1").WillReturnRows(rows)

	count, err := repo.Count(context.Background(), AuditLogFilters{})

	assert.NoError(t, err)
	assert.Equal(t, int64(42), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogRepository_Count_WithFilters(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewAuditLogRepository(sqlxDB)

	uid := int32(1)
	success := true

	rows := sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(10)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM AUDIT_LOG WHERE 1=1 AND UID = .* AND SUCCESS = .*").
		WithArgs(uid, success).
		WillReturnRows(rows)

	filters := AuditLogFilters{
		UID:     &uid,
		Success: &success,
	}
	count, err := repo.Count(context.Background(), filters)

	assert.NoError(t, err)
	assert.Equal(t, int64(10), count)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuditLogRepository_CleanupOld(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewAuditLogRepository(sqlxDB)

	mock.ExpectExec("DELETE FROM AUDIT_LOG WHERE TIMESTAMP < DATE_SUB").
		WithArgs(90).
		WillReturnResult(sqlmock.NewResult(0, 123))

	deleted, err := repo.CleanupOld(context.Background(), 90)

	assert.NoError(t, err)
	assert.Equal(t, int64(123), deleted)
	assert.NoError(t, mock.ExpectationsWereMet())
}
