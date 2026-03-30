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
- Завершена Phase 1.5 finalization:
  - `/metrics` endpoint
  - Prometheus wiring c runtime series (`cts_core_*`) + Go/process collectors
  - integration coverage: health + metrics + ws lifecycle path
- WS observability delta:
  - Добавлены метрики отключений: `cts_core_ws_disconnect_total`, `cts_core_ws_disconnect_close_4009_total`, `cts_core_ws_disconnect_reason_total{reason="..."}` для отслеживания reconnect/close-4009.
  - Сервер при shutdown шлет WS Close (1000) с reason `server_shutdown`, write-deadline унифицирована и настраивается через `session.write_timeout` (дефолт 10s, WAN-friendly) для всех send/control кадров.

Настройка WS write timeout:

- YAML: `session.write_timeout` (пример в `conf/config.example.yaml` = `10s`).
- Рекомендуемый диапазон: `> 0` и `<= 24h`.

## WS Identity Policy (Trader Channel)

- `trader_id` определяется только из CN клиентского сертификата (mTLS).
- `payload.trader_id` не участвует в идентификации.
- Для trader WS принимаются только клиентские сертификаты с `OU=Trading`.
- CN должен быть уникальным в `TRADER.CERTIFICATE_CN`.
- При ротации сертификата CN должен оставаться неизменным; иначе требуется отдельная миграция identity.
- Для статусов `pending` / `active` / `suspended` runtime-логика привязана к CN-derived identity.

Быстрый check `/metrics`:

```bash
curl -k -sS https://localhost:8080/metrics || curl -sS http://localhost:8081/metrics
```

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

Из корня workspace можно запускать через единый entrypoint:

```bash
make smoke-core-ws
```

Этот smoke-сценарий проверяет hard-cutover модель:
- только `wss://` transport для WS;
- mTLS handshake с клиентским сертификатом трейдера;
- auto-create трейдера по CN сертификата;
- duplicate CN конфликт: второе одновременное подключение с тем же сертификатом получает `DUPLICATE_CONNECTION`;
- новый трейдер получает `status=pending`;
- pending-трейдер не получает assignment в `MONITORING` / `TRADE`.

По умолчанию smoke-скрипт невмешивающийся:
- `SMOKE_SKIP_UP=1` (не делает `docker compose up -d`)
- `SMOKE_NO_RESTART=1` (не делает restart/stop/start)

Полный инвазивный режим:

```bash
SMOKE_SKIP_UP=0 SMOKE_NO_RESTART=0 ./tests/smoke_phase2_ws_lifecycle.sh
```

## Roadmap (операционный)

1. Расширять Phase 3 business logic (assignment/resource-aware scheduling) по фиксированному правилу:
  - CTS-Core выбирает trader для массива `exchange_ids`, но не принимает решение `buy/sell`.
  - `buy/sell` и локальная стратегия исполнения определяются внутри trader.
  - Для `task_type=trade` используется latency-aware ранг:
    - Eligibility: к выбору допускаются только trader в режимах `trade` и `both`.
    - Trader в режиме `monitor` исключается из кандидатов для `trade`.
    - `score_trade = latency_profile_ms + 1000 * (load_index^2)` (меньше = лучше).
    - `latency_profile_ms` = робастный профиль задержек по всем `exchange_ids` (worst/p95 + разброс).
    - `load_index` (`0..1`) формируется на стороне trader (CPU + RAM + network + очередь задач).
  - Для `task_type=monitor` используется отдельный ранг без latency:
    - `score_monitor = 1000 * (trade_load_index^2) + monitor_capacity_penalty + monitor_role_penalty` (меньше = лучше).
    - `trade_load_index` (`0..1`) отражает загрузку trader именно торговыми задачами.
    - Приоритет по режиму: `monitor` > `both` > `trader`.
    - Пример `monitor_role_penalty`: `monitor=0`, `both=100`, `trader=300`.
    - latency подключения к биржам для monitor-выбора не учитывается.
    - `primary` выбирается по минимальному `score_monitor`, `backup` - второй в ранге.
  - Trader после подключения регулярно отправляет телеметрию (`trader.heartbeat` + `metrics.report`), CTS-Core агрегирует ее для scheduler.
  - После `trader.register_ack` CTS-Core отдает trader каталог доступных бирж (`exchange_id`, `code`, `name`, endpoints, market types, limits); trader выполняет connectivity test и присылает стартовые метрики.
  - Для `task_type=trade` латентность проверяется по всем биржам из `capabilities`; выбор по одной бирже недостаточен.
  - Для `task_type=monitor` latency в ранжировании не участвует.
2. Развивать runtime metrics/reporting для scheduler quality signals.
3. Поддерживать docs parity с кодом в каждом PR.

## Связанные сервисы

- `services/hsm-service`
- `services/trader`
- `services/web-ui-go`

## Примечание

В старых документах могут встречаться ссылки на `other-sub-system/*` и статусы "Phase 0". Актуальная структура в этом workspace: `services/*`.
