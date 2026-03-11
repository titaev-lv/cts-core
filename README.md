# CTS-Core: Crypto Trading System Core

Центральный оркестратор для распределенной системы арбитражной торговли.

## Текущий статус

- Кодовая база активна и собирается.
- Тесты проходят: `go test ./...`.
- Миграции применены в локальной БД (таблицы `TRADER`, `TRADER_SESSION`, `TRADER_EXCHANGE_RESOURCE`, `AUDIT_LOG`, `SCHEDULER_TASKS` существуют).
- Реализовано:
  - Config + logger
  - MySQL client + repositories
  - HSM clients (Trading + 2FA)
  - State manager (`state/daemon.state` + backup)
  - REST health endpoints: `/health`, `/ready`, `/live`
  - WS handler (базовый stub)
- Еще не завершено:
  - Полный WebSocket протокол (`trader.register`, heartbeat, commands)
  - Session manager и scheduler в runtime-слое
  - `/metrics` endpoint, Prometheus wiring и интеграционные тесты (отложено до рабочего WS runtime)

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

## Roadmap (операционный)

1. Закрыть Phase 2 runtime основу:
   - WS protocol layer
   - session lifecycle
   - scheduler skeleton
2. После Phase 2 закрыть Phase 1.5 finalization:
  - `/metrics`
  - базовые runtime-метрики
  - integration tests
3. Синхронизировать документацию с фактической реализацией.

## Связанные сервисы

- `services/hsm-service`
- `services/trader`
- `services/web-ui-go`

## Примечание

В старых документах могут встречаться ссылки на `other-sub-system/*` и статусы "Phase 0". Актуальная структура в этом workspace: `services/*`.
