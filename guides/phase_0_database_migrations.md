# Phase 0: Database Migrations - Детальный гайд

> **Статус**: 🔵 Ready to Execute  
> **Время**: ~1.5 часа  
> **Приоритет**: 🔴 Critical (НАЧАТЬ ЗДЕСЬ)

---

## Обзор

**Цель:** Применить SQL миграции для создания всех необходимых таблиц Phase 1.

**Готово:** ✅ migrations/001_phase1_schema.sql создан (11 tables, 397 строк SQL)

**Что будет создано:**
- 11 новых таблиц (TRADER, TRADER_SESSION, EXCHANGE_LIMITS, и др.)
- ALTER USER_2FA для HSM key rotation
- 4 scheduler tasks по умолчанию

---

## 0.0 Предварительные проверки (15 минут)

### Шаг 1: Проверить доступ к MySQL

```bash
mysql -u root -proot -h 127.0.0.1 -e "SELECT VERSION();"
# Expected: MySQL 9.0.x

mysql -u root -proot -h 127.0.0.1 -e "SHOW DATABASES LIKE 'ct_system';"
# Expected: ct_system exists
```

**✅ Ожидаемый результат:**
```
+--------------------+
| Database (ct_system) |
+--------------------+
| ct_system          |
+--------------------+
```

### Шаг 2: Проверить существующие таблицы

```bash
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW TABLES;"
```

**✅ Ожидаемые таблицы (5 existing):**
- ARBITRAGE_TRANS
- USER
- EXCHANGE_ACCOUNTS
- USER_2FA
- MONITORING

### Шаг 3: Backup существующих данных (опционально для DEV)

```bash
mysqldump -u root -proot -h 127.0.0.1 ct_system > backup_$(date +%Y%m%d_%H%M%S).sql

# Verify backup
ls -lh backup_*.sql
```

### Шаг 4: Проверить файл миграции

```bash
wc -l migrations/001_phase1_schema.sql
# Expected: 397 lines

head -20 migrations/001_phase1_schema.sql
# Check: Header comment present, USE ct_system; statement

# Check for syntax errors
mysql -u root -proot -h 127.0.0.1 ct_system --execute="source migrations/001_phase1_schema.sql" --dry-run 2>&1 | grep -i error
# Expected: No output (no syntax errors)
```

**✅ Definition of Done:**
- [x] MySQL доступен и версия >= 9.0
- [x] База ct_system существует
- [x] Backup создан (если нужен)
- [x] Файл миграции прочитан и понятен

---

## 0.1 Применение миграций (30 минут)

### Команда выполнения

```bash
cd /home/dev/docker/cts-core

mysql -u root -proot -h 127.0.0.1 ct_system < migrations/001_phase1_schema.sql 2>&1 | tee migration.log
```

### Что происходит

**Section 1-8:** CREATE TABLE для 8 основных таблиц
- TRADER (admin pre-registration)
- TRADER_SESSION (connection history, 7 days retention)
- EXCHANGE_LIMITS (per-exchange rate limits)
- TRADER_EXCHANGE_RESOURCE (usage tracking)
- ARBITRAGE_ORDER (middle level - per exchange)
- ORDER_TRANSACTION (bottom level - fills/partials)
- AUDIT_LOG (Phase 2, optional)
- (MONITORING - ALTER to add trader assignment fields)

**Section 9:** HSM Key Rotation Support (3 tables)
- ALTER USER_2FA (add enc_key_version, needs_reencryption)
- REENCRYPTION_JOBS (job tracking)
- REENCRYPTION_PROGRESS (per-record tracking)

**Section 10:** SCHEDULER_TASKS + 4 default tasks
- cleanup_trader_sessions (daily 2 AM)
- cleanup_audit_logs (daily 3 AM)
- reset_daily_limits (daily midnight)
- check_reencryption_jobs (every 60 seconds)

### Ожидаемый вывод

```
Query OK, 0 rows affected (0.05 sec)
Query OK, 0 rows affected (0.03 sec)
Query OK, 0 rows affected (0.04 sec)
...
Query OK, 4 rows affected (0.01 sec)
Records: 4  Duplicates: 0  Warnings: 0
```

### Проверка в процессе

```bash
# Если ошибка "Table already exists" - нормально, идем дальше (IF NOT EXISTS)
# Если ошибка "Syntax error" - STOP, проверить migration.log детально
# Если ошибка "Foreign key constraint" - проверить порядок таблиц

tail -50 migration.log
```

### Troubleshooting

**Проблема:** `ERROR 1050 (42S01): Table 'TRADER' already exists`
**Решение:** Нормально, миграция использует `IF NOT EXISTS`. Продолжить.

**Проблема:** `ERROR 1064 (42000): You have an error in your SQL syntax`
**Решение:** 
1. Найти строку с ошибкой в migration.log
2. Проверить migrations/001_phase1_schema.sql на этой строке
3. Исправить синтаксис и запустить снова

**Проблема:** `ERROR 1452 (23000): Cannot add or update a child row: a foreign key constraint fails`
**Решение:**
1. Проверить порядок создания таблиц (parent должен быть раньше child)
2. Проверить что referenced таблица существует

**✅ Definition of Done:**
- [x] Команда выполнена без critical errors
- [x] migration.log содержит успешные результаты
- [x] Нет синтаксических ошибок SQL

---

## 0.2 Верификация таблиц (15 минут)

### Проверка 1: Все таблицы созданы

```sql
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW TABLES;"
```

**✅ Expected output (16 tables total):**
```
+-------------------------+
| Tables_in_ct_system     |
+-------------------------+
| ARBITRAGE_ORDER         | (NEW)
| ARBITRAGE_TRANS         | (existing)
| AUDIT_LOG               | (NEW)
| EXCHANGE_ACCOUNTS       | (existing)
| EXCHANGE_LIMITS         | (NEW)
| MONITORING              | (existing, ALTER applied)
| ORDER_TRANSACTION       | (NEW)
| REENCRYPTION_JOBS       | (NEW)
| REENCRYPTION_PROGRESS   | (NEW)
| SCHEDULER_TASKS         | (NEW)
| TRADER                  | (NEW)
| TRADER_EXCHANGE_RESOURCE| (NEW)
| TRADER_SESSION          | (NEW)
| USER                    | (existing)
| USER_2FA                | (existing, ALTER applied)
+-------------------------+
```

### Проверка 2: Структура ключевых таблиц

```sql
-- TRADER (admin pre-registration)
mysql -u root -proot -h 127.0.0.1 ct_system -e "DESCRIBE TRADER;"
```

**✅ Expected columns:**
- trader_id (PK)
- certificate_cn
- region
- status (active/suspended/deleted)
- max_tasks
- created_at, updated_at, created_by

```sql
-- TRADER_SESSION (connection history)
mysql -u root -proot -h 127.0.0.1 ct_system -e "DESCRIBE TRADER_SESSION;"
```

**✅ Expected columns:**
- id (PK, AUTO_INCREMENT)
- session_id (UNIQUE)
- trader_id (FK → TRADER)
- ws_connection_id
- connected_at, last_heartbeat, ended_at

```sql
-- USER_2FA (HSM key rotation added)
mysql -u root -proot -h 127.0.0.1 ct_system -e "DESCRIBE USER_2FA;"
```

**✅ Expected NEW columns:**
- enc_key_version (INT) - для tracking KEK version
- needs_reencryption (BOOLEAN) - флаг для scheduler

```sql
-- REENCRYPTION_JOBS (HSM key rotation)
mysql -u root -proot -h 127.0.0.1 ct_system -e "DESCRIBE REENCRYPTION_JOBS;"
```

**✅ Expected columns:**
- id (PK)
- job_type (enum: user_2fa, exchange_accounts, other)
- old_key_version, new_key_version
- status (pending/in_progress/completed/failed/cancelled)
- total_records, processed_records, failed_records
- batch_size

```sql
-- SCHEDULER_TASKS (background jobs)
mysql -u root -proot -h 127.0.0.1 ct_system -e "SELECT task_name, enabled FROM SCHEDULER_TASKS;"
```

**✅ Expected 4 tasks:**
```
+---------------------------+---------+
| task_name                 | enabled |
+---------------------------+---------+
| cleanup_trader_sessions   |       1 |
| cleanup_audit_logs        |       1 |
| reset_daily_limits        |       1 |
| check_reencryption_jobs   |       1 |
+---------------------------+---------+
```

### Проверка 3: Индексы

```sql
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW INDEX FROM TRADER;"
```

**✅ Expected indexes:**
- PRIMARY (trader_id)
- idx_certificate_cn
- idx_status
- idx_region

```sql
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW INDEX FROM TRADER_SESSION;"
```

**✅ Expected indexes:**
- PRIMARY (id)
- UNIQUE uk_session_id (session_id)
- idx_trader_id (trader_id)
- idx_connected_at
- idx_cleanup (ended_at, connected_at) - для auto-cleanup

```sql
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW INDEX FROM ARBITRAGE_ORDER;"
```

**✅ Expected indexes:**
- PRIMARY (id)
- UNIQUE uk_exchange_order (arbitrage_trans_id, exchange_name, exchange_order_id) - deduplication!

### Проверка 4: Foreign Keys

```sql
mysql -u root -proot -h 127.0.0.1 ct_system -e "
SELECT 
    TABLE_NAME,
    CONSTRAINT_NAME,
    REFERENCED_TABLE_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = 'ct_system'
AND REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY TABLE_NAME;
"
```

**✅ Expected foreign keys:**
```
+---------------------------+---------------------------+------------------------+
| TABLE_NAME                | CONSTRAINT_NAME           | REFERENCED_TABLE_NAME  |
+---------------------------+---------------------------+------------------------+
| ARBITRAGE_ORDER           | fk_arbitrage_trans        | ARBITRAGE_TRANS        |
| ORDER_TRANSACTION         | fk_arbitrage_order        | ARBITRAGE_ORDER        |
| REENCRYPTION_PROGRESS     | fk_reencryption_job       | REENCRYPTION_JOBS      |
| TRADER_SESSION            | fk_trader                 | TRADER                 |
+---------------------------+---------------------------+------------------------+
```

**✅ Definition of Done:**
- [x] 16 таблиц существуют (11 новых + 5 existing)
- [x] USER_2FA имеет enc_key_version, needs_reencryption
- [x] SCHEDULER_TASKS содержит 4 задачи (все enabled=TRUE)
- [x] Все индексы на месте (PRIMARY, UNIQUE, indexes)
- [x] Foreign keys работают (4 FK verified)

---

## 0.3 Тестирование базовых операций (15 минут)

### Тест 1: INSERT в TRADER (admin pre-registration)

```sql
mysql -u root -proot -h 127.0.0.1 ct_system <<EOF
-- Insert test trader
INSERT INTO TRADER (
    trader_id, certificate_cn, region, status, max_tasks, created_by
) VALUES (
    'trader-test-1', 
    'CN=trader-test-1,OU=Trading,O=Private',
    'EU',
    'active',
    5,
    1  -- admin USER.ID
);

-- Verify
SELECT * FROM TRADER WHERE trader_id = 'trader-test-1';

-- Cleanup
DELETE FROM TRADER WHERE trader_id = 'trader-test-1';
EOF
```

**✅ Expected result:**
```
trader_id: trader-test-1
certificate_cn: CN=trader-test-1,OU=Trading,O=Private
region: EU
status: active
max_tasks: 5
created_by: 1
```

### Тест 2: Foreign Key Constraint

```sql
mysql -u root -proot -h 127.0.0.1 ct_system <<EOF
-- Test 1: Should FAIL (trader doesn't exist)
INSERT INTO TRADER_SESSION (trader_id, ws_connection_id)
VALUES ('nonexistent', 'ws-123');
EOF
```

**✅ Expected error:**
```
ERROR 1452 (23000): Cannot add or update a child row: a foreign key constraint fails
```

```sql
mysql -u root -proot -h 127.0.0.1 ct_system <<EOF
-- Test 2: Should SUCCEED (trader exists)
INSERT INTO TRADER (trader_id, certificate_cn, region, status)
VALUES ('trader-test-2', 'CN=test', 'EU', 'active');

INSERT INTO TRADER_SESSION (trader_id, ws_connection_id)
VALUES ('trader-test-2', 'ws-123');

-- Verify
SELECT * FROM TRADER_SESSION WHERE trader_id = 'trader-test-2';

-- Test CASCADE delete
DELETE FROM TRADER WHERE trader_id = 'trader-test-2';

-- Verify CASCADE worked (session should be deleted too)
SELECT COUNT(*) as count FROM TRADER_SESSION WHERE trader_id = 'trader-test-2';
EOF
```

**✅ Expected result:**
```
count: 0  (CASCADE delete worked!)
```

### Тест 3: UNIQUE Constraint (deduplication)

```sql
mysql -u root -proot -h 127.0.0.1 ct_system <<EOF
-- Assuming ARBITRAGE_TRANS with ID=1 exists
-- If not, create one first

-- Insert first order - should SUCCEED
INSERT INTO ARBITRAGE_ORDER (
    arbitrage_trans_id, exchange_name, exchange_order_id, side, price
) VALUES (1, 'binance', 'ORDER-123', 'buy', 50000.00);

-- Try duplicate - should FAIL
INSERT INTO ARBITRAGE_ORDER (
    arbitrage_trans_id, exchange_name, exchange_order_id, side, price
) VALUES (1, 'binance', 'ORDER-123', 'sell', 50100.00);
EOF
```

**✅ Expected error on duplicate:**
```
ERROR 1062 (23000): Duplicate entry '1-binance-ORDER-123' for key 'uk_exchange_order'
```

```sql
-- Cleanup
mysql -u root -proot -h 127.0.0.1 ct_system -e "
DELETE FROM ARBITRAGE_ORDER WHERE exchange_order_id = 'ORDER-123';
"
```

### Тест 4: SCHEDULER_TASKS defaults

```sql
mysql -u root -proot -h 127.0.0.1 ct_system -e "
SELECT 
    task_name,
    enabled,
    schedule_type,
    schedule_value,
    last_run_at
FROM SCHEDULER_TASKS;
"
```

**✅ Expected output:**
```
+---------------------------+---------+---------------+----------------+-------------+
| task_name                 | enabled | schedule_type | schedule_value | last_run_at |
+---------------------------+---------+---------------+----------------+-------------+
| cleanup_trader_sessions   |       1 | cron          | 0 2 * * *      | NULL        |
| cleanup_audit_logs        |       1 | cron          | 0 3 * * *      | NULL        |
| reset_daily_limits        |       1 | cron          | 0 0 * * *      | NULL        |
| check_reencryption_jobs   |       1 | interval      | 60             | NULL        |
+---------------------------+---------+---------------+----------------+-------------+
```

**✅ Definition of Done:**
- [x] INSERT в TRADER работает
- [x] Foreign key constraints работают (error при нарушении)
- [x] CASCADE delete работает
- [x] UNIQUE constraints работают (deduplication)
- [x] SCHEDULER_TASKS содержит валидные задачи

---

## 0.4 Документация изменений (10 минут)

### Создать migration log

```bash
cat > migration_applied_$(date +%Y%m%d).md <<EOF
# Migration Applied: 001_phase1_schema.sql

**Date:** $(date)
**Applied by:** $(whoami)
**Database:** ct_system @ 127.0.0.1

## Tables Created (11 new):
1. TRADER - Admin pre-registration of traders
2. TRADER_SESSION - Connection history (7 days retention)
3. EXCHANGE_LIMITS - Exchange rate limits (orders/volume per day)
4. TRADER_EXCHANGE_RESOURCE - Trader resource usage tracking
5. ARBITRAGE_ORDER - Middle level (per exchange orders)
6. ORDER_TRANSACTION - Bottom level (fills/partials)
7. AUDIT_LOG - Admin operations audit trail
8. REENCRYPTION_JOBS - HSM key rotation job tracking
9. REENCRYPTION_PROGRESS - Per-record re-encryption progress
10. SCHEDULER_TASKS - Background job definitions

## Tables Altered (1):
1. USER_2FA - Added enc_key_version, needs_reencryption for HSM key rotation

## Verification Results:
- Total tables: 16 (11 new + 5 existing)
- Total indexes: $(mysql -u root -proot -h 127.0.0.1 ct_system -e "SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA='ct_system';" -N)
- Foreign keys: 4 (verified CASCADE works)
- UNIQUE constraints: 3 (verified deduplication works)
- Default scheduler tasks: 4

## Tests Passed:
- ✅ INSERT into TRADER
- ✅ Foreign key constraints enforced
- ✅ CASCADE delete works
- ✅ UNIQUE constraint deduplication works
- ✅ Scheduler tasks initialized

## Next Steps:
- Phase 1.1: Project Setup (go.mod, config, logger)
- Commit changes to git
EOF

cat migration_applied_$(date +%Y%m%d).md
```

### Git commit

```bash
git add migrations/001_phase1_schema.sql
git add migration_applied_$(date +%Y%m%d).md

git commit -m "feat(db): phase 0 complete - applied 11 table migrations

- TRADER, TRADER_SESSION for session management
- EXCHANGE_LIMITS, TRADER_EXCHANGE_RESOURCE for load balancing
- ARBITRAGE_ORDER, ORDER_TRANSACTION for 3-level trade structure
- REENCRYPTION_JOBS, REENCRYPTION_PROGRESS, SCHEDULER_TASKS for HSM key rotation
- AUDIT_LOG for compliance
- ALTER USER_2FA for key versioning

All verifications passed:
- 16 tables created/altered
- 4 foreign keys verified
- 3 UNIQUE constraints tested
- 4 scheduler tasks initialized

Ready for Phase 1.1."

git push
```

**✅ Definition of Done:**
- [x] Migration log создан с датой и результатами
- [x] Изменения закоммичены в git
- [x] Документация обновлена

---

## 0.5 Rollback Plan (если что-то пошло не так)

### ⚠️ WARNING: Destructive Operation!

**Если нужно откатить миграции:**

```sql
-- Save this as migrations/rollback_001.sql

USE ct_system;

-- WARNING: This will DELETE all data in new tables!

-- Drop tables in reverse dependency order (child first, parent last)
DROP TABLE IF EXISTS REENCRYPTION_PROGRESS;
DROP TABLE IF EXISTS REENCRYPTION_JOBS;
DROP TABLE IF EXISTS SCHEDULER_TASKS;
DROP TABLE IF EXISTS ORDER_TRANSACTION;
DROP TABLE IF EXISTS ARBITRAGE_ORDER;
DROP TABLE IF EXISTS AUDIT_LOG;
DROP TABLE IF EXISTS TRADER_EXCHANGE_RESOURCE;
DROP TABLE IF EXISTS EXCHANGE_LIMITS;
DROP TABLE IF EXISTS TRADER_SESSION;
DROP TABLE IF EXISTS TRADER;

-- Revert USER_2FA changes
ALTER TABLE USER_2FA
DROP COLUMN IF EXISTS enc_key_version,
DROP COLUMN IF EXISTS needs_reencryption;

-- Verify rollback
SHOW TABLES;
```

**Выполнить rollback:**
```bash
mysql -u root -proot -h 127.0.0.1 ct_system < migrations/rollback_001.sql

# Verify - should show only 5 original tables
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW TABLES;"
```

### Восстановление из backup

**Если был создан backup:**
```bash
# List backups
ls -lh backup_*.sql

# Restore specific backup
mysql -u root -proot -h 127.0.0.1 ct_system < backup_20260128_100000.sql

# Verify
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW TABLES;"
```

---

## Phase 0 Summary

### ✅ Completed Checklist

- [x] **0.0 Pre-checks** (15 min)
  - [x] MySQL accessible, version >= 9.0
  - [x] Database ct_system exists
  - [x] Backup created (optional)
  - [x] Migration file verified

- [x] **0.1 Apply migrations** (30 min)
  - [x] Executed migrations/001_phase1_schema.sql
  - [x] No critical errors
  - [x] Migration log created

- [x] **0.2 Verify tables** (15 min)
  - [x] 16 tables present (11 new + 5 existing)
  - [x] All columns correct
  - [x] Indexes created
  - [x] Foreign keys working

- [x] **0.3 Test operations** (15 min)
  - [x] INSERT works
  - [x] Foreign key constraints enforced
  - [x] CASCADE delete works
  - [x] UNIQUE constraints work (deduplication)

- [x] **0.4 Documentation** (10 min)
  - [x] Migration log created
  - [x] Git commit/push
  - [x] Ready for Phase 1.1

### 📊 Metrics

**Total Time:** ~1.5 hours  
**Tables Created:** 11 new  
**Tables Altered:** 1 (USER_2FA)  
**Total Tables:** 16  
**Foreign Keys:** 4  
**UNIQUE Constraints:** 3  
**Scheduler Tasks:** 4  
**SQL Lines:** 397  

### 🎯 Next Phase

**Phase 1.1: Project Setup**
- Location: `guides/phase_1_1_project_setup.md`
- Time: 1 day
- Deliverables: go.mod, config.yaml, logger, main.go, Makefile

---

## ❓ FAQ

**Q: Можно ли запустить миграцию повторно?**  
A: Да, все CREATE TABLE используют IF NOT EXISTS. Повторный запуск безопасен.

**Q: Что если таблица уже существует?**  
A: Будет warning "Table already exists", но миграция продолжится. Это нормально.

**Q: Как проверить что всё работает?**  
A: Запустите все тесты из секции 0.3. Все должны пройти без ошибок.

**Q: Можно ли откатить изменения?**  
A: Да, см. секцию 0.5 Rollback Plan. Но это удалит все данные в новых таблицах.

**Q: Сколько места займут таблицы?**  
A: Пустые таблицы ~1-2 MB. С данными зависит от нагрузки (примерно 100-500 MB для Phase 1).

---

**🗑️ DELETE THIS FILE** after successfully completing Phase 0
