# Changelog

## v0.0.1 - 2026-04-02

### Новые возможности
- Реализован Phase 2 WebSocket runtime lifecycle для Trader channel (`trader.register`, `trader.register_ack`, `trader.heartbeat`) с персистентностью runtime-сессий в `TRADER_SESSION`.
- Добавлен runtime loop шедуллера по активным WS-сессиям и latency sweep для профилирования связности.
- Добавлены операционные endpoint-ы и телеметрия: `/health`, `/ready`, `/live`, `/metrics`, `/api/v1/version`.
- Добавлена запись release-метаданных Trader из WS register payload в `TRADER.RELEASE_VERSION` для контроля версий флота.

### Исправления безопасности
- Для Trader WS закреплена строгая identity-политика: канонический `trader_id` берется только из CN клиентского mTLS-сертификата.
- Допуск в Trader WS ограничен клиентскими сертификатами с `OU=Trading`.
- Усилен WS ingress: проверка protocol version, inbound rate limiting, request dedup window, ограничение размера payload и защита от flood неизвестных action.

### Надежность
- Добавлена ротация логов при старте процесса: каждый запуск пишет в новые файлы.
- Входящий и исходящий WS-потоки объединены в единый runtime-файл `ws.log`.
- Выравнены дефолты живости сессий (`heartbeat_interval=60s`, `heartbeat_timeout=180s`, настраиваемый `session.write_timeout`).

### Сборка и релизы
- Добавлена startup-строка метаданных сборки: `INIT START cts-core` с `release`, `commit`, `build_time`.
- Политика release identity в Docker выровнена с Trader:
  - точный tag на `HEAD` => release build,
  - коммиты после последнего tag => `${last_tag}-dev.${commits_since_tag}+${utc_timestamp}.${short_sha}`,
  - отсутствие tag в репозитории => ошибка сборки.
- Семантическая версия сервиса (`main.version`) продолжает браться из `VERSION`.

### База данных и миграции
- Добавлена миграция `004_trader_release_version.sql` для `TRADER.RELEASE_VERSION`.
- Миграционные скрипты приведены к DBeaver-friendly исполнению (построчный сценарий).
- Bootstrap SQL init синхронизирован с инициализацией `RELEASE_VERSION`.

### Тестирование
- Расширены тесты логгера: startup-rotation, text-formatting поведение, объединенный WS-поток.
- Обновлены config-тесты под контракт единого WS-пути (`logging.ws_path`).
- Целевые suite-ы scheduler/logger/config/cmd проходят для релизного состояния.

### Документация
- Обновлены WS transport/API документы под `release` payload в register и контракт `protocol_version=1`.
- Тайминги runtime приведены к модели heartbeat `60s/180s`.
- Архитектурные заметки по логированию обновлены под единый поток `ws.log`.
