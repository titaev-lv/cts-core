-- ============================================================================
-- CTS-Core Migration 003: Enforce Single Active Trader Session
-- ============================================================================
-- Created: 2026-03-30
-- Purpose:
--   1) Ensure at most one active session (ENDED_AT IS NULL) per TRADER_ID.
--   2) Make the invariant race-safe at DB level for concurrent registrations.
-- ============================================================================

USE ct_system;

SELECT 'start: migration 003_single_active_trader_session' AS info;

SELECT COUNT(*) INTO @tbl_trader_session_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION';

-- Keep only the most recent active session per trader before adding uniqueness.
SET @sql = IF(
    @tbl_trader_session_exists = 1,
    'UPDATE TRADER_SESSION ts JOIN (SELECT id FROM (SELECT ID, ROW_NUMBER() OVER (PARTITION BY TRADER_ID ORDER BY CONNECTED_AT DESC, ID DESC) AS rn FROM TRADER_SESSION WHERE ENDED_AT IS NULL) ranked WHERE ranked.rn > 1) dup ON dup.id = ts.ID SET ts.ENDED_AT = COALESCE(ts.LAST_HEARTBEAT, ts.CONNECTED_AT, NOW()), ts.DISCONNECT_REASON = ''kicked'', ts.ERROR_MESSAGE = COALESCE(ts.ERROR_MESSAGE, ''migration_003_duplicate_active_session_closed'') WHERE ts.ENDED_AT IS NULL',
    'SELECT ''skip: TRADER_SESSION table missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @col_active_slot_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION'
  AND COLUMN_NAME = 'ACTIVE_SESSION_SLOT';

SET @sql = IF(
    @tbl_trader_session_exists = 1 AND @col_active_slot_exists = 0,
    'ALTER TABLE TRADER_SESSION ADD COLUMN ACTIVE_SESSION_SLOT TINYINT GENERATED ALWAYS AS (CASE WHEN ENDED_AT IS NULL THEN 1 ELSE NULL END) STORED',
    'SELECT ''skip: TRADER_SESSION.ACTIVE_SESSION_SLOT already exists or table missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @idx_single_active_exists
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION'
  AND INDEX_NAME = 'uk_trader_single_active_session';

SET @sql = IF(
    @tbl_trader_session_exists = 1 AND @idx_single_active_exists = 0,
    'ALTER TABLE TRADER_SESSION ADD UNIQUE KEY uk_trader_single_active_session (TRADER_ID, ACTIVE_SESSION_SLOT)',
    'SELECT ''skip: TRADER_SESSION.uk_trader_single_active_session already exists or table missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT 'done: migration 003_single_active_trader_session' AS info;
