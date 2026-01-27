-- ============================================================================
-- CTS-Core Phase 1 Database Schema Migrations
-- ============================================================================
-- Created: 2026-01-28
-- Purpose: Implement all architectural decisions from BEFORE_START.md
-- Database: ct_system (MySQL 9)
-- 
-- Tables:
--   1. TRADER - trader registration (admin pre-registration)
--   2. TRADER_SESSION - connection history (7 days retention)
--   3. MONITORING - ALTER to add assignment fields
--   4. EXCHANGE_LIMITS - exchange rate limits (orders/volume)
--   5. TRADER_EXCHANGE_RESOURCE - trader resource usage tracking
--   6. ARBITRAGE_ORDER - middle level (orders per exchange)
--   7. ORDER_TRANSACTION - bottom level (individual fills/partials)
--   8. AUDIT_LOG - audit trail (Phase 2, optional)
-- ============================================================================

USE ct_system;

-- ============================================================================
-- 1. TRADER: Trader Registration
-- ============================================================================
-- Purpose: Admin pre-registers traders before they connect
-- Lifecycle: Admin creates → Trader connects → STATUS changes to 'active'

CREATE TABLE IF NOT EXISTS TRADER (
    id INT PRIMARY KEY AUTO_INCREMENT,
    trader_id VARCHAR(100) UNIQUE NOT NULL COMMENT 'Unique trader identifier (e.g., trader-eu-1)',
    name VARCHAR(255) NOT NULL COMMENT 'Human-readable name',
    certificate_cn VARCHAR(255) NOT NULL COMMENT 'mTLS certificate CN',
    region VARCHAR(50) COMMENT 'Geographic region (eu, us, asia)',
    status ENUM('registered', 'active', 'suspended', 'decommissioned') NOT NULL DEFAULT 'registered',
    max_tasks INT NOT NULL DEFAULT 10 COMMENT 'Max concurrent tasks',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    created_by INT COMMENT 'USER.ID who created',
    notes TEXT COMMENT 'Admin notes',
    
    INDEX idx_status (status),
    INDEX idx_region (region),
    INDEX idx_certificate (certificate_cn)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Trader pre-registration (admin-managed)';

-- ============================================================================
-- 2. TRADER_SESSION: Connection History
-- ============================================================================
-- Purpose: Track trader connections/disconnections
-- Retention: 7 days (cleanup job)
-- Used for: Audit, troubleshooting, failover

CREATE TABLE IF NOT EXISTS TRADER_SESSION (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    trader_id VARCHAR(100) NOT NULL,
    session_id VARCHAR(100) UNIQUE NOT NULL COMMENT 'UUID for this session',
    ws_connection_id VARCHAR(255) COMMENT 'WebSocket connection ID',
    ip_address VARCHAR(45) COMMENT 'Client IP (IPv4/IPv6)',
    connected_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_heartbeat TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP NULL COMMENT 'NULL = still connected',
    disconnect_reason ENUM('graceful', 'timeout', 'error', 'server_shutdown', 'kicked') COMMENT 'Disconnect reason',
    error_message TEXT COMMENT 'Error details if disconnect_reason=error',
    
    FOREIGN KEY (trader_id) REFERENCES TRADER(trader_id) ON DELETE CASCADE,
    INDEX idx_trader (trader_id),
    INDEX idx_connected_at (connected_at),
    INDEX idx_active_sessions (trader_id, ended_at),
    INDEX idx_cleanup (ended_at) COMMENT 'For cleanup job (DELETE WHERE ended_at < NOW() - 7 days)'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Trader connection history (7 days retention)';

-- ============================================================================
-- 3. MONITORING: ALTER to add assignment fields
-- ============================================================================
-- Purpose: Add trader assignment tracking to existing MONITORING table
-- Note: Assumes MONITORING table already exists

ALTER TABLE MONITORING 
ADD COLUMN IF NOT EXISTS assigned_trader_id VARCHAR(100) COMMENT 'Currently assigned trader',
ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMP NULL COMMENT 'When task was assigned',
ADD COLUMN IF NOT EXISTS backup_trader_id VARCHAR(100) COMMENT 'Backup trader (monitoring duplication)';

ALTER TABLE MONITORING
ADD INDEX IF NOT EXISTS idx_assigned_trader (assigned_trader_id),
ADD INDEX IF NOT EXISTS idx_assignment (assigned_trader_id, assigned_at);

-- Add foreign key if TRADER table exists
ALTER TABLE MONITORING
ADD CONSTRAINT fk_monitoring_trader 
FOREIGN KEY IF NOT EXISTS (assigned_trader_id) REFERENCES TRADER(trader_id) ON DELETE SET NULL;

-- ============================================================================
-- 4. EXCHANGE_LIMITS: Exchange Rate Limits
-- ============================================================================
-- Purpose: Define per-exchange rate limits (orders/volume per day)
-- Managed by: Admin (manual updates based on exchange documentation)
-- Used by: Scheduler (resource availability scoring)

CREATE TABLE IF NOT EXISTS EXCHANGE_LIMITS (
    id INT PRIMARY KEY AUTO_INCREMENT,
    exchange_id INT NOT NULL COMMENT 'EXCHANGE.ID',
    exchange_account_id INT NOT NULL COMMENT 'EXCHANGE_ACCOUNTS.ID',
    limit_type ENUM('orders_per_day', 'volume_per_day', 'orders_per_minute', 'api_requests_per_minute') NOT NULL,
    max_value DECIMAL(20, 8) NOT NULL COMMENT 'Maximum allowed value',
    current_value DECIMAL(20, 8) NOT NULL DEFAULT 0 COMMENT 'Current usage (resets daily/minutely)',
    reset_at TIMESTAMP NOT NULL COMMENT 'When current_value resets',
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    notes TEXT COMMENT 'Admin notes about limit',
    
    UNIQUE KEY uk_limit (exchange_account_id, limit_type),
    INDEX idx_exchange (exchange_id),
    INDEX idx_account (exchange_account_id),
    INDEX idx_reset (reset_at) COMMENT 'For cleanup/reset job'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Exchange rate limits (orders/volume per account)';

-- ============================================================================
-- 5. TRADER_EXCHANGE_RESOURCE: Trader Resource Usage
-- ============================================================================
-- Purpose: Track trader's usage of exchange resources
-- Updated by: Trader (reports via metrics), CTS-Core (aggregates)
-- Used by: Scheduler (resource availability scoring 20%)

CREATE TABLE IF NOT EXISTS TRADER_EXCHANGE_RESOURCE (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    trader_id VARCHAR(100) NOT NULL,
    exchange_id INT NOT NULL COMMENT 'EXCHANGE.ID',
    resource_type ENUM('orders_today', 'volume_today', 'api_requests_minute', 'websocket_connections') NOT NULL,
    used_value DECIMAL(20, 8) NOT NULL DEFAULT 0,
    max_value DECIMAL(20, 8) NOT NULL COMMENT 'Limit for this trader (from EXCHANGE_LIMITS or config)',
    last_updated TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    reset_at TIMESTAMP NOT NULL COMMENT 'When used_value resets',
    
    UNIQUE KEY uk_resource (trader_id, exchange_id, resource_type),
    FOREIGN KEY (trader_id) REFERENCES TRADER(trader_id) ON DELETE CASCADE,
    INDEX idx_trader (trader_id),
    INDEX idx_exchange (exchange_id),
    INDEX idx_availability (trader_id, exchange_id, resource_type, used_value) COMMENT 'For scoring'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Trader resource usage tracking (for load balancing)';

-- ============================================================================
-- 6. ARBITRAGE_ORDER: Middle Level (Orders per Exchange)
-- ============================================================================
-- Purpose: Track individual orders on each exchange within an arbitrage deal
-- Hierarchy: ARBITRAGE_TRANS (top) → ARBITRAGE_ORDER (middle) → ORDER_TRANSACTION (bottom)
-- Idempotency: UNIQUE(arbitrage_trans_id, exchange_order_id)

CREATE TABLE IF NOT EXISTS ARBITRAGE_ORDER (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    arbitrage_trans_id BIGINT NOT NULL COMMENT 'ARBITRAGE_TRANS.ID (parent)',
    exchange_id INT NOT NULL COMMENT 'EXCHANGE.ID',
    exchange_account_id INT NOT NULL COMMENT 'EXCHANGE_ACCOUNTS.ID',
    exchange_order_id VARCHAR(255) NOT NULL COMMENT 'Order ID from exchange',
    trader_id VARCHAR(100) NOT NULL COMMENT 'Trader who executed',
    
    side ENUM('buy', 'sell') NOT NULL,
    order_type ENUM('market', 'limit', 'stop_limit') NOT NULL,
    pair VARCHAR(50) NOT NULL COMMENT 'e.g., BTC/USDT',
    
    requested_quantity DECIMAL(20, 8) NOT NULL,
    filled_quantity DECIMAL(20, 8) NOT NULL DEFAULT 0,
    avg_price DECIMAL(20, 8) COMMENT 'Average fill price',
    total_cost DECIMAL(20, 8) COMMENT 'Total cost in quote currency',
    total_fee DECIMAL(20, 8) NOT NULL DEFAULT 0,
    fee_currency VARCHAR(20),
    
    status ENUM('pending', 'partial', 'filled', 'cancelled', 'rejected', 'error') NOT NULL DEFAULT 'pending',
    error_message TEXT COMMENT 'Error details if status=error',
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    filled_at TIMESTAMP NULL COMMENT 'When fully filled',
    
    UNIQUE KEY uk_exchange_order (arbitrage_trans_id, exchange_order_id) COMMENT 'Deduplication',
    FOREIGN KEY (arbitrage_trans_id) REFERENCES ARBITRAGE_TRANS(ID) ON DELETE CASCADE,
    FOREIGN KEY (trader_id) REFERENCES TRADER(trader_id),
    INDEX idx_arbitrage (arbitrage_trans_id),
    INDEX idx_exchange (exchange_id),
    INDEX idx_trader (trader_id),
    INDEX idx_status (status),
    INDEX idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Orders per exchange (middle level of arbitrage)';

-- ============================================================================
-- 7. ORDER_TRANSACTION: Bottom Level (Individual Fills/Partials)
-- ============================================================================
-- Purpose: Track individual fills/partial executions within an order
-- Used for: Detailed execution analysis, fee tracking, audit
-- Idempotency: UNIQUE(arbitrage_order_id, exchange_transaction_id)

CREATE TABLE IF NOT EXISTS ORDER_TRANSACTION (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    arbitrage_order_id BIGINT NOT NULL COMMENT 'ARBITRAGE_ORDER.ID (parent)',
    exchange_transaction_id VARCHAR(255) NOT NULL COMMENT 'Transaction/fill ID from exchange',
    
    quantity DECIMAL(20, 8) NOT NULL COMMENT 'Filled quantity in this transaction',
    price DECIMAL(20, 8) NOT NULL COMMENT 'Fill price',
    cost DECIMAL(20, 8) NOT NULL COMMENT 'quantity * price',
    fee DECIMAL(20, 8) NOT NULL DEFAULT 0,
    fee_currency VARCHAR(20),
    
    timestamp TIMESTAMP NOT NULL COMMENT 'Exchange execution timestamp',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_exchange_tx (arbitrage_order_id, exchange_transaction_id) COMMENT 'Deduplication',
    FOREIGN KEY (arbitrage_order_id) REFERENCES ARBITRAGE_ORDER(id) ON DELETE CASCADE,
    INDEX idx_order (arbitrage_order_id),
    INDEX idx_timestamp (timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Individual fills/partials (bottom level of arbitrage)';

-- ============================================================================
-- 8. AUDIT_LOG: Audit Trail (Optional - Phase 2)
-- ============================================================================
-- Purpose: Track admin operations for compliance
-- Primary storage: JSON file (logs/audit.log)
-- Secondary storage: MySQL (for UI/queries in Phase 2)
-- Retention: 7 days in MySQL, 30 days in file

CREATE TABLE IF NOT EXISTS AUDIT_LOG (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    timestamp TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    user_id INT COMMENT 'USER.ID who performed action',
    username VARCHAR(100),
    action VARCHAR(100) NOT NULL COMMENT 'e.g., TRADER_DELETE, CONFIG_UPDATE',
    resource_type VARCHAR(50) COMMENT 'e.g., trader, config, monitor',
    resource_id VARCHAR(255) COMMENT 'e.g., trader-eu-1',
    old_value JSON COMMENT 'State before change',
    new_value JSON COMMENT 'State after change',
    ip_address VARCHAR(45),
    user_agent TEXT,
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT COMMENT 'If success=FALSE',
    
    INDEX idx_user (user_id),
    INDEX idx_timestamp (timestamp),
    INDEX idx_action (action),
    INDEX idx_resource (resource_type, resource_id),
    INDEX idx_cleanup (timestamp) COMMENT 'For auto-cleanup (7 days)'
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
ADD COLUMN IF NOT EXISTS enc_key_version INT COMMENT 'HSM KEK version from key_id (e.g., kek-2fa-v1 → 1)',
ADD COLUMN IF NOT EXISTS needs_reencryption BOOLEAN DEFAULT FALSE COMMENT 'Flag for re-encryption job';

ALTER TABLE USER_2FA
ADD INDEX IF NOT EXISTS idx_reencryption (needs_reencryption, enc_key_version) COMMENT 'For re-encryption job';

-- Set current version for existing records (if any)
-- UPDATE USER_2FA SET enc_key_version = 1 WHERE enc_key_version IS NULL AND SECRET_ENC IS NOT NULL;

-- 9.2 Re-encryption tracking table
-- ============================================================================
-- Purpose: Track re-encryption jobs after HSM key rotation
-- Scheduler uses this to manage re-encryption process

CREATE TABLE IF NOT EXISTS REENCRYPTION_JOBS (
    id INT PRIMARY KEY AUTO_INCREMENT,
    job_type ENUM('user_2fa', 'exchange_accounts', 'other') NOT NULL,
    old_key_version INT NOT NULL COMMENT 'KEK version to migrate FROM',
    new_key_version INT NOT NULL COMMENT 'KEK version to migrate TO',
    context VARCHAR(50) NOT NULL COMMENT 'HSM context (2fa, exchange-key)',
    
    status ENUM('pending', 'in_progress', 'completed', 'failed', 'cancelled') NOT NULL DEFAULT 'pending',
    
    total_records INT NOT NULL DEFAULT 0 COMMENT 'Total records to re-encrypt',
    processed_records INT NOT NULL DEFAULT 0 COMMENT 'Records already processed',
    failed_records INT NOT NULL DEFAULT 0 COMMENT 'Failed records',
    
    batch_size INT NOT NULL DEFAULT 100 COMMENT 'Records per batch',
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    last_processed_at TIMESTAMP NULL,
    error_message TEXT,
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by INT COMMENT 'USER.ID who initiated',
    
    INDEX idx_status (status),
    INDEX idx_job_type (job_type, status),
    INDEX idx_processing (status, last_processed_at) COMMENT 'For scheduler pickup'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='HSM key rotation re-encryption jobs';

-- 9.3 Re-encryption progress tracking (per-record)
-- ============================================================================
-- Purpose: Track which specific records have been re-encrypted
-- Allows retry of failed records without re-processing successful ones

CREATE TABLE IF NOT EXISTS REENCRYPTION_PROGRESS (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    job_id INT NOT NULL COMMENT 'REENCRYPTION_JOBS.ID',
    table_name VARCHAR(100) NOT NULL COMMENT 'e.g., USER_2FA, EXCHANGE_ACCOUNTS',
    record_id VARCHAR(255) NOT NULL COMMENT 'Primary key of the record',
    
    status ENUM('pending', 'completed', 'failed', 'skipped') NOT NULL DEFAULT 'pending',
    attempt_count INT NOT NULL DEFAULT 0,
    error_message TEXT,
    
    processed_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE KEY uk_job_record (job_id, table_name, record_id),
    FOREIGN KEY (job_id) REFERENCES REENCRYPTION_JOBS(id) ON DELETE CASCADE,
    INDEX idx_job (job_id, status),
    INDEX idx_pending (job_id, status, id) COMMENT 'For batch processing'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Per-record re-encryption progress';

-- ============================================================================
-- 10. SCHEDULER TASKS TABLE
-- ============================================================================
-- Purpose: Track scheduled background jobs (cleanup, re-encryption, etc.)
-- CTS-Core scheduler uses this to manage periodic tasks

CREATE TABLE IF NOT EXISTS SCHEDULER_TASKS (
    id INT PRIMARY KEY AUTO_INCREMENT,
    task_name VARCHAR(100) UNIQUE NOT NULL COMMENT 'e.g., cleanup_trader_sessions, reencrypt_2fa',
    task_type ENUM('cleanup', 'reencryption', 'monitoring', 'maintenance', 'other') NOT NULL,
    schedule_cron VARCHAR(100) COMMENT 'Cron expression (e.g., "0 0 * * *" for daily)',
    schedule_interval_sec INT COMMENT 'Interval in seconds (alternative to cron)',
    
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    status ENUM('idle', 'running', 'failed', 'disabled') NOT NULL DEFAULT 'idle',
    
    last_run_at TIMESTAMP NULL,
    last_run_duration_ms INT COMMENT 'Duration of last run in milliseconds',
    last_run_status ENUM('success', 'error', 'timeout') NULL,
    last_error TEXT COMMENT 'Error message from last failed run',
    
    next_run_at TIMESTAMP NULL COMMENT 'Calculated next run time',
    run_count INT NOT NULL DEFAULT 0,
    error_count INT NOT NULL DEFAULT 0,
    
    config JSON COMMENT 'Task-specific configuration',
    
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    INDEX idx_enabled (enabled, next_run_at),
    INDEX idx_status (status),
    INDEX idx_type (task_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Scheduled background tasks for CTS-Core';

-- Insert default scheduled tasks
INSERT INTO SCHEDULER_TASKS (task_name, task_type, schedule_cron, enabled, config) VALUES
    ('cleanup_trader_sessions', 'cleanup', '0 2 * * *', TRUE, '{"retention_days": 7}'),
    ('cleanup_audit_logs', 'cleanup', '0 3 * * *', TRUE, '{"retention_days": 7}'),
    ('reset_daily_limits', 'maintenance', '0 0 * * *', TRUE, '{}'),
    ('check_reencryption_jobs', 'reencryption', NULL, TRUE, '{"check_interval_sec": 60}')
ON DUPLICATE KEY UPDATE updated_at = CURRENT_TIMESTAMP;

-- ============================================================================
-- Sample Data (Optional - for testing)
-- ============================================================================

-- Insert sample trader
-- INSERT INTO TRADER (trader_id, name, certificate_cn, region, status, max_tasks)
-- VALUES 
--     ('trader-eu-1', 'EU Frankfurt Trader', 'trader-eu-1', 'eu', 'registered', 10),
--     ('trader-us-1', 'US New York Trader', 'trader-us-1', 'us', 'registered', 10);

-- ============================================================================
-- Cleanup Jobs (Should be scheduled via cron or application)
-- ============================================================================

-- Cleanup old trader sessions (older than 7 days)
-- DELETE FROM TRADER_SESSION WHERE ended_at IS NOT NULL AND ended_at < DATE_SUB(NOW(), INTERVAL 7 DAY);

-- Cleanup old audit logs (older than 7 days)
-- DELETE FROM AUDIT_LOG WHERE timestamp < DATE_SUB(NOW(), INTERVAL 7 DAY);

-- Reset daily limits (run daily at midnight)
-- UPDATE EXCHANGE_LIMITS 
-- SET current_value = 0, reset_at = DATE_ADD(CURDATE(), INTERVAL 1 DAY)
-- WHERE limit_type IN ('orders_per_day', 'volume_per_day') AND reset_at < NOW();

-- ============================================================================
-- End of Migration
-- ============================================================================
