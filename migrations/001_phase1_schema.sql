-- ============================================================================
-- CTS-Core Phase 1 Database Schema Migrations
-- ============================================================================
-- Created: 2026-01-28
-- Purpose: Implement all architectural decisions (see ARCHITECTURE.md section 2.2)
-- Database: ct_system (MySQL 9)
-- 
-- Tables:
--   0. ARBITRAGE_TRANS - ALTER to extend ID to BIGINT (future-proof)
--   1. TRADER - trader registration (admin pre-registration)
--   2. TRADER_SESSION - connection history (7 days retention)
--   3. MONITORING - ALTER to add assignment fields
--   4. TRADE - ALTER to add trader assignment
--   5. TRADER_EXCHANGE_RESOURCE - trader metrics (rate limits, load)
--   6. ARBITRAGE_ORDER - middle level (orders per exchange)
--   7. ARBITRAGE_ORDER_TRANS - bottom level (individual fills/partials)
--   8. AUDIT_LOG - audit trail (Phase 2, optional)
--   9. REENCRYPTION_JOBS - HSM key rotation jobs
--   10. REENCRYPTION_PROGRESS - per-record re-encryption tracking
--   11. SCHEDULER_TASKS - background jobs
--
-- Architecture Note: Rate limits managed by TRADERS autonomously
--   - Exchange rate limits are IP-bound (trader's IP, not Core's)
--   - Traders report metrics to Core via TRADER_EXCHANGE_RESOURCE
--   - Core uses metrics for load balancing, but traders make final decisions
-- ============================================================================

USE ct_system;

-- ============================================================================
-- 0. ARBITRAGE_TRANS: Extend ID to BIGINT
-- ============================================================================
-- Purpose: Future-proof for high-volume trading
-- INT max: 4.3 billion (insufficient for long-term high-frequency trading)
-- BIGINT max: 9.2 quintillion (sufficient for decades)

ALTER TABLE ARBITRAGE_TRANS 
MODIFY COLUMN ID BIGINT NOT NULL AUTO_INCREMENT;

-- ============================================================================
-- 1. TRADER: Trader Registration
-- ============================================================================
-- Purpose: Admin pre-registers traders before they connect
-- Lifecycle: Admin creates → Trader connects → STATUS changes to 'active'

CREATE TABLE IF NOT EXISTS TRADER (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    TRADER_NAME VARCHAR(100) NOT NULL COMMENT 'Human-readable name (e.g., EU Frankfurt Trader)',
    CERTIFICATE_CN VARCHAR(255) UNIQUE NOT NULL COMMENT 'mTLS certificate CN (e.g., trader-eu-1.cts.internal)',
    REGION VARCHAR(50) COMMENT 'Geographic region (eu, us, asia)',
    STATUS ENUM('registered', 'active', 'suspended', 'decommissioned') NOT NULL DEFAULT 'registered',
    MAX_TASKS INT NOT NULL DEFAULT 10 COMMENT 'Max concurrent tasks',
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATE_MODIFY TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    USER_CREATED INT COMMENT 'USER.ID who created',
    USER_MODIFY INT COMMENT 'USER.ID who last modified',
    NOTES TEXT COMMENT 'Admin notes',
    
    INDEX idx_status (STATUS),
    INDEX idx_region (REGION),
    INDEX idx_certificate (CERTIFICATE_CN),
    
    CONSTRAINT fk_trader_user_created FOREIGN KEY (USER_CREATED) 
        REFERENCES USER(ID) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_trader_user_modify FOREIGN KEY (USER_MODIFY) 
        REFERENCES USER(ID) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Trader pre-registration (admin-managed)';

-- ============================================================================
-- 2. TRADER_SESSION: Connection History
-- ============================================================================
-- Purpose: Track trader connections/disconnections
-- Retention: 7 days (cleanup job)
-- Used for: Audit, troubleshooting, failover

CREATE TABLE IF NOT EXISTS TRADER_SESSION (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    TRADER_ID INT NOT NULL COMMENT 'TRADER.ID',
    SESSION_ID VARCHAR(100) UNIQUE NOT NULL COMMENT 'UUID for this session',
    WS_CONNECTION_ID VARCHAR(100) COMMENT 'WebSocket connection ID',
    IP_ADDRESS VARCHAR(45) COMMENT 'Client IP (IPv4/IPv6)',
    CONNECTED_AT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    LAST_HEARTBEAT TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ENDED_AT TIMESTAMP NULL COMMENT 'NULL = still connected',
    DISCONNECT_REASON ENUM('graceful', 'timeout', 'error', 'server_shutdown', 'kicked') COMMENT 'Disconnect reason',
    ERROR_MESSAGE TEXT COMMENT 'Error details if DISCONNECT_REASON=error',
    INDEX idx_connected_at (CONNECTED_AT),
    INDEX idx_active_sessions (TRADER_ID, ENDED_AT),
    INDEX idx_cleanup (ENDED_AT) COMMENT 'For cleanup job (DELETE WHERE ENDED_AT < NOW() - 7 days)',
    CONSTRAINT fk_trader_session_trader FOREIGN KEY (TRADER_ID) 
        REFERENCES TRADER(ID) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Trader connection history (7 days retention)';

-- ============================================================================
-- 3. MONITORING: ALTER to add assignment fields
-- ============================================================================
-- Purpose: Add trader assignment tracking to existing MONITORING table
-- Note: Assumes MONITORING table already exists

ALTER TABLE MONITORING 
ADD COLUMN ASSIGNED_TRADER_ID INT DEFAULT NULL COMMENT 'TRADER.ID currently assigned',
ADD COLUMN ASSIGNED_AT TIMESTAMP NULL DEFAULT NULL COMMENT 'When task was assigned',
ADD COLUMN BACKUP_TRADER_ID INT DEFAULT NULL COMMENT 'Backup TRADER.ID (monitoring duplication)',
ADD INDEX idx_assigned_trader (ASSIGNED_TRADER_ID),
ADD INDEX idx_assignment (ASSIGNED_TRADER_ID, ASSIGNED_AT),
ADD CONSTRAINT fk_monitoring_assigned_trader 
    FOREIGN KEY (ASSIGNED_TRADER_ID) REFERENCES TRADER(ID) ON DELETE SET NULL ON UPDATE CASCADE,
ADD CONSTRAINT fk_monitoring_backup_trader 
    FOREIGN KEY (BACKUP_TRADER_ID) REFERENCES TRADER(ID) ON DELETE SET NULL ON UPDATE CASCADE;

-- ============================================================================
-- 3.1. TRADE: ALTER to add trader assignment
-- ============================================================================
-- Purpose: Track which trader is executing trade tasks
-- Note: Simpler than MONITORING (no backup trader needed for trades)

ALTER TABLE TRADE
ADD COLUMN TRADER_ID INT DEFAULT NULL COMMENT 'TRADER.ID executing this trade task',
ADD COLUMN ASSIGNED_AT TIMESTAMP NULL DEFAULT NULL COMMENT 'When trader was assigned',
ADD INDEX idx_trader (TRADER_ID),
ADD INDEX idx_assignment (TRADER_ID, ASSIGNED_AT),
ADD CONSTRAINT fk_trade_trader 
    FOREIGN KEY (TRADER_ID) REFERENCES TRADER(ID) ON DELETE SET NULL ON UPDATE CASCADE;

-- ============================================================================
-- 4. TRADER_EXCHANGE_RESOURCE: Trader Resource Usage (Metrics from Traders)
-- ============================================================================
-- Purpose: Track trader's current load and rate limit status
-- Architecture Decision: Rate limits managed by TRADERS, not Core
--   - IP-level limits: Trader IP → EXCHANGE (e.g., Binance 1200 req/min per IP)
--   - Account-level limits: Trader IP → EXCHANGE_ACCOUNT (e.g., Binance VIP0 vs VIP9 order limits)
--   - Trader autonomously tracks both types from exchange API headers
--   - Trader periodically reports metrics to Core (this table)
--   - Core uses metrics for smart task distribution (avoid overloaded traders)
--   - Trader can reject tasks if rate limit exceeded
-- Updated by: Trader (WebSocket metrics reports every 10-30 seconds)
-- Used by: Scheduler (load balancing, avoid overloaded traders)

CREATE TABLE IF NOT EXISTS TRADER_EXCHANGE_RESOURCE (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    TRADER_ID INT NOT NULL COMMENT 'TRADER.ID',
    EXCHANGE_ID INT NOT NULL COMMENT 'EXCHANGE.ID',
    EXCHANGE_ACCOUNT_ID INT DEFAULT NULL COMMENT 'EXCHANGE_ACCOUNTS.ID (NULL = IP-level limit, NOT NULL = account-level limit)',
    RESOURCE_TYPE ENUM('api_requests_minute', 'api_weight_minute', 'orders_minute', 'websocket_connections') NOT NULL,
    USED_VALUE DECIMAL(20, 8) NOT NULL DEFAULT 0 COMMENT 'Current usage reported by trader',
    MAX_VALUE DECIMAL(20, 8) NOT NULL COMMENT 'Limit from exchange (trader discovers via API)',
    LAST_UPDATED TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    RESET_AT TIMESTAMP NOT NULL COMMENT 'When USED_VALUE resets (calculated by trader)',
    
    UNIQUE KEY uk_resource (TRADER_ID, EXCHANGE_ID, EXCHANGE_ACCOUNT_ID, RESOURCE_TYPE),
    INDEX idx_trader (TRADER_ID),
    INDEX idx_exchange (EXCHANGE_ID),
    INDEX idx_account (EXCHANGE_ACCOUNT_ID),
    INDEX idx_availability (TRADER_ID, EXCHANGE_ID, RESOURCE_TYPE, USED_VALUE) COMMENT 'For scoring',
    
    CONSTRAINT fk_trader_resource_trader FOREIGN KEY (TRADER_ID) 
        REFERENCES TRADER(ID) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_trader_resource_exchange FOREIGN KEY (EXCHANGE_ID) 
        REFERENCES EXCHANGE(ID) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_trader_resource_account FOREIGN KEY (EXCHANGE_ACCOUNT_ID) 
        REFERENCES EXCHANGE_ACCOUNTS(ID) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Trader resource usage tracking (IP-level + account-level limits)';

-- ============================================================================
-- 6. ARBITRAGE_ORDER: Middle Level (Orders per Exchange)
-- ============================================================================
-- Purpose: Track individual orders on each exchange within an arbitrage deal
-- Hierarchy: ARBITRAGE_TRANS (top) → ARBITRAGE_ORDER (middle) → ORDER_TRANSACTION (bottom)
-- Idempotency: UNIQUE(arbitrage_trans_id, exchange_order_id)

CREATE TABLE IF NOT EXISTS ARBITRAGE_ORDER (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    ARBITRAGE_TRANS_ID BIGINT NOT NULL COMMENT 'ARBITRAGE_TRANS.ID (parent)',
    EXCHANGE_ID INT NOT NULL COMMENT 'EXCHANGE.ID',
    EXCHANGE_ACCOUNT_ID INT NOT NULL COMMENT 'EXCHANGE_ACCOUNTS.ID',
    EXCHANGE_ORDER_ID VARCHAR(255) NOT NULL COMMENT 'Order ID from exchange',
    TRADER_ID INT NOT NULL COMMENT 'TRADER.ID who executed',
    
    SIDE ENUM('buy', 'sell') NOT NULL,
    ORDER_TYPE ENUM('market', 'limit', 'stop_limit') NOT NULL,
    PAIR_ID INT NOT NULL COMMENT 'TRADE_PAIR.ID (BASE_CURRENCY + QUOTE_CURRENCY + EXCHANGE)',
    
    REQUESTED_QUANTITY DECIMAL(30, 12) NOT NULL,
    FILLED_QUANTITY DECIMAL(30, 12) NOT NULL DEFAULT 0,
    AVG_PRICE DECIMAL(30, 12) COMMENT 'Average fill price',
    TOTAL_COST DECIMAL(30, 12) COMMENT 'Total cost in quote currency',
    TOTAL_FEE DECIMAL(30, 12) NOT NULL DEFAULT 0,
    FEE_CURRENCY VARCHAR(20),
    
    STATUS ENUM('pending', 'partial', 'filled', 'cancelled', 'rejected', 'error') NOT NULL DEFAULT 'pending',
    ERROR_MESSAGE TEXT COMMENT 'Error details if STATUS=error',
    
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FILLED_AT TIMESTAMP NULL COMMENT 'When fully filled',
    
    UNIQUE KEY uk_exchange_order (ARBITRAGE_TRANS_ID, EXCHANGE_ORDER_ID) COMMENT 'Deduplication',
    INDEX idx_exchange (EXCHANGE_ID, STATUS),
    INDEX idx_status (STATUS),
    INDEX idx_created (DATE_CREATE),
    
    CONSTRAINT fk_arbitrage_order_trans FOREIGN KEY (ARBITRAGE_TRANS_ID) 
        REFERENCES ARBITRAGE_TRANS(ID) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_arbitrage_order_exchange FOREIGN KEY (EXCHANGE_ID) 
        REFERENCES EXCHANGE(ID) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_arbitrage_order_account FOREIGN KEY (EXCHANGE_ACCOUNT_ID) 
        REFERENCES EXCHANGE_ACCOUNTS(ID) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_arbitrage_order_pair FOREIGN KEY (PAIR_ID) 
        REFERENCES TRADE_PAIR(ID) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_arbitrage_order_trader FOREIGN KEY (TRADER_ID) 
        REFERENCES TRADER(ID) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Orders per exchange (middle level of arbitrage)';

-- ============================================================================
-- 7. ARBITRAGE_ORDER_TRANS: Bottom Level (Individual Fills/Partials)
-- ============================================================================
-- Purpose: Track individual fills/partial executions within an order
-- Used for: Detailed execution analysis, fee tracking, audit
-- Idempotency: UNIQUE(arbitrage_order_id, exchange_transaction_id)

CREATE TABLE IF NOT EXISTS ARBITRAGE_ORDER_TRANS (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    ARBITRAGE_ORDER_ID BIGINT NOT NULL COMMENT 'ARBITRAGE_ORDER.ID (parent)',
    EXCHANGE_TRANSACTION_ID VARCHAR(255) NOT NULL COMMENT 'Transaction/fill ID from exchange',
    
    QUANTITY DECIMAL(30, 12) NOT NULL COMMENT 'Filled quantity in this transaction',
    PRICE DECIMAL(30, 12) NOT NULL COMMENT 'Fill price',
    COST DECIMAL(30, 12) NOT NULL COMMENT 'QUANTITY * PRICE',
    FEE DECIMAL(30, 12) NOT NULL DEFAULT 0,
    FEE_CURRENCY VARCHAR(20),
    
    TIMESTAMP TIMESTAMP NOT NULL COMMENT 'Exchange execution timestamp',
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_exchange_tx (ARBITRAGE_ORDER_ID, EXCHANGE_TRANSACTION_ID) COMMENT 'Deduplication',
    FOREIGN KEY (ARBITRAGE_ORDER_ID) REFERENCES ARBITRAGE_ORDER(ID) ON DELETE CASCADE,
    INDEX idx_timestamp (TIMESTAMP)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Individual fills/partials within ARBITRAGE_ORDER (bottom level)';

-- ============================================================================
-- 8. AUDIT_LOG: Audit Trail (Optional - Phase 2)
-- ============================================================================
-- Purpose: Track admin operations for compliance
-- Primary storage: JSON file (logs/audit.log)
-- Secondary storage: MySQL (for UI/queries in Phase 2)
-- Retention: 7 days in MySQL, 30 days in file

CREATE TABLE IF NOT EXISTS AUDIT_LOG (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    TIMESTAMP TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UID INT COMMENT 'USER.ID who performed action',
    ACTION VARCHAR(100) NOT NULL COMMENT 'e.g., TRADER_DELETE, CONFIG_UPDATE',
    RESOURCE_TYPE VARCHAR(50) COMMENT 'e.g., trader, config, monitor',
    RESOURCE_ID VARCHAR(255) COMMENT 'e.g., trader-eu-1',
    OLD_VALUE JSON COMMENT 'State before change',
    NEW_VALUE JSON COMMENT 'State after change',
    IP_ADDRESS VARCHAR(45),
    USER_AGENT TEXT COMMENT 'HTTP User-Agent header (browser/client info)',
    SUCCESS BOOLEAN NOT NULL DEFAULT TRUE,
    ERROR_MESSAGE TEXT COMMENT 'If SUCCESS=FALSE',
    
    INDEX idx_user (UID),
    INDEX idx_timestamp (TIMESTAMP) COMMENT 'For queries by time and cleanup job',
    INDEX idx_action (ACTION),
    INDEX idx_resource (RESOURCE_TYPE, RESOURCE_ID),
    
    CONSTRAINT fk_audit_log_user FOREIGN KEY (UID) 
        REFERENCES USER(ID) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Audit trail (Phase 2, primary storage is JSON file)';

-- ============================================================================
-- 9. HSM KEY ROTATION SUPPORT
-- ============================================================================
-- Purpose: Support HSM key rotation with re-encryption tracking
-- hsm-service has key rotation capability, need to track versions in DB

-- 9.1 Fix USER_2FA: Add key version tracking
-- ============================================================================
-- CRITICAL: USER_2FA missing ENC_KEY_VERSION (unlike EXCHANGE_ACCOUNTS)
-- Without version, can't re-encrypt after key rotation!
-- Note: enc_alg NOT stored - HSM API doesn't use it (always AES-256-GCM, embedded in key_id)

ALTER TABLE USER_2FA
ADD COLUMN ENC_KEY_VERSION INT DEFAULT NULL COMMENT 'HSM KEK version from KEY_ID (e.g., kek-2fa-v1 → 1)',
ADD COLUMN NEEDS_REENCRYPTION BOOLEAN DEFAULT FALSE COMMENT 'Flag for re-encryption job',
ADD INDEX idx_reencryption (NEEDS_REENCRYPTION, ENC_KEY_VERSION) COMMENT 'For re-encryption job';

-- Set current version for existing records (if any)
-- UPDATE USER_2FA SET ENC_KEY_VERSION = 1 WHERE ENC_KEY_VERSION IS NULL AND SECRET_ENC IS NOT NULL;

-- 9.2 Re-encryption tracking table
-- ============================================================================
-- Purpose: Track re-encryption jobs after HSM key rotation
-- Scheduler uses this to manage re-encryption process

CREATE TABLE IF NOT EXISTS REENCRYPTION_JOBS (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    JOB_TYPE ENUM('user_2fa', 'exchange_accounts', 'other') NOT NULL,
    OLD_KEY_VERSION INT NOT NULL COMMENT 'KEK version to migrate FROM',
    NEW_KEY_VERSION INT NOT NULL COMMENT 'KEK version to migrate TO',
    CONTEXT VARCHAR(50) NOT NULL COMMENT 'HSM context (2fa, exchange-key)',
    
    STATUS ENUM('pending', 'in_progress', 'completed', 'failed', 'cancelled') NOT NULL DEFAULT 'pending',
    
    TOTAL_RECORDS INT NOT NULL DEFAULT 0 COMMENT 'Total records to re-encrypt',
    PROCESSED_RECORDS INT NOT NULL DEFAULT 0 COMMENT 'Records already processed',
    FAILED_RECORDS INT NOT NULL DEFAULT 0 COMMENT 'Failed records',
    
    BATCH_SIZE INT NOT NULL DEFAULT 100 COMMENT 'Records per batch',
    STARTED_AT TIMESTAMP NULL,
    COMPLETED_AT TIMESTAMP NULL,
    LAST_PROCESSED_AT TIMESTAMP NULL,
    ERROR_MESSAGE TEXT,
    
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_status (STATUS),
    INDEX idx_job_type (JOB_TYPE, STATUS),
    INDEX idx_processing (STATUS, LAST_PROCESSED_AT) COMMENT 'For scheduler pickup'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='HSM key rotation re-encryption jobs';

-- 9.3 Re-encryption progress tracking (per-record)
-- ============================================================================
-- Purpose: Track which specific records have been re-encrypted
-- Allows retry of failed records without re-processing successful ones

CREATE TABLE IF NOT EXISTS REENCRYPTION_PROGRESS (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    JOB_ID INT NOT NULL COMMENT 'REENCRYPTION_JOBS.ID',
    TABLE_NAME VARCHAR(100) NOT NULL COMMENT 'e.g., USER_2FA, EXCHANGE_ACCOUNTS',
    RECORD_ID VARCHAR(255) NOT NULL COMMENT 'Primary key of the record',
    
    STATUS ENUM('pending', 'completed', 'failed', 'skipped') NOT NULL DEFAULT 'pending',
    ATTEMPT_COUNT INT NOT NULL DEFAULT 0,
    ERROR_MESSAGE TEXT,
    
    PROCESSED_AT TIMESTAMP NULL,
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_job_record (JOB_ID, TABLE_NAME, RECORD_ID),
    FOREIGN KEY (JOB_ID) REFERENCES REENCRYPTION_JOBS(ID) ON DELETE CASCADE,
    INDEX idx_job (JOB_ID, STATUS),
    INDEX idx_pending (JOB_ID, STATUS, ID) COMMENT 'For batch processing'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Per-record re-encryption progress';

-- ============================================================================
-- 10. SCHEDULER TASKS TABLE
-- ============================================================================
-- Purpose: Track scheduled background jobs (cleanup, re-encryption, etc.)
-- CTS-Core scheduler uses this to manage periodic tasks

CREATE TABLE IF NOT EXISTS SCHEDULER_TASKS (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    TASK_NAME VARCHAR(100) UNIQUE NOT NULL COMMENT 'e.g., cleanup_trader_sessions, reencrypt_2fa',
    TASK_TYPE ENUM('cleanup', 'reencryption', 'monitoring', 'maintenance', 'other') NOT NULL,
    SCHEDULE_CRON VARCHAR(100) COMMENT 'Cron expression (e.g., "0 0 * * *" for daily)',
    SCHEDULE_INTERVAL_SEC INT COMMENT 'Interval in seconds (alternative to cron)',
    
    ENABLED BOOLEAN NOT NULL DEFAULT TRUE,
    STATUS ENUM('idle', 'running', 'failed', 'disabled') NOT NULL DEFAULT 'idle',
    
    LAST_RUN_AT TIMESTAMP NULL,
    LAST_RUN_DURATION_MS INT COMMENT 'Duration of last run in milliseconds',
    LAST_RUN_STATUS ENUM('success', 'error', 'timeout') NULL,
    LAST_ERROR TEXT COMMENT 'Error message from last failed run',
    
    NEXT_RUN_AT TIMESTAMP NULL COMMENT 'Calculated next run time',
    RUN_COUNT INT NOT NULL DEFAULT 0,
    ERROR_COUNT INT NOT NULL DEFAULT 0,
    
    CONFIG JSON COMMENT 'Task-specific configuration',
    
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATE_MODIFY TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    USER_CREATED INT COMMENT 'USER.ID who created the task',
    USER_MODIFY INT COMMENT 'USER.ID who last modified',
    
    INDEX idx_enabled (ENABLED, NEXT_RUN_AT),
    INDEX idx_status (STATUS),
    INDEX idx_type (TASK_TYPE),
    
    CONSTRAINT fk_scheduler_user_created FOREIGN KEY (USER_CREATED) 
        REFERENCES USER(ID) ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_scheduler_user_modify FOREIGN KEY (USER_MODIFY) 
        REFERENCES USER(ID) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Scheduled background tasks for CTS-Core';

-- Insert default scheduled tasks
INSERT INTO SCHEDULER_TASKS (TASK_NAME, TASK_TYPE, SCHEDULE_CRON, ENABLED, CONFIG) VALUES
    ('cleanup_trader_sessions', 'cleanup', '0 2 * * *', TRUE, '{"retention_days": 7}'),
    ('cleanup_audit_logs', 'cleanup', '0 3 * * *', TRUE, '{"retention_days": 180}'),
    ('reset_daily_limits', 'maintenance', '0 0 * * *', TRUE, '{}'),
    ('check_reencryption_jobs', 'reencryption', NULL, TRUE, '{"check_interval_sec": 60}')
ON DUPLICATE KEY UPDATE DATE_MODIFY = CURRENT_TIMESTAMP;

-- ============================================================================
-- Sample Data (Optional - for testing)
-- ============================================================================

-- Insert sample trader
-- INSERT INTO TRADER (TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS)
-- VALUES 
--     ('EU Frankfurt Trader', 'trader-eu-1.cts.internal', 'eu', 'registered', 10),
--     ('US New York Trader', 'trader-us-1.cts.internal', 'us', 'registered', 10);

-- ============================================================================
-- Cleanup Jobs (Should be scheduled via cron or application)
-- ============================================================================

-- Cleanup old trader sessions (older than 7 days)
-- DELETE FROM TRADER_SESSION WHERE ENDED_AT IS NOT NULL AND ENDED_AT < DATE_SUB(NOW(), INTERVAL 7 DAY);

-- Cleanup old audit logs (older than 7 days)
-- DELETE FROM AUDIT_LOG WHERE TIMESTAMP < DATE_SUB(NOW(), INTERVAL 7 DAY);

-- Reset daily limits (run daily at midnight)
-- UPDATE EXCHANGE_LIMITS 
-- SET CURRENT_VALUE = 0, RESET_AT = DATE_ADD(CURDATE(), INTERVAL 1 DAY)
-- WHERE LIMIT_TYPE IN ('orders_per_day', 'volume_per_day') AND RESET_AT < NOW();

-- ============================================================================
-- End of Migration
-- ============================================================================
