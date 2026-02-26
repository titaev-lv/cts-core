# CTS-Core: Crypto Trading System Core

> **Центральный оркестратор для распределённой системы арбитражной торговли криптовалютами**

[![Status](https://img.shields.io/badge/status-ready-green)](DEVELOPMENT_PLAN.md)
[![Architecture](https://img.shields.io/badge/architecture-complete-green)](ARCHITECTURE.md)
[![API](https://img.shields.io/badge/API-specified-green)](API_SPECIFICATION.md)
[![Phase](https://img.shields.io/badge/phase-0_migrations-blue)](migrations/)

---

## 📋 Обзор

**CTS-Core** — это Go-приложение для управления распределённой системой криптовалютной торговли:

- 🎯 **Оркестрация**: Управление 25+ независимыми traders
- 📊 **Задачи**: TRADE (арбитраж) + MONITOR (сбор рыночных данных)
- 🔒 **Безопасность**: mTLS, envelope encryption (HSM), certificate-based auth
- ⚡ **Производительность**: WebSocket для low-latency communication
- 📈 **Масштабирование**: Resource pool management, intelligent task assignment

---

## ✅ Архитектура готова - можно начинать Phase 1

**Готовность: 100%**  
**Статус: 🟢 Phase 0 - Database Migrations**

### Что готово

✅ **Все архитектурные решения приняты:**
1. ✅ Hybrid trader registration (admin pre-register + auto-connect)
2. ✅ State: daemon.state + MySQL sync
3. ✅ Idempotency: UNIQUE constraints + exchange_order_id tracking
4. ✅ SQL schema: 11 таблиц (автоматизация HSM re-encryption пока не реализована)
5. ✅ Failover: single instance + trader resilience (Phase 1)

📖 **См. [ARCHITECTURE.md](ARCHITECTURE.md)** для всех архитектурных решений

---

## 📚 Документация

### 🎯 Начните отсюда

| Документ | Описание | Когда читать |
|----------|----------|-------------|
| **[DOCS_INDEX.md](DOCS_INDEX.md)** | 📑 Навигация по всем документам | Первым делом |
| **[DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)** | 📋 План разработки (Phase 0-4) | НАЧАТЬ ЗДЕСЬ |
| **[ARCHITECTURE.md](ARCHITECTURE.md)** | 🏗️ Архитектура + все решения | Для понимания системы |
| **[API_SPECIFICATION.md](API_SPECIFICATION.md)** | 🔌 WebSocket + REST API | При работе с API |
| **[HSM_KEY_ROTATION.md](HSM_KEY_ROTATION.md)** | 🔐 HSM key rotation guide | Для production |

### 📖 Дополнительно

- **[TRADER_MODES.md](TRADER_MODES.md)** - TRADE vs MONITOR режимы
- **[CONTEXT.md](CONTEXT.md)** - Контекст для AI/разработчиков
- **[gpt.txt](gpt.txt)** - Исходная спецификация (БД, стратегии)

---

## 🏗️ Архитектура

### High-Level Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      CTS-CORE                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Orchestrator: Session Mgr + Task Scheduler          │   │
│  │ + Latency Analyzer + Metrics Collector              │   │
│  └─────────────────────────────────────────────────────┘   │
│           ↕ WebSocket (mTLS)           ↕ REST API          │
└─────────────────────────────────────────────────────────────┘
          ↓                                      ↓
   ┌─────────────┐                        ┌─────────────┐
   │ Trader #1   │  ...  (25+ traders)    │   Web UI    │
   │ EU-Frankfurt│                        │   Admin     │
   └─────────────┘                        └─────────────┘
          ↓
   ┌─────────────────────────────────────┐
   │  Exchanges (Binance, KuCoin, ...)   │
   └─────────────────────────────────────┘
```

**Ключевые особенности:**
- Traders подключаются к CTS-Core через WebSocket (mTLS)
- CTS-Core управляет задачами (TRADE + MONITOR)
- Traders напрямую обращаются к HSM (для credentials) и ClickHouse (для данных)
- Web UI через REST API + WebSocket (realtime stats)

---

## 🔐 Безопасность

### Envelope Encryption

```
KEK (Key Encryption Key) в HSM
    ↓
DEK (Data Encryption Key) зашифрован KEK
    ↓
API Keys зашифрованы DEK
```

### mTLS Everywhere

```
CTS-Core ←→ Traders:   mTLS (OU=Trading)
CTS-Core ←→ Web:       JWT + optional mTLS
Traders  ←→ HSM:       mTLS (OU=Trading)
Web      ←→ HSM:       mTLS (OU=2FA)
```

---

## 🚀 Быстрый старт

### Prerequisites

```bash
# Требования:
- Go 1.24.9+
- MySQL 9+
- ClickHouse 23+
- SoftHSM v2 (через hsm-service)
- Сертификаты (mTLS)
```

### Установка (когда будет готово)

```bash
# 1. Clone repository
git clone <repo-url>
cd cts-core

# 2. Install dependencies
go mod download

# 3. Configure
cp conf/config.example.yaml conf/config.yaml
# Edit conf/config.yaml

# 4. Setup database
mysql -u root -p < migrations/001_initial_schema.sql

# 5. Generate certificates
cd pki && ./scripts/generate-certs.sh

# 6. Build
make build

# 7. Run
./bin/cts-core -config conf/config.yaml
```

**⚠️ ВНИМАНИЕ:** Это пример для будущего. Сейчас проект НЕ готов к запуску.

---

## 📊 Технологический стек

```yaml
Language: Go 1.24.9
Framework: Gin (REST API)
WebSocket: gorilla/websocket
Databases:
  Transactional: MySQL 9 (mTLS)
  Time-series: ClickHouse
Security:
  HSM: SoftHSM v2 (via hsm-service)
  TLS: TLS 1.3, mTLS everywhere
  Encryption: AES-256-GCM (envelope encryption)
  Auth: Certificate-based (OU in subject) + JWT
Monitoring:
  Metrics: Prometheus
  Logging: Structured JSON
```

---

## 🗺️ Roadmap

```
✅ Phase 0: Architecture & Migrations (завершено)
   - ARCHITECTURE.md (25 Phase 1 decisions + HSM key rotation)
   - API_SPECIFICATION.md (27 error codes, 3-level trade structure)
   - DEVELOPMENT_PLAN.md (Phase 0-4 roadmap)
   - migrations/001_phase1_schema.sql (11 tables)
   - HSM_KEY_ROTATION.md (complete guide)

🔵 Phase 0 Current: Apply migrations (~1 час)
   - mysql < migrations/001_phase1_schema.sql
   - Verify 11 tables created
   - Ready for Phase 1 development

⏳ Phase 1: Foundation (2 недели)
   - Project setup (go.mod, config, logger)
   - MySQL connection pool
   - HSM client (mTLS)
   - Basic REST API (/health, /metrics)

⏳ Phase 2: Core Features (1.5 недели)
   - WebSocket server (traders)
   - Session manager
   - Task scheduler (basic)
   - Heartbeat & health check

⏳ Phase 3: Business Logic (2 недели)
   - DB Proxy (encrypted credentials)
   - Latency analyzer
   - Task assignment algorithm
   - Metrics collector

⏳ Phase 4: Integration (2 недели)
   - WebSocket for web (admin)
   - Full REST API
   - Trade result processing
   - Integration testing

⏳ Phase 1.5 (optional): Failover (1 неделя)
   - Active-Passive CTS-Core setup
   - Redis for state sharing
```

---

## 🤝 Contributing

### Перед началом работы

1. **Примените миграции**: `mysql -u root -proot -h 127.0.0.1 ct_system < migrations/001_phase1_schema.sql`
2. **Изучите [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)** - начните с Phase 1
3. **Прочитайте [ARCHITECTURE.md](ARCHITECTURE.md)** - понимание системы
4. **Ознакомьтесь с [API_SPECIFICATION.md](API_SPECIFICATION.md)** - контракты

### Workflow

```bash
# 1. Create feature branch
git checkout -b feature/your-feature

# 2. Make changes
# ...

# 3. Run tests (когда появятся)
make test

# 4. Commit
git commit -m "feat: your feature description"

# 5. Push
git push origin feature/your-feature

# 6. Create Pull Request
```

---

## 📄 License

TBD

---

## 📞 Контакты

**Проект**: CTS-Core  
**Версия**: 0.0.1 (pre-alpha)  
**Статус**: Design Phase  

---

## 🔗 Связанные проекты

- **[daemon2](other-sub-system/daemon2/)** - Trader (базовая структура)
- **[hsm-service](other-sub-system/hsm-service/)** - SoftHSM service (mTLS, KEK/DEK)
- **[www-go](other-sub-system/www-go/)** - Web UI (Gin, cryptostore patterns)

---

**✅ Готово к Phase 1 - начинайте с [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)**
