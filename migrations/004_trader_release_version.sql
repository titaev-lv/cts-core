-- ============================================================================
-- CTS-Core Migration 004: Add trader release version tracking
-- ============================================================================
-- Created: 2026-03-31
-- Purpose:
--   Store latest trader build release reported during trader.register
--   to identify outdated trader deployments.
--
-- DBeaver line-by-line mode:
--   1) Execute each statement sequentially.
--   2) Run ALTER only if pre-check says column is missing.
-- ============================================================================

USE ct_system;

SELECT 'start: migration 004_trader_release_version' AS info;

-- Step 1: Ensure TRADER table exists.
SELECT COUNT(*) AS trader_table_exists
FROM INFORMATION_SCHEMA.TABLES
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER';

-- Step 2: Check whether RELEASE_VERSION already exists.
SELECT COUNT(*) AS release_column_exists
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER'
  AND COLUMN_NAME = 'RELEASE_VERSION';

-- Step 3: Run ONLY if trader_table_exists=1 and release_column_exists=0.
ALTER TABLE TRADER
    ADD COLUMN RELEASE_VERSION VARCHAR(64) NULL
    COMMENT 'Latest trader release reported on register';

-- Step 4: Verify column was added.
SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, CHARACTER_MAXIMUM_LENGTH
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'TRADER'
  AND COLUMN_NAME = 'RELEASE_VERSION';

SELECT 'done: migration 004_trader_release_version' AS info;
