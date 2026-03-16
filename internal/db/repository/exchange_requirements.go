package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// exchangeRequirementsRepository implements ExchangeRequirementsRepository.
type exchangeRequirementsRepository struct {
	db sqlx.ExtContext
}

// NewExchangeRequirementsRepository creates requirements repository for DB connection.
func NewExchangeRequirementsRepository(db *sqlx.DB) ExchangeRequirementsRepository {
	return &exchangeRequirementsRepository{db: db}
}

// NewExchangeRequirementsRepositoryWithTx creates requirements repository in transaction context.
func NewExchangeRequirementsRepositoryWithTx(tx *sqlx.Tx) ExchangeRequirementsRepository {
	return &exchangeRequirementsRepository{db: tx}
}

func (r *exchangeRequirementsRepository) ListTradeExchanges(ctx context.Context) ([]ExchangeInfo, error) {
	query := "SELECT DISTINCT " +
		"e.ID AS exchange_id, " +
		"LOWER(e.NAME) AS exchange_name " +
		"FROM TRADE t " +
		"INNER JOIN `USER` u ON u.ID = t.UID " +
		"INNER JOIN USERS_GROUP ug ON ug.UID = u.ID AND ug.GID = 2 " +
		"INNER JOIN `GROUP` g ON g.ID = ug.GID " +
		"INNER JOIN TRADE_PAIRS tp ON tp.TRADE_ID = t.ID " +
		"INNER JOIN TRADE_PAIR tpdef ON tpdef.ID = tp.PAIR_ID " +
		"INNER JOIN EXCHANGE e ON e.ID = tpdef.EXCHANGE_ID " +
		"WHERE t.ACTIVE = 1 " +
		"AND u.ACTIVE = 1 " +
		"AND g.ACTIVE = 1 " +
		"AND tpdef.ACTIVE = 1 " +
		"AND e.ACTIVE = 1 " +
		"ORDER BY exchange_name"

	var items []ExchangeInfo
	if err := sqlx.SelectContext(ctx, r.db, &items, query); err != nil {
		return nil, fmt.Errorf("failed to list trade exchanges: %w", err)
	}
	return items, nil
}

func (r *exchangeRequirementsRepository) ListMonitorExchanges(ctx context.Context) ([]ExchangeInfo, error) {
	query := "SELECT DISTINCT " +
		"e.ID AS exchange_id, " +
		"LOWER(e.NAME) AS exchange_name " +
		"FROM MONITORING m " +
		"INNER JOIN `USER` u ON u.ID = m.UID " +
		"INNER JOIN MONITORING_TRADE_PAIRS mtp ON mtp.MONITOR_ID = m.ID " +
		"INNER JOIN TRADE_PAIR tp ON tp.ID = mtp.PAIR_ID " +
		"INNER JOIN EXCHANGE e ON e.ID = tp.EXCHANGE_ID " +
		"WHERE m.ACTIVE = 1 " +
		"AND u.ACTIVE = 1 " +
		"AND tp.ACTIVE = 1 " +
		"AND e.ACTIVE = 1 " +
		"ORDER BY exchange_name"

	var items []ExchangeInfo
	if err := sqlx.SelectContext(ctx, r.db, &items, query); err != nil {
		return nil, fmt.Errorf("failed to list monitor exchanges: %w", err)
	}
	return items, nil
}
