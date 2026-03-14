# CTS-Core & Trader Development Plan

> Версия: 2.0.0
> Обновлено: 2026-03-10
> Статус: Phase 1.4 Complete | Phase 2 Priority | Phase 1.5 Finalization Deferred
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
- Частично завершено:
  - Phase 1.5 foundation: REST/WS base (`/health`, `/ready`, `/live`, middleware, WS stub)
- Отложено до Phase 2 WS runtime:
  - `/metrics` endpoint
  - Prometheus wiring
  - REST/WS integration tests

### База данных (локальный контур)

- Ключевые таблицы существуют: `TRADER`, `TRADER_SESSION`, `TRADER_EXCHANGE_RESOURCE`, `AUDIT_LOG`, `SCHEDULER_TASKS`.
- `SCHEDULER_TASKS` инициализирована дефолтными задачами.

### Trader / Web UI

- Текущее направление для trader в workspace: `services/trader`.
- По интеграции с UI приоритеты и контракт вынесены в `CROSS_PROJECT_WWW_GO.md`.

## 3. План по фазам

### Phase 2 (Core Features, current priority)

- WebSocket protocol layer:
  - `trader.register`
  - `trader.heartbeat`
  - protocol errors/ack
- Session manager:
  - lifecycle сессии
  - timeout/disconnect причины
- Task scheduler skeleton:
  - базовый assignment cycle
  - учет статуса трейдеров

Критерий завершения:
- трейдер может пройти полный базовый цикл connect -> register -> heartbeat -> disconnect.

Операционный smoke-runbook (Sprint 6):
- `guides/PHASE2_SMOKE_RUNBOOK.md`
- helper script: `scripts/smoke_phase2_ws_lifecycle.sh`

### Phase 1.5 Finalization (after Phase 2 WS protocol)

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

1. Поднять WS protocol state machine (минимальный контракт из `API_SPECIFICATION.md`).
2. Реализовать runtime session lifecycle.
3. Добавить scheduler runtime skeleton.
4. После WS runtime: добавить `/metrics` + exporter и integration tests.

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
