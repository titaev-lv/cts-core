# 📚 CTS-Core Documentation Index

> **Последнее обновление**: 2026-01-27  
> **Статус проекта**: 🔴 Требуется Phase 0.5 (Architecture Hardening)  
> **Готовность**: 65%

---

## 🎯 С чего начать

### Если вы новый разработчик:
1. 📖 **[README.md](README.md)** - Обзор проекта
2. 🏗️ **[ARCHITECTURE.md](ARCHITECTURE.md)** - Архитектура системы
3. 🚨 **[BEFORE_START.md](BEFORE_START.md)** - ⚠️ КРИТИЧЕСКИЕ вопросы (ОБЯЗАТЕЛЬНО к прочтению)
4. 📋 **[DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)** - План разработки

### Если хотите понять API:
1. 🔌 **[API_SPECIFICATION.md](API_SPECIFICATION.md)** - Единый API (WebSocket + REST)

### Если работаете с трейдерами:
1. 🔄 **[TRADER_MODES.md](TRADER_MODES.md)** - TRADE vs MONITOR режимы

---

## 📁 Структура документации

### 🔴 Критические документы (требуют действий)

| Документ | Описание | Статус | Приоритет |
|----------|----------|--------|-----------|
| [BEFORE_START.md](BEFORE_START.md) | 12 критических вопросов без ответа | 🔴 Блокер | **НЕМЕДЛЕННО** |

**Основные блокеры:**
- Механизм регистрации трейдеров (automatic vs manual vs hybrid)
- State persistence strategy (MySQL vs Redis vs file)
- Idempotency для trade operations (trade.intent flow)
- Недостающие SQL таблицы (TRADER, MONITOR_PAIR, EXCHANGE_LIMITS, etc.)
- Failover design (Active-Passive minimum)

---

### ✅ Завершенные документы

| Документ | Описание | Дата |
|----------|----------|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Полная архитектура (диаграммы, компоненты, безопасность) | 2026-01-26 |
| [API_SPECIFICATION.md](API_SPECIFICATION.md) | Единый API: WebSocket + REST | 2026-01-27 |
| [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) | План разработки с Gantt-диаграммами | 2026-01-26 |
| [TRADER_MODES.md](TRADER_MODES.md) | Dual-mode operation (TRADE + MONITOR) | 2026-01-27 |
| [CONTEXT.md](CONTEXT.md) | Контекст для AI/новых разработчиков | 2026-01-26 |

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
- [x] REST API спецификация
- [x] TRADE/MONITOR режимы документированы
- [x] Security design (HSM, mTLS, envelope encryption)
- [x] Plan разработки с фазами

### Требует решения 🔴

- [ ] **Trader registration mechanism** (см. BEFORE_START.md #1)
- [ ] **State persistence strategy** (см. BEFORE_START.md #2)
- [ ] **Idempotency guarantees** (см. BEFORE_START.md #4)
- [ ] **SQL migrations** для недостающих таблиц (см. BEFORE_START.md #3)
- [ ] **Failover design** (Active-Passive minimum)
- [ ] **Error handling & retry policies** (см. BEFORE_START.md #6)
- [ ] **Observability** (metrics, logging) (см. BEFORE_START.md #7)

### Можно отложить ⏳

- [ ] Distributed CTS-Core cluster (etcd/Consul)
- [ ] Advanced load balancing
- [ ] Auto-scaling трейдеров
- [ ] Chaos testing
- [ ] Performance tuning

---

## 🗺️ Roadmap

```
Phase 0.5: Architecture Hardening (1-2 недели) 🔴 ТЕКУЩАЯ ФАЗА
├─ Решить все вопросы из BEFORE_START.md
├─ SQL migrations для новых таблиц
├─ Финализировать API specification
└─ State management design

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
BEFORE_START.md (блокеры)
    ↓
    ├─→ API_SPECIFICATION.md (trade.intent, REST endpoints)
    ├─→ ARCHITECTURE.md (state management, failover)
    ├─→ DEVELOPMENT_PLAN.md (Phase 0.5 добавлена)
    └─→ CONTEXT.md (ссылки на блокеры)

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

**A: НЕТ.** Сначала нужно закрыть все блокеры в [BEFORE_START.md](BEFORE_START.md).

### Q: Какой самый критичный вопрос?

**A: Trader registration mechanism** (см. BEFORE_START.md #1.1). От этого зависят таблицы БД и API flows.

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

# Проверка блокеров
grep -A 5 "БЛОКЕР" BEFORE_START.md

# Генерация SQL migrations (когда готово)
# cd other-sub-system/daemon2/
# mysql -u root -p < migrations/001_trader_tables.sql
```

---

**🔴 ВАЖНО: НЕ начинайте Phase 1 пока не закрыты все блокеры в [BEFORE_START.md](BEFORE_START.md)**
