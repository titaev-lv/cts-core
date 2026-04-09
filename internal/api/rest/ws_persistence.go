package rest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/api/ws"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/db/models"
	"github.com/titaev-lv/cts-core/internal/db/repository"
	"github.com/titaev-lv/cts-core/internal/logger"
)

type wsSessionPersistence struct {
	db                sqlx.ExtContext
	traderRepo        repository.TraderRepository
	traderSessionRepo repository.TraderSessionRepository
	staleAfter        time.Duration
	auditLog          interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
	}
}

func newWSSessionPersistence(dbClient *db.MySQLClient, staleAfter time.Duration) ws.SessionPersistence {
	if dbClient == nil || dbClient.DB() == nil {
		return nil
	}

	sqlxDB := sqlx.NewDb(dbClient.DB(), "mysql")
	repo := repository.New(sqlxDB)
	return &wsSessionPersistence{
		db:                sqlxDB,
		traderRepo:        repo.Trader(),
		traderSessionRepo: repo.TraderSession(),
		staleAfter:        staleAfter,
		auditLog:          logger.GetAudit("ws_persistence"),
	}
}

type wsAvailableExchangeRow struct {
	ExchangeID   int            `db:"exchange_id"`
	Code         string         `db:"code"`
	Name         string         `db:"name"`
	Enabled      bool           `db:"enabled"`
	WSEndpoint   sql.NullString `db:"ws_endpoint"`
	RESTEndpoint sql.NullString `db:"rest_endpoint"`
	MarketTypes  sql.NullString `db:"market_types"`
}

func (p *wsSessionPersistence) ListAvailableExchanges(ctx context.Context) ([]ws.ExchangeCatalogEntry, error) {
	if p == nil || p.db == nil {
		return nil, nil
	}

	const query = "SELECT " +
		"e.ID AS exchange_id, " +
		"LOWER(e.NAME) AS code, " +
		"e.NAME AS name, " +
		"e.ACTIVE AS enabled, " +
		"NULLIF(TRIM(e.WEBSOCKET_URL), '') AS ws_endpoint, " +
		"COALESCE(NULLIF(TRIM(e.BASE_URL), ''), NULLIF(TRIM(e.URL), '')) AS rest_endpoint, " +
		"GROUP_CONCAT(DISTINCT LOWER(tp.MARKET_TYPE) ORDER BY LOWER(tp.MARKET_TYPE) SEPARATOR ',') AS market_types " +
		"FROM TRADE t " +
		"INNER JOIN `USER` u ON u.ID = t.UID " +
		"INNER JOIN USERS_GROUP ug ON ug.UID = u.ID AND ug.GID = 2 " +
		"INNER JOIN `GROUP` g ON g.ID = ug.GID " +
		"INNER JOIN TRADE_PAIRS tps ON tps.TRADE_ID = t.ID " +
		"INNER JOIN EXCHANGE_ACCOUNTS ea ON ea.ID = tps.EAID " +
		"INNER JOIN TRADE_PAIR tp ON tp.ID = tps.PAIR_ID " +
		"INNER JOIN EXCHANGE e ON e.ID = tp.EXCHANGE_ID AND e.ID = ea.EXID " +
		"WHERE t.ACTIVE = 1 " +
		"AND u.ACTIVE = 1 " +
		"AND g.ACTIVE = 1 " +
		"AND ea.ACTIVE = 1 " +
		"AND tp.ACTIVE = 1 " +
		"AND e.ACTIVE = 1 " +
		"AND COALESCE(e.DELETED, 0) = 0 " +
		"GROUP BY e.ID, e.NAME, e.ACTIVE, e.WEBSOCKET_URL, e.BASE_URL, e.URL " +
		"ORDER BY code"

	rows := make([]wsAvailableExchangeRow, 0)
	if err := sqlx.SelectContext(ctx, p.db, &rows, query); err != nil {
		return nil, fmt.Errorf("list available exchanges: %w", err)
	}

	items := make([]ws.ExchangeCatalogEntry, 0, len(rows))
	for _, row := range rows {
		entry := ws.ExchangeCatalogEntry{
			ExchangeID:   row.ExchangeID,
			Code:         strings.TrimSpace(row.Code),
			Name:         strings.TrimSpace(row.Name),
			Enabled:      row.Enabled,
			MarketTypes:  parseMarketTypesCSV(row.MarketTypes.String),
			RESTEndpoint: strings.TrimSpace(row.RESTEndpoint.String),
			RateLimits: map[string]int{
				"global": 20,
			},
		}
		wsEndpoint := strings.TrimSpace(row.WSEndpoint.String)
		if wsEndpoint != "" {
			entry.WSPublicEndpoint = wsEndpoint
			entry.WSPrivateEndpoint = wsEndpoint
		}
		if len(entry.MarketTypes) == 0 {
			entry.MarketTypes = []string{"spot"}
		}
		items = append(items, entry)
	}

	return items, nil
}

func parseMarketTypesCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	seen := make(map[string]struct{}, 2)
	out := make([]string, 0, 2)
	for _, item := range strings.Split(raw, ",") {
		value := strings.ToLower(strings.TrimSpace(item))
		if value != "spot" && value != "futures" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	return out
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
	if err := p.traderSessionRepo.Create(ctx, session); err != nil {
		if !isMySQLDuplicateEntryError(err) {
			return err
		}

		recovered, recoverErr := p.tryRecoverStaleActiveSession(ctx, input)
		if recoverErr != nil {
			return recoverErr
		}
		if !recovered {
			return fmt.Errorf("%w: trader_id=%d", ws.ErrActiveSessionExists, input.TraderID)
		}

		if retryErr := p.traderSessionRepo.Create(ctx, session); retryErr != nil {
			if isMySQLDuplicateEntryError(retryErr) {
				return fmt.Errorf("%w: trader_id=%d", ws.ErrActiveSessionExists, input.TraderID)
			}
			return retryErr
		}
	}
	return nil
}

func (p *wsSessionPersistence) tryRecoverStaleActiveSession(ctx context.Context, input ws.SessionCreateInput) (bool, error) {
	if p == nil || p.traderSessionRepo == nil || p.staleAfter <= 0 {
		return false, nil
	}

	active, err := p.traderSessionRepo.GetActiveByTraderID(ctx, input.TraderID)
	if err != nil {
		return false, err
	}
	if active == nil {
		return false, nil
	}

	now := input.ConnectedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if !isSessionStale(active, now, p.staleAfter) {
		return false, nil
	}

	errMsg := fmt.Sprintf("stale session auto-finalized before new register: stale_after=%s last_heartbeat=%s", p.staleAfter, active.LastHeartbeat.UTC().Format(time.RFC3339Nano))
	if err := p.traderSessionRepo.EndSession(ctx, active.SessionID, models.DisconnectTimeout, &errMsg); err != nil {
		return false, err
	}
	if p.auditLog != nil {
		p.auditLog.Warn("ws_stale_active_session_recovered", "trader_id", input.TraderID, "stale_session_id", active.SessionID, "stale_after", p.staleAfter.String(), "last_heartbeat", active.LastHeartbeat.UTC().Format(time.RFC3339Nano))
	}

	return true, nil
}

func isSessionStale(session *models.TraderSession, now time.Time, staleAfter time.Duration) bool {
	if session == nil || staleAfter <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	reference := session.LastHeartbeat
	if reference.IsZero() {
		reference = session.ConnectedAt
	}
	if reference.IsZero() {
		return false
	}

	return !reference.Add(staleAfter).After(now)
}

func (p *wsSessionPersistence) UpdateTraderRelease(ctx context.Context, traderID int, release string) error {
	if traderID <= 0 {
		return fmt.Errorf("invalid trader id for release update: %d", traderID)
	}
	return p.traderRepo.UpdateRelease(ctx, traderID, release)
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

func isMySQLDuplicateEntryError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
