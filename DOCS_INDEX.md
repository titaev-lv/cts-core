# CTS-Core Documentation Index

Последнее обновление: 2026-03-10

## Текущий срез

- Кодовая реализация: Phase 1.4 завершен, Phase 1.5 частично.
- В проекте уже есть runtime-сервис, state manager, health REST endpoints и WS stub.
- Часть старых документов содержит статусы "Phase 0" и старые пути. Используйте этот индекс как точку входа.

## С чего начать

1. `README.md` - короткий operational summary.
2. `DEVELOPMENT_PLAN.md` - полный план фаз (проверяйте с реальным кодом).
3. `ARCHITECTURE.md` - архитектурные решения и ограничения.
4. `API_SPECIFICATION.md` - целевой контракт WS/REST.

## Актуальные документы

- `README.md`
- `DEVELOPMENT_PLAN.md`
- `ARCHITECTURE.md`
- `API_SPECIFICATION.md`
- `HSM_KEY_ROTATION.md`
- `TRADER_MODES.md`
- `RATE_LIMITS_ARCHITECTURE.md`
- `TEST_COVERAGE_MODELS.md`
- `CROSS_PROJECT_WWW_GO.md`
- `guides/phase_1_4_state_management.md`
- `guides/phase_1_5_rest_server.md`

## Документы с риском устаревания

- `DEVELOPMENT_PLAN.md`:
    - содержит старые чек-листы "in progress" для уже сделанных частей.
- `guides/phase_1_5_rest_server.md`:
    - включает шаблонные шаги, не полностью совпадающие с текущим кодом.

Примечание: `CONTEXT.md` удален как устаревший дубликат. Его роль закрыта `README.md` + `DOCS_INDEX.md` + профильными документами.

## Что считать источником истины

1. Сначала код в `cmd/` и `internal/`.
2. Затем миграции в `migrations/`.
3. Потом документы.

При конфликте документации и кода приоритет у кода и схемы БД.

## Быстрые проверки

```bash
# tests
go test ./...

# наличие ключевых таблиц
mysql -h 127.0.0.1 -u root -proot -e "USE ct_system; SHOW TABLES LIKE 'TRADER'; SHOW TABLES LIKE 'TRADER_SESSION'; SHOW TABLES LIKE 'AUDIT_LOG';"

# количество записей в интеграционных таблицах
mysql -h 127.0.0.1 -u root -proot -e "USE ct_system; SELECT 'TRADER' t, COUNT(*) c FROM TRADER UNION ALL SELECT 'TRADER_SESSION', COUNT(*) FROM TRADER_SESSION UNION ALL SELECT 'TRADER_EXCHANGE_RESOURCE', COUNT(*) FROM TRADER_EXCHANGE_RESOURCE UNION ALL SELECT 'AUDIT_LOG', COUNT(*) FROM AUDIT_LOG UNION ALL SELECT 'SCHEDULER_TASKS', COUNT(*) FROM SCHEDULER_TASKS;"
```

## Связанные сервисы в workspace

- `services/hsm-service`
- `services/trader`
- `services/web-ui-go`

Для интеграции с UI используйте `CROSS_PROJECT_WWW_GO.md`.
