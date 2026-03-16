# CTS-Core & Trader Development Plan

> Версия: 2.2.0
> Обновлено: 2026-03-14
> Статус: Phase 2 Complete | Phase 1.5 Complete | Phase 3 Priority
> Связанные документы: `ARCHITECTURE.md`, `API_SPECIFICATION.md`, `DOCS_INDEX.md`, `CROSS_PROJECT_WWW_GO.md`

## 1. Цель

Документ фиксирует актуальный план разработки без исторических пошаговых инструкций.

## 2. Текущее состояние

### CTS-Core

- Завершено:
  - Phase 0: SQL schema и миграции
  - Phase 1.1: config/logger/project setup
  - Phase 1.2: MySQL client + repository layer
  - Phase 1.3: HSM client (Trading + 2FA)
  - Phase 1.4: state manager (`state/daemon.state`, backup, background sync)
  - Phase 2: WS runtime protocol + session lifecycle + scheduler skeleton + Sprint 6 smoke tooling
  - Phase 1.5 finalization: `/metrics`, Prometheus wiring, integration coverage

### База данных (локальный контур)

- Ключевые таблицы существуют: `TRADER`, `TRADER_SESSION`, `TRADER_EXCHANGE_RESOURCE`, `AUDIT_LOG`, `SCHEDULER_TASKS`.
- `SCHEDULER_TASKS` инициализирована дефолтными задачами.

### Trader / Web UI

- Текущее направление для trader в workspace: `services/trader`.
- По интеграции с UI приоритеты и контракт вынесены в `CROSS_PROJECT_WWW_GO.md`.

## 3. План по фазам

### Phase 2 (Core Features, completed)

Результат:
- Реализованы `trader.register` и `trader.heartbeat` (request/event, ack/error).
- Реализован lifecycle сессий с persistence в `TRADER_SESSION`.
- Реализован scheduler runtime cycle по active/healthy WS snapshots.
- Добавлен protocol hardening (version checks, rate limit, dedup, payload and action flood guards).
- Добавлен Sprint 6 operational smoke набор:
  - `guides/PHASE2_SMOKE_RUNBOOK.md`
  - `tests/smoke_phase2_ws_lifecycle.sh`
  - `tests/smoke_ws_lifecycle_client.go`

### Phase 1.5 Finalization (completed)

Результат:
- Реализован `/metrics` endpoint на REST роутере (конфигурируемый путь).
- Добавлены Prometheus collectors:
  - Go runtime/process
  - `cts_core_ws_active_connections`
  - `cts_core_ws_total_connections`
  - `cts_core_runtime_scheduler_cycle_count`
  - `cts_core_runtime_scheduler_last_candidate_count`
  - `cts_core_runtime_ws_timeout_count`
- Добавлены тесты:
  - unit coverage для metrics endpoint/path normalization
  - integration test `health + metrics + ws lifecycle`

Code touchpoints:

- `cmd/cts-core/main.go`
- `internal/api/`
- `internal/middleware/`
- `internal/ws/`

Критерий завершения:
- `/health`, `/ready`, `/live`, `/metrics` доступны и покрыты тестами.

### Phase 3 (Business Logic)

- Зафиксированная модель выбора trader для массива бирж:
  - CTS-Core выбирает исполнителя только для `exchange_ids`; решение `buy/sell` принимает trader.
  - Для `task_type=trade`:
    - eligibility по режиму trader: только `trade` и `both`.
    - режим `monitor` не участвует в выборе исполнителя для trade-задач.
    - latency считается как профиль по требуемым биржам с защитой от выбросов (не простое среднее).
    - нагрузка берется из `load_index` (0..1), который репортит trader в heartbeat.
    - формула ранга: `score_trade = latency_profile_ms + 1000 * (load_index^2)`.
  - Для `task_type=monitor`:
    - приоритет у минимальной торговой загрузки trader; latency не участвует в ранжировании.
    - приоритет по режиму trader: `monitor` > `both` > `trader`.
    - формула ранга: `score_monitor = 1000 * (trade_load_index^2) + monitor_capacity_penalty + monitor_role_penalty`.
    - `primary` = минимум `score_monitor`, `backup` = второй минимум.
  - Детализация формул (v1):
    - `latency_profile_ms`: агрегат задержек по всем требуемым биржам с учетом worst/p95 и разброса между биржами (только для `trade`).
    - `load_index`: нормированный индекс общей загрузки trader (`0..1`) из CPU/RAM/network/queue.
    - `trade_load_index`: нормированный индекс загрузки торговыми задачами (`0..1`) для monitor-выбора.
    - `monitor_role_penalty`: штраф за режим (`monitor=0`, `both=100`, `trader=300` в версии по умолчанию).
    - `*_index^2`: нелинейный штраф near saturation.
    - `1000`: масштабирование компонента загрузки в единицы, сопоставимые с `latency_ms`.
  - Trader после подключения обязан слать телеметрию (`trader.heartbeat` + `metrics.report`), CTS-Core агрегирует ее для ранжирования.
  - Для `task_type=trade` latency sweep выполняется по всем биржам из `capabilities`; данные по одной бирже не формируют корректный профиль.
  - Для `task_type=monitor` latency в формуле ранга не используется.
- Load balancing/scoring.
- Resource tracking на основе `TRADER_EXCHANGE_RESOURCE`.
- Metrics/reporting для планировщика.

### Phase 4 (Integration)

- Расширенный REST API для admin/web.
- Полноценная обработка trade result потока.
- E2E/integration test suite.

### Phase 4.1 (Integration): Hybrid Core<->Trader Reconciliation

Цель:
- не допустить длительного рассинхрона между desired state в `cts-core` и фактическими задачами на `trader`.

Подход (гибрид):
- быстрый контур `10-15s` (настраиваемо):
  - `cts-core` сканирует БД на новые/остановленные задачи,
  - пересчитывает desired state,
  - отправляет push-обновления на `trader`.
- медленный контур `1-2m` (настраиваемо):
  - `cts-core` запрашивает у каждого активного `trader` текущий список задач,
  - `trader` отвечает списком + checksum/version,
  - `cts-core` сверяет actual vs desired и при расхождении отправляет корректирующий push.

Критерий готовности:
- push остается основным путем доставки изменений,
- reconciliation работает как self-healing контроль,
- при ручной остановке/удалении задачи рассинхрон автоматически устраняется в пределах sync-интервала,
- причины рассинхрона и количество коррекций видны в логах/метриках.

## 4. Приоритетный backlog (ближайшие задачи)

1. Развивать scheduler business logic и scoring (Phase 3).
2. Расширить модель runtime metrics под assignment quality/latency.
3. Добавить integration/e2e сценарии task dispatch/results.
4. Зафиксировать post-Phase-2 backlog в разрезе trader orchestration.

## 5. Риски

- Риск рассинхронизации между API спецификацией и текущим WS runtime baseline.
- Риск рассинхронизации между docs и кодом при быстрых изменениях.

Снижение рисков:
- Любой новый endpoint/WS action фиксировать одновременно в коде и `API_SPECIFICATION.md`.
- Обновлять `README.md` и `DOCS_INDEX.md` в том же PR.

## 6. Исторические секции

Исторические пошаговые секции (minute-by-minute, длинные командные блоки, архивные rollout/checklists) намеренно удалены из этого файла.

Если нужна ретроспектива реализации, используйте git history:

```bash
git log -- DEVELOPMENT_PLAN.md
git show <commit> -- DEVELOPMENT_PLAN.md
```
