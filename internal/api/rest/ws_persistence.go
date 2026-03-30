package rest

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/db/models"
	"github.com/titaev-lv/cts-core/internal/db/repository"
	"github.com/titaev-lv/cts-core/internal/logger"
)

type wsSessionPersistence struct {
	traderRepo        repository.TraderRepository
	traderSessionRepo repository.TraderSessionRepository
	auditLog          interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
	}
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
		auditLog:          logger.GetAudit("ws_persistence"),
	}
}

func (p *wsSessionPersistence) ResolveOrCreateTraderByCN(ctx context.Context, certificateCN string) (ws.TraderIdentity, error) {
	trader, created, err := p.traderRepo.GetOrCreateByCertificateCN(ctx, certificateCN)
	if err != nil {
		return ws.TraderIdentity{}, err
	}
	if trader == nil {
		return ws.TraderIdentity{}, fmt.Errorf("trader not found after resolve-or-create: %s", certificateCN)
	}

	if created {
		if err := p.logTraderAutoCreate(ctx, trader); err != nil {
			p.auditLog.Warn("ws_trader_auto_create_audit_failed", "certificate_cn", trader.CertificateCN, "trader_id", trader.ID, "error", err)
		}
	}

	return ws.TraderIdentity{
		TraderDBID: trader.ID,
		TraderID:   trader.CertificateCN,
		Status:     string(trader.Status),
	}, nil
}

func (p *wsSessionPersistence) logTraderAutoCreate(ctx context.Context, trader *models.Trader) error {
	if p.auditLog == nil || trader == nil {
		return nil
	}

	p.auditLog.Info("audit",
		"action", string(models.AuditActionTraderCreate),
		"resource_type", string(models.ResourceTypeTrader),
		"resource_id", strconv.Itoa(trader.ID),
		"success", true,
		"source", "ws_mtls_auto_create",
		"certificate_cn", trader.CertificateCN,
		"status", string(trader.Status),
		"trader_id", trader.ID,
	)
	return nil
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
