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
--
-- DBeaver line-by-line mode:
--   1) Execute checks first.
--   2) Execute DDL/DML only when corresponding check says it is required.
-- ============================================================================

USE ct_system;

SELECT 'start: migration 002_tls13_autoreg_pending_hard_cutover' AS info;

-- ============================================================================
-- 1. Detach legacy trader links from mutable business tables
-- ============================================================================

-- Step 1.1 check
SELECT COUNT(*) AS monitoring_assigned_trader_id_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'MONITORING'
  AND COLUMN_NAME = 'ASSIGNED_TRADER_ID';

-- Step 1.1 execute ONLY if monitoring_assigned_trader_id_exists=1
UPDATE MONITORING
SET ASSIGNED_TRADER_ID = NULL
WHERE ASSIGNED_TRADER_ID IS NOT NULL;

-- Step 1.2 check
SELECT COUNT(*) AS monitoring_backup_trader_id_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'MONITORING'
  AND COLUMN_NAME = 'BACKUP_TRADER_ID';

-- Step 1.2 execute ONLY if monitoring_backup_trader_id_exists=1
UPDATE MONITORING
SET BACKUP_TRADER_ID = NULL
WHERE BACKUP_TRADER_ID IS NOT NULL;

-- Step 1.3 check
SELECT COUNT(*) AS monitoring_assigned_at_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'MONITORING'
  AND COLUMN_NAME = 'ASSIGNED_AT';

-- Step 1.3 execute ONLY if monitoring_assigned_at_exists=1
UPDATE MONITORING
SET ASSIGNED_AT = NULL
WHERE ASSIGNED_AT IS NOT NULL;

-- Step 1.4 check
SELECT COUNT(*) AS trade_trader_id_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADE'
  AND COLUMN_NAME = 'TRADER_ID';

-- Step 1.4 execute ONLY if trade_trader_id_exists=1
UPDATE TRADE
SET TRADER_ID = NULL
WHERE TRADER_ID IS NOT NULL;

-- ============================================================================
-- 2. Destructive cleanup of trader/runtime data
-- ============================================================================

-- Step 2.1 check
SELECT COUNT(*) AS arbitrage_order_trans_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'ARBITRAGE_ORDER_TRANS';

-- Step 2.1 execute ONLY if arbitrage_order_trans_exists=1
DELETE FROM ARBITRAGE_ORDER_TRANS;

-- Step 2.2 check
SELECT COUNT(*) AS arbitrage_order_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'ARBITRAGE_ORDER';

-- Step 2.2 execute ONLY if arbitrage_order_exists=1
DELETE FROM ARBITRAGE_ORDER;

-- Step 2.3 check
SELECT COUNT(*) AS trader_exchange_resource_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_EXCHANGE_RESOURCE';

-- Step 2.3 execute ONLY if trader_exchange_resource_exists=1
DELETE FROM TRADER_EXCHANGE_RESOURCE;

-- Step 2.4 check
SELECT COUNT(*) AS trader_session_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION';

-- Step 2.4 execute ONLY if trader_session_exists=1
DELETE FROM TRADER_SESSION;

-- Step 2.5 check
SELECT COUNT(*) AS trader_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER';

-- Step 2.5 execute ONLY if trader_exists=1
DELETE FROM TRADER;

-- Reset identity counters for clean restart from new model.
-- Step 2.6 execute ONLY if arbitrage_order_trans_exists=1
ALTER TABLE ARBITRAGE_ORDER_TRANS AUTO_INCREMENT = 1;

-- Step 2.7 execute ONLY if arbitrage_order_exists=1
ALTER TABLE ARBITRAGE_ORDER AUTO_INCREMENT = 1;

-- Step 2.8 execute ONLY if trader_exchange_resource_exists=1
ALTER TABLE TRADER_EXCHANGE_RESOURCE AUTO_INCREMENT = 1;

-- Step 2.9 execute ONLY if trader_session_exists=1
ALTER TABLE TRADER_SESSION AUTO_INCREMENT = 1;

-- Step 2.10 execute ONLY if trader_exists=1
ALTER TABLE TRADER AUTO_INCREMENT = 1;

-- ============================================================================
-- 3. Schema change: TRADER.STATUS enum migration
-- ============================================================================

-- Step 3.1 check current STATUS definition.
SELECT COLUMN_TYPE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER'
  AND COLUMN_NAME = 'STATUS'
LIMIT 1;

-- Step 3.2 execute ONLY when STATUS still includes 'registered' or default is not 'pending'.
ALTER TABLE TRADER
    MODIFY COLUMN STATUS ENUM('pending','active','suspended','decommissioned') NOT NULL DEFAULT 'pending';

-- Step 3.3 verification.
SELECT COLUMN_TYPE, COLUMN_DEFAULT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER'
  AND COLUMN_NAME = 'STATUS';

SELECT 'done: migration 002_tls13_autoreg_pending_hard_cutover' AS info;
