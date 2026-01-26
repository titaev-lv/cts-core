# CTS-Core: Crypto Trading System Core

> **Центральный оркестратор для распределённой системы арбитражной торговли криптовалютами**

[![Status](https://img.shields.io/badge/status-design-orange)](BEFORE_START.md)
[![Architecture](https://img.shields.io/badge/architecture-ready-green)](ARCHITECTURE.md)
[![API](https://img.shields.io/badge/API-specified-green)](API_SPECIFICATION.md)
[![Readiness](https://img.shields.io/badge/readiness-65%25-yellow)](BEFORE_START.md)

---

## 📋 Обзор

**CTS-Core** — это Go-приложение для управления распределённой системой криптовалютной торговли:

- 🎯 **Оркестрация**: Управление 25+ независимыми trader daemons
- 📊 **Задачи**: TRADE (арбитраж) + MONITOR (сбор рыночных данных)
- 🔒 **Безопасность**: mTLS, envelope encryption (HSM), certificate-based auth
- ⚡ **Производительность**: WebSocket для low-latency communication
- 📈 **Масштабирование**: Resource pool management, intelligent task assignment

---

## 🚨 ВАЖНО: Проект НЕ готов к разработке

**Готовность: 65%**  
**Статус: 🔴 Phase 0.5 - Architecture Hardening**

### Критические блокеры

Перед началом кодирования необходимо решить **12 критических вопросов**:

📖 **См. [BEFORE_START.md](BEFORE_START.md)** для полного списка

**Top-5 блокеров:**
1. ⚠️ Механизм регистрации трейдеров (automatic vs manual vs hybrid)
2. ⚠️ State persistence strategy (MySQL vs Redis vs local file)
3. ⚠️ Idempotency guarantees (trade.intent flow)
4. ⚠️ Недостающие SQL таблицы (TRADER, MONITOR_PAIR, EXCHANGE_LIMITS, etc.)
5. ⚠️ Failover design (Active-Passive minimum)

---

## 📚 Документация

### 🎯 Начните отсюда

| Документ | Описание | Когда читать |
|----------|----------|-------------|
| **[DOCS_INDEX.md](DOCS_INDEX.md)** | 📑 Навигация по всем документам | Первым делом |
| **[BEFORE_START.md](BEFORE_START.md)** | 🔴 Критические вопросы | ОБЯЗАТЕЛЬНО |
| **[ARCHITECTURE.md](ARCHITECTURE.md)** | 🏗️ Архитектура системы | После BEFORE_START |
| **[API_SPECIFICATION.md](API_SPECIFICATION.md)** | 🔌 WebSocket + REST API | При работе с API |
| **[DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)** | 📋 План разработки | Для планирования |

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
Database:
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
✅ Phase 0: Architecture Design (завершено)
   - ARCHITECTURE.md
   - API_SPECIFICATION.md
   - TRADER_MODES.md
   - DEVELOPMENT_PLAN.md

🔴 Phase 0.5: Architecture Hardening (ТЕКУЩАЯ, 1-2 недели)
   - Решить все вопросы из BEFORE_START.md
   - SQL migrations для недостающих таблиц
   - Финализировать state management strategy
   - Failover design (Active-Passive minimum)

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

1. **Прочитайте [BEFORE_START.md](BEFORE_START.md)** - критические вопросы
2. **Изучите [ARCHITECTURE.md](ARCHITECTURE.md)** - понимание системы
3. **Ознакомьтесь с [API_SPECIFICATION.md](API_SPECIFICATION.md)** - контракты

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

- **[daemon2](other-sub-system/daemon2/)** - Trader daemon (базовая структура)
- **[hsm-service](other-sub-system/hsm-service/)** - SoftHSM service (mTLS, KEK/DEK)
- **[www-go](other-sub-system/www-go/)** - Web UI (Gin, cryptostore patterns)

---

**⚠️ НЕ начинайте Phase 1 пока не закрыты все блокеры в [BEFORE_START.md](BEFORE_START.md)**
