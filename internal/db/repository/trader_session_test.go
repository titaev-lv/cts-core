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

func TestTraderSessionRepository_Create(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	now := time.Now().UTC()
	session := &models.TraderSession{
		TraderID:       1,
		SessionID:      "session-uuid",
		WSConnectionID: sql.NullString{String: "ws-123", Valid: true},
		IPAddress:      sql.NullString{String: "192.168.1.100", Valid: true},
		ConnectedAt:    now,
		LastHeartbeat:  now,
	}

	mock.ExpectExec("INSERT INTO TRADER_SESSION").
		WithArgs(1, "session-uuid", sql.NullString{String: "ws-123", Valid: true}, sql.NullString{String: "192.168.1.100", Valid: true}, now, now).
		WillReturnResult(sqlmock.NewResult(42, 1))

	err = repo.Create(context.Background(), session)

	assert.NoError(t, err)
	assert.Equal(t, int64(42), session.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_GetByID_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "SESSION_ID", "WS_CONNECTION_ID", "IP_ADDRESS",
		"CONNECTED_AT", "LAST_HEARTBEAT", "ENDED_AT", "DISCONNECT_REASON", "ERROR_MESSAGE",
	}).AddRow(1, 1, "session-uuid", "ws-123", "192.168.1.100", now, now, nil, nil, nil)

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE ID").WithArgs(int64(1)).WillReturnRows(rows)

	session, err := repo.GetByID(context.Background(), 1)

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, int64(1), session.ID)
	assert.Equal(t, "session-uuid", session.SessionID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_GetByID_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE ID").
		WithArgs(int64(999)).
		WillReturnError(sql.ErrNoRows)

	session, err := repo.GetByID(context.Background(), 999)

	assert.NoError(t, err)
	assert.Nil(t, session)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_GetBySessionID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "SESSION_ID", "WS_CONNECTION_ID", "IP_ADDRESS",
		"CONNECTED_AT", "LAST_HEARTBEAT", "ENDED_AT", "DISCONNECT_REASON", "ERROR_MESSAGE",
	}).AddRow(1, 1, "session-uuid", "ws-123", "192.168.1.100", now, now, nil, nil, nil)

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE SESSION_ID").
		WithArgs("session-uuid").
		WillReturnRows(rows)

	session, err := repo.GetBySessionID(context.Background(), "session-uuid")

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, "session-uuid", session.SessionID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_GetActiveByTraderID(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "SESSION_ID", "WS_CONNECTION_ID", "IP_ADDRESS",
		"CONNECTED_AT", "LAST_HEARTBEAT", "ENDED_AT", "DISCONNECT_REASON", "ERROR_MESSAGE",
	}).AddRow(1, 1, "session-uuid", "ws-123", "192.168.1.100", now, now, nil, nil, nil)

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE TRADER_ID.*AND ENDED_AT IS NULL").
		WithArgs(1).
		WillReturnRows(rows)

	session, err := repo.GetActiveByTraderID(context.Background(), 1)

	assert.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, 1, session.TraderID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_ListActive(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "SESSION_ID", "WS_CONNECTION_ID", "IP_ADDRESS",
		"CONNECTED_AT", "LAST_HEARTBEAT", "ENDED_AT", "DISCONNECT_REASON", "ERROR_MESSAGE",
	}).
		AddRow(1, 1, "session-1", "ws-1", "192.168.1.1", now, now, nil, nil, nil).
		AddRow(2, 2, "session-2", "ws-2", "192.168.1.2", now, now, nil, nil, nil)

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE ENDED_AT IS NULL").WillReturnRows(rows)

	sessions, err := repo.ListActive(context.Background())

	assert.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_UpdateHeartbeat(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER_SESSION SET LAST_HEARTBEAT").
		WithArgs(sqlmock.AnyArg(), "session-uuid").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.UpdateHeartbeat(context.Background(), "session-uuid")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_UpdateHeartbeat_IdempotentWhenTimestampUnchanged(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER_SESSION SET LAST_HEARTBEAT").
		WithArgs(sqlmock.AnyArg(), "session-uuid").
		WillReturnResult(sqlmock.NewResult(0, 0))

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "SESSION_ID", "WS_CONNECTION_ID", "IP_ADDRESS",
		"CONNECTED_AT", "LAST_HEARTBEAT", "ENDED_AT", "DISCONNECT_REASON", "ERROR_MESSAGE",
	}).AddRow(1, 1, "session-uuid", "ws-123", "192.168.1.100", now, now, nil, nil, nil)

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE SESSION_ID").
		WithArgs("session-uuid").
		WillReturnRows(rows)

	err = repo.UpdateHeartbeat(context.Background(), "session-uuid")

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_UpdateHeartbeat_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	mock.ExpectExec("UPDATE TRADER_SESSION SET LAST_HEARTBEAT").
		WithArgs(sqlmock.AnyArg(), "missing-session").
		WillReturnResult(sqlmock.NewResult(0, 0))

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE SESSION_ID").
		WithArgs("missing-session").
		WillReturnError(sql.ErrNoRows)

	err = repo.UpdateHeartbeat(context.Background(), "missing-session")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_EndSession(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	errorMsg := "connection lost"
	mock.ExpectExec("UPDATE TRADER_SESSION SET ENDED_AT").
		WithArgs(sqlmock.AnyArg(), models.DisconnectGraceful, &errorMsg, "session-uuid").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.EndSession(context.Background(), "session-uuid", models.DisconnectGraceful, &errorMsg)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_EndSession_IdempotentWhenAlreadyEnded(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	errorMsg := "connection lost"
	mock.ExpectExec("UPDATE TRADER_SESSION SET ENDED_AT").
		WithArgs(sqlmock.AnyArg(), models.DisconnectError, &errorMsg, "session-uuid").
		WillReturnResult(sqlmock.NewResult(0, 0))

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"ID", "TRADER_ID", "SESSION_ID", "WS_CONNECTION_ID", "IP_ADDRESS",
		"CONNECTED_AT", "LAST_HEARTBEAT", "ENDED_AT", "DISCONNECT_REASON", "ERROR_MESSAGE",
	}).AddRow(1, 1, "session-uuid", "ws-123", "192.168.1.100", now, now, now, "error", "connection lost")

	mock.ExpectQuery("SELECT .* FROM TRADER_SESSION WHERE SESSION_ID").
		WithArgs("session-uuid").
		WillReturnRows(rows)

	err = repo.EndSession(context.Background(), "session-uuid", models.DisconnectError, &errorMsg)

	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTraderSessionRepository_CleanupOldSessions(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := NewTraderSessionRepository(sqlxDB)

	mock.ExpectExec("DELETE FROM TRADER_SESSION WHERE ENDED_AT IS NOT NULL").
		WithArgs(30).
		WillReturnResult(sqlmock.NewResult(0, 5))

	deleted, err := repo.CleanupOldSessions(context.Background(), 30)

	assert.NoError(t, err)
	assert.Equal(t, int64(5), deleted)
	assert.NoError(t, mock.ExpectationsWereMet())
}
