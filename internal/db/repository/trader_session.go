package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/db/models"
)

// traderSessionRepository implements TraderSessionRepository interface
type traderSessionRepository struct {
	db sqlx.ExtContext
}

// NewTraderSessionRepository creates a new trader session repository
func NewTraderSessionRepository(db *sqlx.DB) TraderSessionRepository {
	return &traderSessionRepository{db: db}
}

// NewTraderSessionRepositoryWithTx creates a trader session repository with transaction context
func NewTraderSessionRepositoryWithTx(tx *sqlx.Tx) TraderSessionRepository {
	return &traderSessionRepository{db: tx}
}

// Create starts a new session
func (r *traderSessionRepository) Create(ctx context.Context, session *models.TraderSession) error {
	query := `
		INSERT INTO TRADER_SESSION (
			TRADER_ID, SESSION_ID, WS_CONNECTION_ID, IP_ADDRESS,
			CONNECTED_AT, LAST_HEARTBEAT
		) VALUES (
			:trader_id, :session_id, :ws_connection_id, :ip_address,
			:connected_at, :last_heartbeat
		)
	`

	params := map[string]interface{}{
		"trader_id":        session.TraderID,
		"session_id":       session.SessionID,
		"ws_connection_id": session.WSConnectionID,
		"ip_address":       session.IPAddress,
		"connected_at":     session.ConnectedAt,
		"last_heartbeat":   session.LastHeartbeat,
	}

	result, err := sqlx.NamedExecContext(ctx, r.db, query, params)
	if err != nil {
		return fmt.Errorf("failed to create trader session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	session.ID = id
	return nil
}

// GetByID retrieves a session by ID
func (r *traderSessionRepository) GetByID(ctx context.Context, id int64) (*models.TraderSession, error) {
	query := `
		SELECT ID, TRADER_ID, SESSION_ID, WS_CONNECTION_ID, IP_ADDRESS,
		       CONNECTED_AT, LAST_HEARTBEAT, ENDED_AT, DISCONNECT_REASON, ERROR_MESSAGE
		FROM TRADER_SESSION
		WHERE ID = ?
	`

	var session models.TraderSession
	err := sqlx.GetContext(ctx, r.db, &session, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session by id: %w", err)
	}

	return &session, nil
}

// GetBySessionID retrieves a session by UUID
func (r *traderSessionRepository) GetBySessionID(ctx context.Context, sessionID string) (*models.TraderSession, error) {
	query := `
		SELECT ID, TRADER_ID, SESSION_ID, WS_CONNECTION_ID, IP_ADDRESS,
		       CONNECTED_AT, LAST_HEARTBEAT, ENDED_AT, DISCONNECT_REASON, ERROR_MESSAGE
		FROM TRADER_SESSION
		WHERE SESSION_ID = ?
	`

	var session models.TraderSession
	err := sqlx.GetContext(ctx, r.db, &session, query, sessionID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session by session id: %w", err)
	}

	return &session, nil
}

// GetActiveByTraderID retrieves active session for a trader
func (r *traderSessionRepository) GetActiveByTraderID(ctx context.Context, traderID int) (*models.TraderSession, error) {
	query := `
		SELECT ID, TRADER_ID, SESSION_ID, WS_CONNECTION_ID, IP_ADDRESS,
		       CONNECTED_AT, LAST_HEARTBEAT, ENDED_AT, DISCONNECT_REASON, ERROR_MESSAGE
		FROM TRADER_SESSION
		WHERE TRADER_ID = ? AND ENDED_AT IS NULL
		ORDER BY CONNECTED_AT DESC
		LIMIT 1
	`

	var session models.TraderSession
	err := sqlx.GetContext(ctx, r.db, &session, query, traderID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	return &session, nil
}

// ListActive retrieves all active sessions
func (r *traderSessionRepository) ListActive(ctx context.Context) ([]*models.TraderSession, error) {
	query := `
		SELECT ID, TRADER_ID, SESSION_ID, WS_CONNECTION_ID, IP_ADDRESS,
		       CONNECTED_AT, LAST_HEARTBEAT, ENDED_AT, DISCONNECT_REASON, ERROR_MESSAGE
		FROM TRADER_SESSION
		WHERE ENDED_AT IS NULL
		ORDER BY CONNECTED_AT DESC
	`

	var sessions []*models.TraderSession
	err := sqlx.SelectContext(ctx, r.db, &sessions, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list active sessions: %w", err)
	}

	return sessions, nil
}

// UpdateHeartbeat updates last heartbeat timestamp
func (r *traderSessionRepository) UpdateHeartbeat(ctx context.Context, sessionID string) error {
	query := `
		UPDATE TRADER_SESSION
		SET LAST_HEARTBEAT = ?
		WHERE SESSION_ID = ? AND ENDED_AT IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, time.Now(), sessionID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found or already ended: %s", sessionID)
	}

	return nil
}

// EndSession marks session as ended
func (r *traderSessionRepository) EndSession(ctx context.Context, sessionID string, reason models.DisconnectReason, errorMsg *string) error {
	query := `
		UPDATE TRADER_SESSION
		SET ENDED_AT = ?,
		    DISCONNECT_REASON = ?,
		    ERROR_MESSAGE = ?
		WHERE SESSION_ID = ? AND ENDED_AT IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, time.Now(), reason, errorMsg, sessionID)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		session, lookupErr := r.GetBySessionID(ctx, sessionID)
		if lookupErr != nil {
			return fmt.Errorf("failed to verify session end state: %w", lookupErr)
		}
		if session == nil {
			return fmt.Errorf("session not found: %s", sessionID)
		}
		if session.EndedAt.Valid {
			return nil
		}
		return fmt.Errorf("session not ended and no rows affected: %s", sessionID)
	}

	return nil
}

// CleanupOldSessions deletes sessions older than retention period
func (r *traderSessionRepository) CleanupOldSessions(ctx context.Context, retentionDays int) (int64, error) {
	query := `
		DELETE FROM TRADER_SESSION
		WHERE ENDED_AT IS NOT NULL
		  AND ENDED_AT < DATE_SUB(NOW(), INTERVAL ? DAY)
	`

	result, err := r.db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old sessions: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rows, nil
}
