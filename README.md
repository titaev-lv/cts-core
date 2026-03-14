# CTS-Core: Crypto Trading System Core

Центральный оркестратор для распределенной системы арбитражной торговли.

## Текущий статус

- Кодовая база активна, собирается и покрыта тестами.
- Завершена Phase 2 (Sprint 1-6):
  - WS runtime lifecycle: `connect -> trader.register -> trader.heartbeat -> disconnect`
  - Runtime session lifecycle + persistence в `TRADER_SESSION`
  - Базовый scheduler cycle на активных WS-сессиях
  - WS hardening: protocol version check, rate limiting, dedup, payload bounds, unknown-action flood guard
  - Phase 2 smoke tooling: runbook + lifecycle smoke script + deterministic WS smoke client
- Текущий приоритет: Phase 1.5 finalization (`/metrics`, Prometheus wiring, integration tests).

## Быстрый старт

```bash
# 1) deps
go mod download

# 2) config
cp conf/config.example.yaml conf/config.yaml

# 3) build
make build

# 4) run
./bin/cts-core -config conf/config.yaml
```

Для локальной проверки БД:

```bash
mysql -h 127.0.0.1 -u root -proot -e "USE ct_system; SHOW TABLES;"
```

## Документация

Стартовые документы:

- `DOCS_INDEX.md` - навигация по актуальным документам
- `DEVELOPMENT_PLAN.md` - детальный план (частично устарел, сверяйте с кодом)
- `ARCHITECTURE.md` - архитектурные решения
- `API_SPECIFICATION.md` - целевой API контракт
- `HSM_KEY_ROTATION.md` - текущее состояние ротации ключей
- `CROSS_PROJECT_WWW_GO.md` - интеграция `cts-core <-> web-ui-go`
- `guides/PHASE2_SMOKE_RUNBOOK.md` - smoke-проверка Phase 2 в docker compose

Быстрый smoke запуск (Phase 2):

```bash
./tests/smoke_phase2_ws_lifecycle.sh
```

По умолчанию smoke-скрипт невмешивающийся:
- `SMOKE_SKIP_UP=1` (не делает `docker compose up -d`)
- `SMOKE_NO_RESTART=1` (не делает restart/stop/start)

Полный инвазивный режим:

```bash
SMOKE_SKIP_UP=0 SMOKE_NO_RESTART=0 ./tests/smoke_phase2_ws_lifecycle.sh
```

## Roadmap (операционный)

1. Закрыть Phase 1.5 finalization:
  - `/metrics`
  - базовые runtime-метрики
  - Prometheus wiring
  - integration tests
2. Расширять Phase 3 business logic (assignment/scoring/resource-aware scheduling).
3. Поддерживать docs parity с кодом в каждом PR.

## Связанные сервисы

- `services/hsm-service`
- `services/trader`
- `services/web-ui-go`

## Примечание

В старых документах могут встречаться ссылки на `other-sub-system/*` и статусы "Phase 0". Актуальная структура в этом workspace: `services/*`.
