# Phase 1.4: State Management (ACTUAL)

**Статус документа:** актуализирован под текущее состояние `cts-core`.

**Цель Phase 1.4:**
- добавить persistent state (`state/daemon.state`),
- обеспечить безопасное сохранение/восстановление,
- подготовить основу для Session/WS слоёв Phase 2.

---

## 1) Что актуально сейчас

- Точка входа сервиса: `cmd/cts-core/main.go`.
- Конфиг уже содержит секцию `state`:
  - `file_path`
  - `sync_interval`
  - `backup_count`
- В `main.go` уже есть место интеграции: `TODO: Phase 1.4 - Load state`.
- Health endpoint (`/health`) уже существует и отдает расширенную телеметрию.

---

## 2) Scope Phase 1.4 (что делаем)

### 2.1 Обязательное

1. Создать пакет `internal/state`.
2. Реализовать `StateManager` для файла состояния:
   - load при старте,
   - save по таймеру и при shutdown,
   - atomic write (`.tmp` -> `rename`),
   - versioned JSON.
3. Интегрировать `StateManager` в `cmd/cts-core/main.go`.
4. Добавить unit-тесты для load/save/recovery.

### 2.2 Что не входит в эту фазу

- Полная бизнес-модель трейдеров/сессий из WebSocket Phase 2.
- Детальная агрегация `ping/pong` (это Priority 3 / Phase 2.1-2.2).
- CI/CD и расширенная операционная документация (перенесено в root `Priority 5.5`).

---

## 3) Минимальная целевая структура

```text
internal/state/
  types.go          # структуры состояния + версия формата
  manager.go        # lifecycle: Load/Save/Start/Close
  persistence.go    # atomic write, backup, cleanup
  manager_test.go   # unit tests
```

---

## 4) Контракт состояния (MVP)

```go
// Versioned root object
{
  "version": "1.0",
  "updated_at": "2026-02-27T00:00:00Z",
  "server": {
    "started_at": "...",
    "status": "running"
  },
  "runtime": {
    "active_ws_connections": 0,
    "last_ws_connect_unix": 0
  }
}
```

### Обязательные требования

- `version` обязателен.
- Время в RFC3339 UTC.
- Запись файла только атомарная.
- При поврежденном JSON — fallback к пустому state + warning в лог.

---

## 5) Интеграция в `main.go`

Порядок инициализации в текущем процессе:

1. Load config
2. Init logger
3. Init DB
4. Init HSM clients
5. **Init StateManager (Phase 1.4)**
   - `Load()`
   - `StartBackgroundSync()`
6. Start REST/WS server
7. On shutdown:
   - `StateManager.Close()` (финальный save)
   - shutdown HTTP server

---

## 6) Тест-кейсы (обязательные)

1. `Load()` при отсутствии файла -> пустой state, без ошибки.
2. `Save()` создает корректный JSON.
3. Atomic write не оставляет битый основной файл.
4. Backup rotation удаляет старые backup-файлы сверх лимита.
5. Поврежденный JSON -> graceful recovery (fallback + warning).

---

## 7) Связь с root планом

Этот guide покрывает задачи из root `DEVELOPMENT_PLAN.md`:

- `Реализовать StateManager (load/save daemon.state)`
- `State format: JSON с версионированием`
- `Atomic writes (write → rename)`
- `Tests: state save/load/recovery`

Связанные задачи, но **следующих фаз**:

- WS heartbeat aggregation для trader health -> Priority 3 (Phase 2.1/2.2)
- Документация Debian PROD / watchdog / CI-CD -> Priority 5.5

---

## 8) Definition of Done (Phase 1.4)

- [ ] `internal/state` создан и используется в `main.go`
- [ ] `state/daemon.state` создается и обновляется в runtime
- [ ] graceful shutdown сохраняет финальное состояние
- [ ] unit tests для `internal/state` проходят
- [ ] поведение recovery документировано в этом guide

---

## 9) Примечания

- Не возвращаем полный `daemon.state` в публичные endpoint'ы без отдельного контроля доступа.
- Для web-ui используем агрегированные данные из `/health`, а не сырой state dump.
