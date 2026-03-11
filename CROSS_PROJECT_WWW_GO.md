# Cross-Project Integration: cts-core <-> web-ui-go

**Документ нужен:** да, как рабочий контракт между сервисами и короткий backlog интеграции.  
**Статус:** актуализирован по коду и БД на 2026-03-10.  
**Область:** только пересечение `cts-core` и `web-ui-go`.

## Зачем этот файл

- Фиксирует, какие таблицы и поля `web-ui-go` может безопасно использовать для админки.
- Отделяет уже реализованное от того, что еще нужно сделать.
- Убирает устаревшие пути и несуществующие API из старой версии.

Если интеграционные задачи будут закрыты, документ можно перевести в архив и оставить только ссылки на спецификацию API и модели.

## Текущее состояние (проверено)

### 1) База данных (localhost)

Проверено подключением `mysql -h 127.0.0.1 -u root -proot`:

- Таблицы существуют: `TRADER`, `TRADER_SESSION`, `TRADER_EXCHANGE_RESOURCE`, `AUDIT_LOG`, `SCHEDULER_TASKS`.
- Наполнение сейчас:
  - `TRADER`: 0
  - `TRADER_SESSION`: 0
  - `TRADER_EXCHANGE_RESOURCE`: 0
  - `AUDIT_LOG`: 0
  - `SCHEDULER_TASKS`: 4

Вывод: схема развернута, но функционал по трейдерам/сессиям в UI пока фактически не используется.

### 2) cts-core

- Модели и репозитории для нужных таблиц есть.
- `audit.go` уже содержит константы для операций `USER_GROUP_*` и типы ресурсов.
- Поле ресурса называется `USED_VALUE` (не `CURRENT_VALUE`).

### 3) web-ui-go

На момент проверки есть:

- Контроллеры: `exchange_controller.go`, `exchange_account_controller.go`, `group_controller.go`, `user_controller.go`.
- Middleware: `internal/middleware/audit_log.go`.

Отсутствуют:

- `trader_controller.go`
- `trader_session_controller.go`
- `scheduler_controller.go`
- отдельный viewer для `AUDIT_LOG`

## Актуальные пути проектов

- `cts-core`: `/home/dev/docker/cts-system/services/cts-core`
- `web-ui-go`: `/home/dev/docker/cts-system/services/web-ui-go`

Старые пути вида `/home/dev/docker/cts-core/other-sub-system/...` больше не использовать.

## Минимальный контракт данных для web-ui-go

### TRADER

Ключевые поля:

- `ID`
- `TRADER_NAME`
- `CERTIFICATE_CN` (UNIQUE)
- `REGION`
- `STATUS` (`registered|active|suspended|decommissioned`)
- `MAX_TASKS`
- `DATE_CREATE`, `DATE_MODIFY`
- `USER_CREATED`, `USER_MODIFY`, `NOTES`

Правила:

- `CERTIFICATE_CN` обязан совпадать с CN сертификата трейдера.
- Удаление через soft-delete: `STATUS='decommissioned'`.

### TRADER_SESSION

Ключевые поля:

- `TRADER_ID`
- `SESSION_ID` (UNIQUE)
- `IP_ADDRESS`
- `CONNECTED_AT`
- `LAST_HEARTBEAT`
- `ENDED_AT` (`NULL` = активна)
- `DISCONNECT_REASON` (`graceful|timeout|error|server_shutdown|kicked`)
- `ERROR_MESSAGE`

### TRADER_EXCHANGE_RESOURCE

Ключевые поля:

- `TRADER_ID`
- `EXCHANGE_ID`
- `EXCHANGE_ACCOUNT_ID` (`NULL` для IP-level лимитов)
- `RESOURCE_TYPE`
- `USED_VALUE`
- `MAX_VALUE`
- `LAST_UPDATED`
- `RESET_AT`

Важно: в SQL/Go используется `USED_VALUE`, а не `CURRENT_VALUE`.

### AUDIT_LOG

Ключевые поля:

- `TIMESTAMP(6)`
- `UID`
- `ACTION`
- `RESOURCE_TYPE`, `RESOURCE_ID`
- `OLD_VALUE`, `NEW_VALUE` (JSON)
- `IP_ADDRESS`, `USER_AGENT`
- `SUCCESS`, `ERROR_MESSAGE`

### SCHEDULER_TASKS

Ключевые поля:

- `TASK_NAME`, `TASK_TYPE`
- `SCHEDULE_CRON` / `SCHEDULE_INTERVAL_SEC`
- `ENABLED`, `STATUS`
- `LAST_RUN_*`, `NEXT_RUN_AT`
- `RUN_COUNT`, `ERROR_COUNT`
- `CONFIG` (JSON)

## Что уже неактуально в старой версии документа

- Утверждение, что нужно "добавить константы `USER_GROUP_*` в cts-core" (уже добавлены).
- Упоминания старых путей `other-sub-system`.
- Примеры с `CURRENT_VALUE` (должно быть `USED_VALUE`).
- Упоминания несуществующих внутренних API как будто они уже есть.
- Фрагменты про PHP-структуру как основной target для текущего `web-ui-go` (сейчас основной backend на Go).

## Реальный backlog интеграции (приоритет)

### P1

- Реализовать в `web-ui-go` `TRADER CRUD`:
  - список/фильтры
  - create/update
  - soft-delete (`decommissioned`)
  - валидация `CERTIFICATE_CN`, `STATUS`, `MAX_TASKS`
- Реализовать запись в `AUDIT_LOG` для CRUD операций по трейдерам.

### P2

- Реализовать read-only мониторинг `TRADER_SESSION`:
  - активные сессии
  - история
  - метка stale heartbeat
- Реализовать read-only мониторинг `TRADER_EXCHANGE_RESOURCE`:
  - текущая загрузка
  - расчет `usage_percent = USED_VALUE / MAX_VALUE`

### P3

- Viewer `AUDIT_LOG` в UI с фильтрами и пагинацией.
- UI управления `SCHEDULER_TASKS` (toggle/schedule/edit config).

## Граница ответственности

- `cts-core`:
  - runtime-состояние, WS, обновление сессий/ресурсов, системный audit.
- `web-ui-go`:
  - админский CRUD и read-only дашборды поверх БД.

Этот документ не заменяет API-спеки и не описывает весь `web-ui-go`.

## Быстрые SQL-проверки

```sql
USE ct_system;

SELECT COUNT(*) FROM TRADER;
SELECT COUNT(*) FROM TRADER_SESSION;
SELECT COUNT(*) FROM TRADER_EXCHANGE_RESOURCE;
SELECT COUNT(*) FROM AUDIT_LOG;
SELECT COUNT(*) FROM SCHEDULER_TASKS;

SELECT TASK_NAME, STATUS, ENABLED, NEXT_RUN_AT
FROM SCHEDULER_TASKS
ORDER BY ID;
```

## Критерий, когда документ можно архивировать

- Реализованы `TRADER CRUD`, `TRADER_SESSION` и `TRADER_EXCHANGE_RESOURCE` мониторинги в `web-ui-go`.
- Есть рабочий `AUDIT_LOG` viewer.
- Есть UI/операции для `SCHEDULER_TASKS`.
- Открытые пункты в этом файле пусты или перенесены в трекер задач.
