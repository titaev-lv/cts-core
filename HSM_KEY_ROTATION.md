# HSM Key Rotation (Current State)

> **Версия**: 2.0.0  
> **Дата**: 2026-02-18  
> **Связанные документы**: [ARCHITECTURE.md](ARCHITECTURE.md)

---

## Обзор

В текущей реализации CTS-Core нет автоматического re-encryption пайплайна, таблиц tracking или admin API для перешифровки. Документ описывает только реальные, доступные сегодня шаги.

**Что есть сейчас:**
- Ротация KEK выполняется на стороне hsm-service через `hsm-admin`.
- Старые версии KEK могут сохраняться для обратной совместимости (политика retention в hsm-service).
- CTS-Core не ведет учет `enc_key_version` в БД и не запускает jobs на перешифровку.
- Таблицы `REENCRYPTION_JOBS`, `REENCRYPTION_PROGRESS`, `SCHEDULER_TASKS`.

**Что отсутствует:**
- Admin API `/api/v1/admin/reencryption/*`.
- Автоматический или batch re-encryption данных CTS-Core.

---

## Процесс ротации KEK (реальный)

### 1. Ротация ключа в hsm-service

```bash
# В hsm-service
cd /path/to/hsm-service

# Ротация KEK для exchange-key context
./hsm-admin rotate exchange-key

# Ротация KEK для 2fa context
./hsm-admin rotate 2fa
```

### 2. Проверка состояния ключей

```bash
# Список ключей
./hsm-admin list-kek

# Статус ротации и версии
./hsm-admin rotation-status

# Контроль целостности (если используется)
./hsm-admin update-checksums
```

---

## Влияние на CTS-Core

- CTS-Core не имеет механизма автоматического re-encryption данных в БД.
- Если в БД хранятся значения, зашифрованные через KEK, миграция на новую версию выполняется прикладным способом (вручную или через отдельный скрипт/процесс вне CTS-Core).
- Старые версии KEK должны оставаться в HSM достаточно долго, чтобы успеть мигрировать данные.

---

## Рекомендованный минимальный чеклист

1. Запустить `hsm-admin rotate <context>` в hsm-service.
2. Проверить `hsm-admin rotation-status` и `list-kek`.
3. Если есть данные в БД, завязанные на KEK, выполнить их перешифровку вне CTS-Core.
4. После полной миграции данных можно удалить старые версии KEK через `hsm-admin cleanup-old-versions` (при наличии политик retention).

---

## Будущая работа (не реализовано)

Этот документ больше не описывает несуществующие API/DDL. Если потребуется автоматизация, нужно:

- Определить таблицы tracking re-encryption (job/progress).
- Добавить admin API для запуска и мониторинга job.
- Реализовать batch re-encryption с безопасными транзакциями и retry.

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
