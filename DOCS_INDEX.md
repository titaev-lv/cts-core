# 📚 CTS-Core Documentation Index

> **Последнее обновление**: 2026-01-28  
> **Статус проекта**: 🟢 Phase 0 - Database Migrations  
> **Готовность**: 100% архитектуры

---

## 🎯 С чего начать

### Если вы новый разработчик:
1. 📖 **[README.md](README.md)** - Обзор проекта
2. 📋 **[DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)** - НАЧНИТЕ ЗДЕСЬ (Phase 0-4)
3. 🏗️ **[ARCHITECTURE.md](ARCHITECTURE.md)** - Архитектура + все решения
4. 🔐 **[HSM_KEY_ROTATION.md](HSM_KEY_ROTATION.md)** - HSM key rotation guide

### Если хотите понять API:
1. 🔌 **[API_SPECIFICATION.md](API_SPECIFICATION.md)** - Единый API (WebSocket + REST)

### Если работаете с трейдерами:
1. 🔄 **[TRADER_MODES.md](TRADER_MODES.md)** - TRADE vs MONITOR режимы

---

## 📁 Структура документации

### ✅ Основные документы

| Документ | Описание | Статус | Дата |
|----------|----------|--------|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Полная архитектура + 25 Phase 1 решений | ✅ Завершено | 2026-01-28 |
| [API_SPECIFICATION.md](API_SPECIFICATION.md) | Единый API: WebSocket + REST (27 error codes) | ✅ Завершено | 2026-01-28 |
| [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) | План разработки Phase 0-4 | ✅ Завершено | 2026-01-28 |
| [TRADER_MODES.md](TRADER_MODES.md) | Dual-mode operation (TRADE + MONITOR) | ✅ Завершено | 2026-01-27 |
| [HSM_KEY_ROTATION.md](HSM_KEY_ROTATION.md) | HSM key rotation & re-encryption guide | ✅ Завершено | 2026-01-28 |
| [CONTEXT.md](CONTEXT.md) | Контекст для AI/новых разработчиков | ✅ Завершено | 2026-01-26 |
| [migrations/](migrations/) | SQL schema migrations (11 tables) | ✅ Готово | 2026-01-28 |

---

---

### 📖 Справочные документы

| Документ | Описание | Использование |
|----------|----------|---------------|
| [gpt.txt](gpt.txt) | Исходная спецификация (БД, стратегии) | Reference |
| [shema-go.txt](shema-go.txt) | Схема Go-проекта | Reference |

---

## 🚦 Статус готовности

### Готово ✅

- [x] Архитектурная документация (ASCII + Mermaid диаграммы)
- [x] WebSocket protocol спецификация
- [x] REST API спецификация (27 error codes)
- [x] TRADE/MONITOR режимы документированы
- [x] Security design (HSM, mTLS, envelope encryption, key rotation)
- [x] Plan разработки с фазами
- [x] **Все 25 архитектурных решений Phase 1**
- [x] **Trader registration** (Hybrid: admin pre-register + auto-connect)
- [x] **State persistence** (daemon.state + MySQL sync)
- [x] **Idempotency** (UNIQUE constraints + exchange_order_id)
- [x] **SQL migrations** (11 tables including HSM key rotation)
- [x] **Failover design** (Single instance + trader resilience)
- [x] **Error handling** (27 standardized codes with retry policies)
- [x] **Observability** (Prometheus 20+ metrics, zerolog, audit log)
- [x] **HSM key rotation** (Complete infrastructure with re-encryption)

### Текущая фаза 🔵

- [ ] **Phase 0**: Применить миграции `mysql < migrations/001_phase1_schema.sql`
- [ ] **Phase 1**: Начать разработку (Project setup, MySQL pool, HSM client)

### Можно отложить ⏳

- [ ] Distributed CTS-Core cluster (etcd/Consul)
- [ ] Advanced load balancing
- [ ] Auto-scaling трейдеров
- [ ] Chaos testing
- [ ] Performance tuning

---

## 🗺️ Roadmap

```
Phase 0: Database Migrations (~1 час) 🔵 ТЕКУЩАЯ ФАЗА
├─ Применить migrations/001_phase1_schema.sql
├─ Проверить создание 11 таблиц
└─ Готовность к Phase 1

Phase 1: Foundation (2 недели)
├─ Project setup (go.mod, config, logger)
├─ MySQL connection pool
├─ HSM client (mTLS)
└─ Basic REST API server

Phase 2: Core Features (1.5 недели)
├─ WebSocket server (traders)
├─ Session manager
├─ Task scheduler (basic)
└─ Heartbeat & health check

Phase 3: Business Logic (2 недели)
├─ DB Proxy (encrypted credentials)
├─ Latency analyzer
├─ Task assignment algorithm
└─ Metrics collector

Phase 4: Integration (2 недели)
├─ WebSocket for web (admin)
├─ Full REST API
├─ Trade result processing
└─ Integration testing

Phase 1.5 (optional): Failover (1 неделя)
└─ Active-Passive CTS-Core setup
```

---

## 🔗 Связи между документами

```
ARCHITECTURE.md (25 Phase 1 decisions)
    ↓
    ├─→ API_SPECIFICATION.md (trade.intent, REST endpoints, 27 error codes)
    ├─→ DEVELOPMENT_PLAN.md (Phase 0-4 roadmap)
    ├─→ TRADER_MODES.md (TRADE vs MONITOR)
    ├─→ HSM_KEY_ROTATION.md (key rotation workflow)
    └─→ migrations/ (11 tables DDL)

ARCHITECTURE.md
    ↓
    ├─→ TRADER_MODES.md (TRADE vs MONITOR)
    ├─→ API_SPECIFICATION.md (WebSocket protocol)
    └─→ DEVELOPMENT_PLAN.md (фазы реализации)

API_SPECIFICATION.md
    ↓
    └─→ Единый источник истины для всех API

TRADER_MODES.md
    ↓
    ├─→ ARCHITECTURE.md (архитектурные решения)
    └─→ API_SPECIFICATION.md (task.assign messages)
```

---

## 🎓 Ключевые концепции

### Архитектурные решения

1. **Distributed Architecture**: CTS-Core (orchestrator) + 25+ Trader daemons
2. **Security First**: mTLS everywhere, envelope encryption (KEK/DEK), OU-based ACL
3. **Low Latency**: WebSocket для всей коммуникации, прямой доступ трейдеров к ClickHouse
4. **Dual-Mode Trading**: TRADE (MySQL) и MONITOR (ClickHouse) независимы
5. **Resource Management**: Отслеживание WS лимитов (5-10 per exchange, 30-40 subscriptions per WS)

### Критические потоки

**Trade Execution:**
```
Trader → finds opportunity → trade.intent (request) →
CTS-Core → approves → arbitrage_id →
Trader → executes orders → trade.result (event) →
CTS-Core → writes to MySQL (ARBITRAGE_TRANS)
```

**Trader Lifecycle:**
```
Trader → connects (mTLS) → trader.register →
CTS-Core → validates → session_id →
⟳ Heartbeat every 5 sec →
CTS-Core → task.assign →
Trader → executes →
... →
CTS-Core → trader.shutdown (graceful) →
Trader → cleanup → disconnect
```

---

## 🆘 Часто задаваемые вопросы

### Q: Можно ли начинать кодировать?

**A: ДА.** Примените миграции и начинайте с [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) Phase 1.

### Q: Какой самый критичный вопрос?

**A: Применение миграций** - `mysql -u root -proot -h 127.0.0.1 ct_system < migrations/001_phase1_schema.sql`

### Q: Почему dual-mode (TRADE + MONITOR)?

**A:** См. [TRADER_MODES.md](TRADER_MODES.md). Они пишут в РАЗНЫЕ БД (MySQL vs ClickHouse) и полностью независимы.

### Q: Как трейдеры получают credentials?

**A:** CTS-Core передает encrypted DEK + encrypted API keys. Трейдер расшифровывает через HSM напрямую (OU=Trading). См. [ARCHITECTURE.md](ARCHITECTURE.md) раздел 6.

### Q: WebSocket или REST для trade results?

**A:** WebSocket (см. [API_SPECIFICATION.md](API_SPECIFICATION.md) - trade.result event). REST только для CRUD операций.

---

## 📞 Контакты

**Проект**: CTS-Core (Crypto Trading System Core)  
**Версия документации**: 1.0.0  
**Дата последнего обновления**: 2026-01-27

---

## ⚡ Quick Commands

```bash
# Просмотр структуры документации
ls -la *.md

# Поиск по всем документам
grep -r "trader.register" *.md

# Применить миграции (Phase 0)
mysql -u root -proot -h 127.0.0.1 ct_system < migrations/001_phase1_schema.sql

# Проверить создание таблиц
mysql -u root -proot -h 127.0.0.1 ct_system -e "SHOW TABLES;"
```

---

**✅ ГОТОВО к Phase 1 - начинайте с [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)**

**✅ ГОТОВО к Phase 1 - начинайте с [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)**
