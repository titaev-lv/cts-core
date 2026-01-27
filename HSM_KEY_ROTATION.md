# HSM Key Rotation Guide

> **Версия**: 1.0.0  
> **Дата**: 2026-01-28  
> **Связанные документы**: [ARCHITECTURE.md](ARCHITECTURE.md), [migrations/001_phase1_schema.sql](migrations/001_phase1_schema.sql)

---

## Обзор

HSM-service поддерживает key rotation (смена KEK - Key Encryption Key). После ротации все зашифрованные данные в БД нужно перешифровать на новый ключ.

**Затронутые данные:**
- `EXCHANGE_ACCOUNTS` - API keys (context: exchange-key, OU=Trading)
- `USER_2FA` - TOTP secrets (context: 2fa, OU=2FA)

---

## Подготовка БД

### Миграция USER_2FA

```sql
-- USER_2FA изначально не имел версионирования ключей
-- Добавлено в migrations/001_phase1_schema.sql

ALTER TABLE USER_2FA
ADD COLUMN enc_key_version INT COMMENT 'HSM KEK version (from key_id)',
ADD COLUMN needs_reencryption BOOLEAN DEFAULT FALSE;

-- Установить текущую версию для существующих записей
UPDATE USER_2FA 
SET enc_key_version = 1 
WHERE enc_key_version IS NULL AND SECRET_ENC IS NOT NULL;
```

### Таблицы для tracking

```sql
-- REENCRYPTION_JOBS - управление заданиями на перешифровку
-- REENCRYPTION_PROGRESS - прогресс по каждой записи
-- SCHEDULER_TASKS - автоматическая проверка новых версий ключей

-- См. migrations/001_phase1_schema.sql для полного DDL
```

---

## Процесс Key Rotation

### 1. HSM Key Rotation (Admin)

```bash
# В hsm-service
cd /path/to/hsm-service

# Ротация KEK для exchange-key context
./hsm-admin key-rotate --context exchange-key

# Ротация KEK для 2fa context
./hsm-admin key-rotate --context 2fa

# Проверка версий
curl -X GET https://hsm-service:8443/api/v1/keys/metadata \
  --cert client.crt --key client.key --cacert ca.crt
```

**Response:**
```json
{
  "contexts": [
    {
      "context": "exchange-key",
      "current_version": 2,
      "previous_versions": [1],
      "algorithm": "AES-256-GCM",
      "created_at": "2026-01-28T10:00:00Z"
    },
    {
      "context": "2fa",
      "current_version": 2,
      "previous_versions": [1],
      "created_at": "2026-01-28T10:05:00Z"
    }
  ]
}
```

### 2. Инициация Re-encryption (CTS-Core Admin API)

```bash
# Автоматическая детекция (scheduler проверяет каждые 60 сек)
# ИЛИ ручная инициация:

# Для EXCHANGE_ACCOUNTS
curl -X POST https://cts-core:8443/api/v1/admin/reencryption/initiate \
  --cert admin.crt --key admin.key --cacert ca.crt \
  -H "Content-Type: application/json" \
  -d '{
    "job_type": "exchange_accounts",
    "context": "exchange-key",
    "new_key_version": 2
  }'

# Для USER_2FA
curl -X POST https://cts-core:8443/api/v1/admin/reencryption/initiate \
  --cert admin.crt --key admin.key --cacert ca.crt \
  -H "Content-Type: application/json" \
  -d '{
    "job_type": "user_2fa",
    "context": "2fa",
    "new_key_version": 2
  }'
```

**Response:**
```json
{
  "job_id": 123,
  "status": "pending",
  "job_type": "exchange_accounts",
  "old_key_version": 1,
  "new_key_version": 2,
  "total_records": 1523,
  "estimated_duration_minutes": 15
}
```

### 3. Мониторинг прогресса

```bash
# Статус задания
curl -X GET https://cts-core:8443/api/v1/admin/reencryption/jobs/123 \
  --cert admin.crt --key admin.key --cacert ca.crt
```

**Response:**
```json
{
  "job_id": 123,
  "job_type": "exchange_accounts",
  "status": "in_progress",
  "progress_percent": 45.2,
  "processed_records": 689,
  "failed_records": 3,
  "total_records": 1523,
  "started_at": "2026-01-28T15:00:00Z",
  "last_processed_at": "2026-01-28T15:05:30Z",
  "estimated_completion": "2026-01-28T15:12:00Z"
}
```

### 4. Проверка результатов

```sql
-- Все ли записи перешифрованы?
SELECT 
    enc_key_version,
    COUNT(*) as count
FROM EXCHANGE_ACCOUNTS
GROUP BY enc_key_version;

-- Expected:
-- enc_key_version | count
-- 2               | 1523

-- Проверка USER_2FA
SELECT 
    enc_key_version,
    COUNT(*) as count
FROM USER_2FA
WHERE SECRET_ENC IS NOT NULL
GROUP BY enc_key_version;

-- Проверка failed records
SELECT * FROM REENCRYPTION_PROGRESS
WHERE job_id = 123 AND status = 'failed';
```

### 5. Retry failed records (если есть)

```bash
curl -X POST https://cts-core:8443/api/v1/admin/reencryption/jobs/123/retry \
  --cert admin.crt --key admin.key --cacert ca.crt
```

---

## Алгоритм Re-encryption

### Batch Processing

```
FOR EACH batch OF 100 records:
  1. SELECT records WHERE enc_key_version = old_version LIMIT 100
  2. FOR EACH record:
      a) Decrypt with old KEK (v1):
         HSM POST /decrypt {context, key_version: 1, ciphertext}
      
      b) Encrypt with new KEK (v2):
         HSM POST /encrypt {context, key_version: 2, plaintext}
      
      c) UPDATE record:
         SET DEK_ENC = new_ciphertext,
             API_KEY_ENC = ..., (re-encrypt all encrypted fields)
             enc_key_version = 2
         WHERE ID = record_id
      
      d) Track progress:
         INSERT INTO REENCRYPTION_PROGRESS (status='completed')
  
  3. Sleep 100ms (avoid HSM/DB overload)
  
  4. Update job progress:
     UPDATE REENCRYPTION_JOBS 
     SET processed_records = processed_records + batch_count
```

### Safety Measures

1. **Transactional updates** - rollback on error
2. **Progress tracking** - можно остановить и продолжить
3. **Failed record handling** - не блокирует весь процесс
4. **Batch delays** - 100ms между батчами (не перегружаем)
5. **Old key retention** - старый KEK хранится 30 дней (rollback)

---

## Rollback (если что-то пошло не так)

### Scenario: New key corrupted or HSM issue

```sql
-- 1. Остановить re-encryption job
UPDATE REENCRYPTION_JOBS SET status = 'cancelled' WHERE id = 123;

-- 2. Проверить какие записи уже перешифрованы
SELECT COUNT(*) FROM EXCHANGE_ACCOUNTS WHERE enc_key_version = 2;
SELECT COUNT(*) FROM EXCHANGE_ACCOUNTS WHERE enc_key_version = 1;

-- 3. Если новый ключ неработоспособен:
--    Старый KEK v1 все еще доступен в HSM (30 дней)
--    Можно использовать старые records (enc_key_version=1)

-- 4. Опционально: rollback перешифрованных records
--    (Инициировать reverse re-encryption job v2 → v1)
```

---

## Scheduling

### Автоматическая проверка

```sql
-- Scheduler task (проверяет каждые 60 сек)
SELECT * FROM SCHEDULER_TASKS WHERE task_name = 'check_reencryption_jobs';

-- Config:
{
  "check_interval_sec": 60,
  "batch_size": 100,
  "sleep_between_batches_ms": 100,
  "auto_initiate": false  // true = auto-start, false = manual approval
}
```

### Manual scheduling

```bash
# Планирование на off-peak hours (2:00 AM)
curl -X POST https://cts-core:8443/api/v1/admin/reencryption/schedule \
  --cert admin.crt --key admin.key --cacert ca.crt \
  -H "Content-Type: application/json" \
  -d '{
    "job_type": "exchange_accounts",
    "new_key_version": 2,
    "scheduled_at": "2026-01-29T02:00:00Z"
  }'
```

---

## Troubleshooting

### Problem: Job stuck in 'in_progress'

```sql
-- Check last activity
SELECT 
    id, 
    status, 
    processed_records, 
    total_records,
    last_processed_at,
    TIMESTAMPDIFF(MINUTE, last_processed_at, NOW()) as minutes_since_last_update
FROM REENCRYPTION_JOBS
WHERE status = 'in_progress';

-- If stuck > 30 minutes, check CTS-Core logs:
tail -f /path/to/cts-core/logs/daemon.log | grep reencryption

-- Manual reset (if CTS-Core crashed mid-job):
UPDATE REENCRYPTION_JOBS 
SET status = 'pending', last_processed_at = NULL
WHERE id = 123;
```

### Problem: High failure rate

```sql
-- Check failed records
SELECT 
    rp.record_id,
    rp.error_message,
    rp.attempt_count
FROM REENCRYPTION_PROGRESS rp
WHERE rp.job_id = 123 AND rp.status = 'failed'
LIMIT 20;

-- Common errors:
-- 1. HSM timeout → increase timeout in config
-- 2. Invalid ciphertext → record corrupted, manual fix needed
-- 3. Network error → retry automatically
```

### Problem: Performance impact

```sql
-- Check job duration
SELECT 
    id,
    job_type,
    started_at,
    last_processed_at,
    TIMESTAMPDIFF(MINUTE, started_at, last_processed_at) as duration_minutes,
    processed_records,
    total_records,
    (processed_records / total_records * 100) as progress_percent
FROM REENCRYPTION_JOBS
WHERE id = 123;

-- Adjust batch size and delay:
UPDATE REENCRYPTION_JOBS
SET batch_size = 50  -- reduce from 100
WHERE id = 123;

-- Pause job temporarily:
UPDATE REENCRYPTION_JOBS SET status = 'paused' WHERE id = 123;

-- Resume:
UPDATE REENCRYPTION_JOBS SET status = 'pending' WHERE id = 123;
```

---

## Best Practices

1. **Test in DEV first** - run complete rotation cycle in dev environment
2. **Schedule during low traffic** - 2-4 AM usually best
3. **Monitor HSM load** - watch hsm-service metrics during re-encryption
4. **Keep old keys 30 days** - safety net for rollback
5. **Backup before rotation** - mysqldump EXCHANGE_ACCOUNTS, USER_2FA
6. **Verify after completion** - check all records have new version
7. **Document rotation date** - keep audit trail of rotations

---

## Security Notes

- ✅ Plaintext never written to disk (only in memory during re-encryption)
- ✅ All HSM communication via mTLS
- ✅ Audit trail in AUDIT_LOG + logs/audit.log
- ✅ Old KEK versions retained in HSM (30 days policy)
- ✅ Progress tracking allows pause/resume without data loss
- ✅ Failed records isolated - don't block entire job

---

## References

- [ARCHITECTURE.md - Section 6.7](ARCHITECTURE.md#67-hsm-key-rotation--re-encryption-phase-1)
- [migrations/001_phase1_schema.sql - Section 9](migrations/001_phase1_schema.sql)
- [hsm-service KEY_ROTATION.md](../other-sub-system/hsm-service/KEY_ROTATION.md)
