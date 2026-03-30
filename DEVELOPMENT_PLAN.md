# CTS-Core & Trader Development Plan

> Версия: 3.0.0
> Обновлено: 2026-03-17
> Формат: Подробный реестр реализованного + актуальные следующие шаги
> Связанные документы: `ARCHITECTURE.md`, `API_SPECIFICATION.md`, `DOCS_INDEX.md`, `CROSS_PROJECT_WWW_GO.md`

## 1. Назначение документа

Этот файл фиксирует текущее фактическое состояние разработки в формате:
- что уже реализовано (подробно),
- какие решения и ограничения приняты,
- что остается в следующем цикле.

Фазовые секции, которые уже завершены, из документа убраны.

## 2. Подробно: что уже реализовано

### 2.1 Базовая платформа сервиса

- Поднят каркас сервиса `cts-core` с загрузкой конфигурации, инициализацией логгера, graceful shutdown.
- Подключен MySQL-клиент и repository layer.
- Подключены HSM-клиенты для двух контекстов:
  - trading (ключи бирж),
  - 2FA (re-encryption path).
- Введен state manager с записью runtime-состояния в `state/daemon.state`, ротацией backup и фоновым sync.

### 2.2 WebSocket runtime для trader

- Реализован runtime-протокол регистрации и heartbeat:
  - `trader.register` -> `trader.register_ack`,
  - `trader.heartbeat` -> `trader.heartbeat_ack`.
- Реализованы timeout/disconnect сценарии и телеметрия жизненного цикла сессии.
- Реализована persistence жизненного цикла сессий в `TRADER_SESSION`.
- Реализован in-memory registry активных WS-соединений (для server-initiated сообщений).

### 2.3 Усиление WS-протокола (hardening)

- Проверка версии протокола.
- Защита от flood по неизвестным action.
- Ограничения на размер сообщения.
- Rate limiting inbound WS-потока.
- Dedup request_id в окне времени.
- Typed errors (`INVALID_PAYLOAD.details` и др.) для диагностики.

### 2.4 Scheduler: модель выбора исполнителя

- Выбор кандидатов только из active/healthy WS snapshots.
- Task-type aware отбор:
  - для `trade`: участвуют `trade` и `both`,
  - для `monitor`: собственная логика приоритета.
- Нормализованная scoring-модель:
  - `trade`: latency profile + нелинейный штраф за load,
  - `monitor`: упор на минимальную торговую загрузку и role penalty.
- Введен детерминированный tie-break при равных score.

### 2.5 Scheduler: требования по биржам и latency profile

- Убран runtime-зависимый config-список required exchanges.
- Источник required exchanges переведен на DB-only путь:
  - `ListTradeExchanges()` / `ListMonitorExchanges()`.
- Добавлен server-initiated latency sweep:
  - периодический `latency.test` по полному набору `capabilities`,
  - прием `latency.test_result`,
  - обновление `exchange_latencies` и итогового latency profile.
- Интервал re-test зафиксирован и конфигурируется через scheduler config (`20m` по умолчанию).

### 2.6 Scheduler: ресурсы и вместимость

- Реализованы hard-resource ограничения на уровне выбора кандидатов:
  - кандидат исключается при нарушении жесткого порога.
- Реализованы soft-resource штрафы:
  - кандидат остается в ранжировании,
  - получает penalty к score в зоне soft-limit.
- Введена конфигурация ресурсной политики и валидация:
  - `resource_hard_limit`,
  - `resource_soft_limit`,
  - `resource_soft_penalty_ms`.
- Подключен DB adapter к `TRADER_EXCHANGE_RESOURCE` для расчета utilization `USED/MAX`.

### 2.7 Scheduler: устойчивость назначения

- Добавлена защита от дублей назначения на уровне scheduler:
  - idempotency key `task_id + trader_id + epoch`,
  - in-memory окно дедупликации.
- Добавлен retry policy:
  - ограниченное число повторов,
  - backoff,
  - классификация retryable/non-retryable ошибок.
- Добавлена минимальная in-memory dead-letter очередь для невыполнимых назначений.

### 2.8 Наблюдаемость планировщика

- Расширен runtime state для scheduler quality:
  - `last_assign_status`,
  - `assign_latency_ms`,
  - score distribution (`p50`, `p95`),
  - assign attempts по результатам,
  - resource rejections по причинам.
- Расширен `/metrics` новыми сериями качества назначения:
  - `cts_core_scheduler_assign_attempts_total{result=...}`,
  - `cts_core_scheduler_assign_latency_ms`,
  - `cts_core_scheduler_candidate_pool_size`,
  - `cts_core_scheduler_score_distribution{quantile=...}`,
  - `cts_core_scheduler_resource_rejections_total{reason=...}`.
- Расширен `/health` блоком `scheduler` с качественными полями и причинами отказов.

### 2.9 Тесты и верификация

- Расширены unit/integration тесты по направлениям:
  - WS handler/session lifecycle,
  - scheduler scoring/resources/idempotency/retry,
  - state manager runtime fields,
  - REST `/metrics` и `/health`.
- Регулярная валидация `go test ./...` в `services/cts-core` проходит успешно.

### 2.10 Текущее решение по объему работ

- По внутреннему решению команды задачи безопасного ввода из прежнего Phase 3 backlog исключены из текущего цикла:
  - `P3-009` (feature toggles + dry-run),
  - `P3-010` (расширенный smoke assignment path).
- Реализация hybrid reconciliation `core <-> trader` закреплена как следующий интеграционный блок, а не часть завершенных работ текущего цикла.

## 3. Принятые правила и ограничения

- CTS-Core выбирает исполнителя для массива `exchange_ids`; решение `buy/sell` остается на стороне trader.
- Telemetry от trader (`heartbeat`, latency данные) считается источником для ранжирования.
- Для trade-задач используется full-sweep по всем требуемым биржам, а не выборочная latency по одной бирже.
- Resource-policy и assignment-quality параметры должны быть операционно наблюдаемы через `/metrics` и `/health`.
- Результаты выполнения (`trade.result`, `monitor.result`) передаются из `trader` в `cts-core` по WS как основной путь.
- REST для результатов допускается только как fallback/replay (recovery) и не используется как основной runtime-канал.

## 4. Следующий интеграционный блок

### Hybrid Core<->Trader Reconciliation

Цель:
- гарантировать автосходимость desired state (`cts-core`) и фактического набора задач на `trader`.

Подход:
- быстрый контур `10-15s` (configurable): DB scan + push обновлений,
- медленный контур `1-2m` (configurable): state request/report + checksum/version + corrective push.

Ожидаемый результат:
- push остается основным путем доставки,
- reconciliation выполняет self-healing роль,
- рассинхрон устраняется автоматически в пределах sync-интервала,
- причины и частота рассинхрона наблюдаемы.

## 5. Риски и контроль

- Риск рассинхронизации документации и runtime-поведения.
- Риск рассинхронизации контрактов `cts-core <-> trader` при активных изменениях.

Контроль:
- каждый новый WS action или изменение payload фиксируется одновременно в коде и `API_SPECIFICATION.md`,
- изменения runtime-семантики отражаются в `README.md` и `DOCS_INDEX.md` в том же наборе изменений.

## 6. История изменений

Исторические подробные поэтапные журналы в этот файл не включаются.

Для ретроспективы использовать git history:

```bash
git log -- DEVELOPMENT_PLAN.md
git show <commit> -- DEVELOPMENT_PLAN.md
```
