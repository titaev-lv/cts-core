# CTS-Core & Trader Development Plan

> Версия: 2.1.0
> Обновлено: 2026-03-14
> Статус: Phase 2 Complete | Phase 1.5 Finalization Priority
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
- В работе (текущий приоритет):
  - Phase 1.5 finalization: `/metrics`, Prometheus wiring, integration tests

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
  - `scripts/smoke_phase2_ws_lifecycle.sh`
  - `scripts/smoke_ws_lifecycle_client.go`

### Phase 1.5 Finalization (current priority)

- Реализовать `/metrics`.
- Добавить базовые runtime/db/hsm/ws метрики.
- Добавить интеграционные тесты для REST/WS health-path.

Code touchpoints:

- `cmd/cts-core/main.go`
- `internal/api/`
- `internal/middleware/`
- `internal/ws/`

Критерий завершения:
- `/health`, `/ready`, `/live`, `/metrics` доступны и покрыты тестами.

### Phase 3 (Business Logic)

- Load balancing/scoring.
- Resource tracking на основе `TRADER_EXCHANGE_RESOURCE`.
- Metrics/reporting для планировщика.

### Phase 4 (Integration)

- Расширенный REST API для admin/web.
- Полноценная обработка trade result потока.
- E2E/integration test suite.

## 4. Приоритетный backlog (ближайшие задачи)

1. Реализовать `/metrics` endpoint и связать с текущим runtime state/health.
2. Добавить Prometheus wiring и базовые runtime/db/hsm/ws метрики.
3. Добавить integration tests для health/ws smoke path.
4. Зафиксировать post-Phase-2 backlog для scheduler business logic (Phase 3).

## 5. Риски

- Риск рассинхронизации между API спецификацией и текущим WS stub.
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
