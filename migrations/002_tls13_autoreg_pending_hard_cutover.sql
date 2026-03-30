-- ============================================================================
-- CTS-Core Migration 002: TLS1.3 + Auto-Registration (pending) Hard Cutover
-- ============================================================================
-- Created: 2026-03-29
-- Purpose:
--   1) Perform destructive cleanup of trader/runtime legacy data.
--   2) Remove legacy TRADER status "registered" from schema.
--   3) Set TRADER.STATUS default to "pending".
--
-- IMPORTANT:
--   - This migration is intentionally destructive.
--   - Legacy data is not preserved by design.
-- ============================================================================

USE ct_system;

SELECT 'start: migration 002_tls13_autoreg_pending_hard_cutover' AS info;

-- ============================================================================
-- 1. Detach legacy trader links from mutable business tables
-- ============================================================================

SELECT COUNT(*) INTO @col_monitoring_assigned_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'MONITORING'
  AND COLUMN_NAME = 'ASSIGNED_TRADER_ID';

SET @sql = IF(
    @col_monitoring_assigned_exists = 1,
    'UPDATE MONITORING SET ASSIGNED_TRADER_ID = NULL WHERE ASSIGNED_TRADER_ID IS NOT NULL',
    'SELECT ''skip: MONITORING.ASSIGNED_TRADER_ID missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @col_monitoring_backup_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'MONITORING'
  AND COLUMN_NAME = 'BACKUP_TRADER_ID';

SET @sql = IF(
    @col_monitoring_backup_exists = 1,
    'UPDATE MONITORING SET BACKUP_TRADER_ID = NULL WHERE BACKUP_TRADER_ID IS NOT NULL',
    'SELECT ''skip: MONITORING.BACKUP_TRADER_ID missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @col_monitoring_assigned_at_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'MONITORING'
  AND COLUMN_NAME = 'ASSIGNED_AT';

SET @sql = IF(
    @col_monitoring_assigned_at_exists = 1,
    'UPDATE MONITORING SET ASSIGNED_AT = NULL WHERE ASSIGNED_AT IS NOT NULL',
    'SELECT ''skip: MONITORING.ASSIGNED_AT missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @col_trade_trader_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADE'
  AND COLUMN_NAME = 'TRADER_ID';

SET @sql = IF(
    @col_trade_trader_exists = 1,
    'UPDATE TRADE SET TRADER_ID = NULL WHERE TRADER_ID IS NOT NULL',
    'SELECT ''skip: TRADE.TRADER_ID missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ============================================================================
-- 2. Destructive cleanup of trader/runtime data
-- ============================================================================

SELECT COUNT(*) INTO @tbl_arbitrage_order_trans_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'ARBITRAGE_ORDER_TRANS';

SET @sql = IF(
    @tbl_arbitrage_order_trans_exists = 1,
    'DELETE FROM ARBITRAGE_ORDER_TRANS',
    'SELECT ''skip: ARBITRAGE_ORDER_TRANS missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @tbl_arbitrage_order_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'ARBITRAGE_ORDER';

SET @sql = IF(
    @tbl_arbitrage_order_exists = 1,
    'DELETE FROM ARBITRAGE_ORDER',
    'SELECT ''skip: ARBITRAGE_ORDER missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @tbl_trader_resource_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_EXCHANGE_RESOURCE';

SET @sql = IF(
    @tbl_trader_resource_exists = 1,
    'DELETE FROM TRADER_EXCHANGE_RESOURCE',
    'SELECT ''skip: TRADER_EXCHANGE_RESOURCE missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @tbl_trader_session_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION';

SET @sql = IF(
    @tbl_trader_session_exists = 1,
    'DELETE FROM TRADER_SESSION',
    'SELECT ''skip: TRADER_SESSION missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT COUNT(*) INTO @tbl_trader_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER';

SET @sql = IF(
    @tbl_trader_exists = 1,
    'DELETE FROM TRADER',
    'SELECT ''skip: TRADER missing'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Reset identity counters for clean restart from new model.
SET @sql = IF(
    @tbl_arbitrage_order_trans_exists = 1,
    'ALTER TABLE ARBITRAGE_ORDER_TRANS AUTO_INCREMENT = 1',
    'SELECT ''skip: ARBITRAGE_ORDER_TRANS auto_increment reset'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
    @tbl_arbitrage_order_exists = 1,
    'ALTER TABLE ARBITRAGE_ORDER AUTO_INCREMENT = 1',
    'SELECT ''skip: ARBITRAGE_ORDER auto_increment reset'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
    @tbl_trader_resource_exists = 1,
    'ALTER TABLE TRADER_EXCHANGE_RESOURCE AUTO_INCREMENT = 1',
    'SELECT ''skip: TRADER_EXCHANGE_RESOURCE auto_increment reset'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
    @tbl_trader_session_exists = 1,
    'ALTER TABLE TRADER_SESSION AUTO_INCREMENT = 1',
    'SELECT ''skip: TRADER_SESSION auto_increment reset'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
    @tbl_trader_exists = 1,
    'ALTER TABLE TRADER AUTO_INCREMENT = 1',
    'SELECT ''skip: TRADER auto_increment reset'''
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- ============================================================================
-- 3. Schema change: TRADER.STATUS enum migration
-- ============================================================================

SELECT COLUMN_TYPE, COLUMN_DEFAULT
INTO @trader_status_type, @trader_status_default
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER'
  AND COLUMN_NAME = 'STATUS'
LIMIT 1;

SET @sql = IF(
    @trader_status_type IS NULL,
    'SELECT ''skip: TRADER.STATUS missing''',
    IF(
        @trader_status_type LIKE '%registered%' OR @trader_status_default <> 'pending',
        'ALTER TABLE TRADER MODIFY COLUMN STATUS ENUM(''pending'',''active'',''suspended'',''decommissioned'') NOT NULL DEFAULT ''pending''',
        'SELECT ''skip: TRADER.STATUS already migrated'''
    )
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT 'done: migration 002_tls13_autoreg_pending_hard_cutover' AS info;
