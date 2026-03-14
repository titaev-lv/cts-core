package rest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/db/models"
	"github.com/titaev-lv/cts-core/internal/db/repository"
)

type wsSessionPersistence struct {
	traderRepo        repository.TraderRepository
	traderSessionRepo repository.TraderSessionRepository
}

func newWSSessionPersistence(dbClient *db.MySQLClient) ws.SessionPersistence {
	if dbClient == nil || dbClient.DB() == nil {
		return nil
	}

	sqlxDB := sqlx.NewDb(dbClient.DB(), "mysql")
	repo := repository.New(sqlxDB)
	return &wsSessionPersistence{
		traderRepo:        repo.Trader(),
		traderSessionRepo: repo.TraderSession(),
	}
}

func (p *wsSessionPersistence) ResolveTraderID(ctx context.Context, traderRef string) (int, error) {
	trader, err := p.traderRepo.GetByCertificateCN(ctx, traderRef)
	if err != nil {
		return 0, err
	}
	if trader == nil {
		return 0, fmt.Errorf("trader not found: %s", traderRef)
	}
	return trader.ID, nil
}

func (p *wsSessionPersistence) CreateSession(ctx context.Context, input ws.SessionCreateInput) error {
	session := &models.TraderSession{
		TraderID:       input.TraderID,
		SessionID:      input.SessionID,
		WSConnectionID: nullableString(input.WSConnectionID),
		IPAddress:      nullableString(input.IPAddress),
		ConnectedAt:    input.ConnectedAt,
		LastHeartbeat:  input.LastHeartbeat,
	}
	return p.traderSessionRepo.Create(ctx, session)
}

func (p *wsSessionPersistence) UpdateHeartbeat(ctx context.Context, sessionID string) error {
	return p.traderSessionRepo.UpdateHeartbeat(ctx, sessionID)
}

func (p *wsSessionPersistence) FinalizeSession(ctx context.Context, sessionID string, reason string, errorMsg *string) error {
	mapped, err := mapDisconnectReason(reason)
	if err != nil {
		return err
	}
	return p.traderSessionRepo.EndSession(ctx, sessionID, mapped, errorMsg)
}

func mapDisconnectReason(reason string) (models.DisconnectReason, error) {
	switch reason {
	case "client_close":
		return models.DisconnectGraceful, nil
	case "timeout":
		return models.DisconnectTimeout, nil
	case "server_shutdown":
		return models.DisconnectServerShutdown, nil
	case "protocol_error":
		return models.DisconnectError, nil
	case "read_error":
		return models.DisconnectError, nil
	default:
		return "", fmt.Errorf("unsupported disconnect reason: %s", reason)
	}
}

func nullableString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
