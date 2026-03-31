-- ============================================================================
-- CTS-Core Migration 003: Enforce Single Active Trader Session
-- ============================================================================
-- Created: 2026-03-30
-- Purpose:
--   1) Ensure at most one active session (ENDED_AT IS NULL) per TRADER_ID.
--   2) Make the invariant race-safe at DB level for concurrent registrations.
--
-- DBeaver line-by-line mode:
--   1) Execute checks first.
--   2) Execute DDL/DML only when corresponding check says it is required.
-- ============================================================================

USE ct_system;

SELECT 'start: migration 003_single_active_trader_session' AS info;

-- Step 1: Verify table exists.
SELECT COUNT(*) AS trader_session_table_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION';

-- Step 2: Run ONLY if trader_session_table_exists=1.
-- Keep only the most recent active session per trader before adding uniqueness.
UPDATE TRADER_SESSION ts
JOIN (
    SELECT id
    FROM (
        SELECT ID,
               ROW_NUMBER() OVER (PARTITION BY TRADER_ID ORDER BY CONNECTED_AT DESC, ID DESC) AS rn
        FROM TRADER_SESSION
        WHERE ENDED_AT IS NULL
    ) ranked
    WHERE ranked.rn > 1
) dup ON dup.id = ts.ID
SET ts.ENDED_AT = COALESCE(ts.LAST_HEARTBEAT, ts.CONNECTED_AT, NOW()),
    ts.DISCONNECT_REASON = 'kicked',
    ts.ERROR_MESSAGE = COALESCE(ts.ERROR_MESSAGE, 'migration_003_duplicate_active_session_closed')
WHERE ts.ENDED_AT IS NULL;

-- Step 3: Check whether generated column already exists.
SELECT COUNT(*) AS active_slot_column_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION'
  AND COLUMN_NAME = 'ACTIVE_SESSION_SLOT';

-- Step 4: Run ONLY if trader_session_table_exists=1 and active_slot_column_exists=0.
ALTER TABLE TRADER_SESSION
    ADD COLUMN ACTIVE_SESSION_SLOT TINYINT
    GENERATED ALWAYS AS (CASE WHEN ENDED_AT IS NULL THEN 1 ELSE NULL END) STORED;

-- Step 5: Check whether unique index already exists.
SELECT COUNT(*) AS single_active_index_exists
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION'
  AND INDEX_NAME = 'uk_trader_single_active_session';

-- Step 6: Run ONLY if trader_session_table_exists=1 and single_active_index_exists=0.
ALTER TABLE TRADER_SESSION
    ADD UNIQUE KEY uk_trader_single_active_session (TRADER_ID, ACTIVE_SESSION_SLOT);

-- Step 7: Verification queries.
SELECT COLUMN_NAME, EXTRA
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION'
  AND COLUMN_NAME = 'ACTIVE_SESSION_SLOT';

SELECT INDEX_NAME, NON_UNIQUE
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER_SESSION'
  AND INDEX_NAME = 'uk_trader_single_active_session';

SELECT 'done: migration 003_single_active_trader_session' AS info;
