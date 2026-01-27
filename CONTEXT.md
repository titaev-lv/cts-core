# CTS-Core Development Context

> **Цель этого документа**: Передать полный контекст для продолжения разработки без потери информации.  
> **Дата создания**: 2026-01-26  
> **Последнее обновление**: 2026-01-26

---

## 1. Обзор проекта

**CTS-Core** — центральный оркестратор для распределённой системы арбитражной торговли криптовалютами.

### 1.1 Ключевые документы

| Документ | Описание | Статус |
|----------|----------|--------|
| ~~[BEFORE_START.md]~~ (УДАЛЕНО) | ✅ Все решения перенесены в ARCHITECTURE.md | ✅ Завершено |
| [API_SPECIFICATION.md](API_SPECIFICATION.md) | Единый API (WebSocket + REST) | ✅ Готов |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Полная архитектура системы | ✅ Готов |
| [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) | План разработки с Gantt-диаграммами | ✅ Готов |
| [TRADER_MODES.md](TRADER_MODES.md) | TRADE vs MONITOR режимы | ✅ Готов |
| [gpt.txt](gpt.txt) | Исходная спецификация (база данных, стратегии) | 📖 Reference |
| [shema-go.txt](shema-go.txt) | Схема Go-проекта | 📖 Reference |

### 1.2 Связанные компоненты (уже существуют)

| Путь | Описание | Что использовать |
|------|----------|------------------|
| `/other-sub-system/hsm-service/` | SoftHSM service для KEK | Паттерны mTLS, config, ACL |
| `/other-sub-system/www-go/` | Web-интерфейс на Gin | Envelope encryption, cryptostore |
| `/other-sub-system/daemon2/` | Базовая структура трейдера | Exchange drivers, WS manager |

---

## 2. Принятые архитектурные решения

> **Важно!** Эти решения зафиксированы и не подлежат пересмотру.

| # | Решение | Детали |
|---|---------|--------|
| 1 | **Трейдеры → HSM напрямую** | CTS-Core передаёт encrypted DEK, трейдер сам расшифровывает через HSM (OU=Trading) |
| 2 | **Трейдеры → ClickHouse напрямую** | Tick data пишется напрямую, не через CTS-Core |
| 3 | **Web → HSM напрямую для 2FA** | www-go обращается к HSM для шифрования/расшифровки 2FA secrets (OU=2FA) |
| 4 | **CTS-Core НЕ имеет доступа к HSM** | Только передаёт зашифрованные данные |
| 5 | **Стратегии**: Cross-exchange → Triangular → Limit+Market | Приоритет реализации |
| 6 | **Futures/DEX** | Только заглушки в архитектуре |
| 7 | **Инфраструктура** | Docker для dev, VM Debian для prod |
| 8 | **Сертификаты** | Ручная генерация через CA |

---

## 3. Технологический стек

```yaml
language: Go 1.24.9
framework: Gin (для REST API)
websocket: gorilla/websocket
database:
  primary: MySQL 9 (mTLS)
  timeseries: ClickHouse
security:
  hsm: SoftHSM v2 (via hsm-service)
  tls: TLS 1.3, mTLS everywhere
  encryption: AES-256-GCM (envelope encryption)
  auth: Certificate-based (OU in subject)
```

---

## 4. База данных

### 4.1 Существующие таблицы (из gpt.txt)

- `USER` — пользователи
- `EXCHANGE` — биржи
- `EXCHANGE_ACCOUNTS` — аккаунты бирж (encrypted API keys)
- `TRADE` — торговые конфигурации
- `TRADE_SPOT_ARRAYS` — пары для торговли
- `SPOT_TRADE_PAIR` — торговые пары

### 4.2 Новые таблицы (определены в ARCHITECTURE.md, секция 10)

- `TRADER_SESSION` — сессии трейдеров
- `TASK_ASSIGNMENT` — назначения задач
- `TRADER_LATENCY` — метрики latency
- `ARBITRAGE_TRANS` — арбитражные транзакции
- `ARBITRAGE_ORDER` — ордера внутри транзакции

---

## 5. Структура проекта CTS-Core

```
cts-core/
├── cmd/cts-core/main.go           # Entry point
├── internal/
│   ├── config/                    # Конфигурация (YAML)
│   ├── logger/                    # Логирование (как в daemon2)
│   ├── db/                        # MySQL pool + models
│   ├── hsm/                       # HSM client (mTLS) — НЕ используется CTS-Core!
│   ├── api/
│   │   ├── server.go              # HTTP/WS server
│   │   ├── rest/                  # REST handlers
│   │   └── ws/                    # WebSocket handlers
│   ├── session/                   # Session manager
│   ├── scheduler/                 # Task scheduler
│   └── metrics/                   # Prometheus
├── conf/config.yaml
├── pki/                           # Certificates
├── Dockerfile
└── docker-compose.yml
```

---

## 6. WebSocket Protocol

> **См. полную спецификацию**: [API_SPECIFICATION.md](API_SPECIFICATION.md)

### 6.1 Endpoint

```
wss://cts-core:8443/ws/trader   # Для трейдеров
wss://cts-core:8443/ws/admin    # Для web-интерфейса
```

### 6.2 Message Format

```json
{
    "id": "uuid-v4",
    "type": "request|response|event",
    "action": "string",
    "payload": { },
    "timestamp": 1737823200000,
    "correlation_id": "uuid"
}
```

### 6.3 Ключевые actions

| Direction | Action | Описание |
|-----------|--------|----------|
| T→C | `trader.register` | Регистрация трейдера |
| T→C | `trader.heartbeat` | Heartbeat каждые 5 сек |
| T→C | `trade.result` | Результат арбитража |
| C→T | `task.assign` | Назначение задачи |
| C→T | `task.cancel` | Отмена задачи |
| C→T | `trader.shutdown` | Graceful shutdown |

---

## 7. Безопасность

### 7.1 OU-based ACL в HSM

| OU | Доступ | Кто использует |
|----|--------|----------------|
| `Trading` | KEK: exchange-key | Трейдеры |
| `2FA` | KEK: 2fa | www-go |

### 7.2 Credential Flow

```
1. CTS-Core загружает из MySQL: encrypted_dek + encrypted_api_key
2. CTS-Core отправляет трейдеру: task.assign { encrypted_dek, encrypted_api_key }
3. Трейдер → HSM: POST /decrypt (encrypted_dek)
4. HSM → Трейдер: plain DEK
5. Трейдер расшифровывает API key локально
6. API key существует только в памяти трейдера
```

---

## 8. Текущий статус

### 8.1 Готово

- [x] Архитектурный документ (ARCHITECTURE.md)
- [x] План разработки (DEVELOPMENT_PLAN.md)
- [x] Mermaid-диаграммы для всех секций
- [x] SQL-схема для новых таблиц
- [x] WebSocket protocol specification

### 8.2 Готовность к разработке

✅ **СТАТУС: 100% готовности архитектуры**

**ВСЕ БЛОКЕРЫ РЕШЕНЫ:**
- [x] Trader registration: Hybrid (admin pre-register + auto-connect)
- [x] State persistence: daemon.state + MySQL sync
- [x] SQL таблицы: 11 tables в migrations/001_phase1_schema.sql
- [x] Idempotency: UNIQUE constraints + exchange_order_id tracking
- [x] Failover: Single instance + trader resilience (Phase 1)
- [x] Rate limiting: 1000 req/min REST, 10000 msg/min WS
- [x] Error codes: 27 стандартизированных кодов
- [x] HSM key rotation: Complete infrastructure with re-encryption

### 8.3 Следующие шаги

**Phase 0: Database Migrations (ТЕКУЩАЯ ФАЗА)**
1. **Применить миграции** - `mysql < migrations/001_phase1_schema.sql`
2. **Проверить таблицы** - `mysql -e "SHOW TABLES;"`
3. **Готовность к Phase 1**

**Phase 1: Foundation (после миграций)**
   - [ ] Config loader (YAML)
   - [ ] Logger (zerolog hybrid: text DEV, JSON PROD)
   - [ ] MySQL connection pool (mTLS)
   - [ ] HSM client (mTLS)
   - [ ] Basic REST API (/health, /metrics)

**Phase 2: Core Features**
   - [ ] WebSocket server для трейдеров
   - [ ] Session manager
   - [ ] Task scheduler

---

## 9. Ссылки на код для изучения

### 9.1 Паттерны из hsm-service

```go
// Config loading with hot reload
// See: /other-sub-system/hsm-service/internal/config/config.go

// mTLS server setup
// See: /other-sub-system/hsm-service/internal/server/server.go

// OU-based ACL
// See: /other-sub-system/hsm-service/internal/server/middleware.go
```

### 9.2 Паттерны из www-go

```go
// Envelope encryption
// See: /other-sub-system/www-go/internal/cryptostore/exchange_account_crypto.go

// Gin handlers
// See: /other-sub-system/www-go/internal/controllers/
```

### 9.3 Паттерны из daemon2

```go
// Logger
// See: /other-sub-system/daemon2/internal/logger/

// Exchange drivers
// See: /other-sub-system/daemon2/internal/exchange/

// WebSocket manager
// See: /other-sub-system/daemon2/internal/ws/
```

---

## 10. Команды для разработки

```bash
# Инициализация проекта
cd /home/dev/docker/cts-core
go mod init cts-core

# Запуск тестов
go test ./...

# Сборка
go build -o bin/cts-core ./cmd/cts-core

# Запуск
./bin/cts-core -config conf/config.yaml
```

---

## 11. Важные заметки

1. **CTS-Core НЕ расшифровывает credentials** — только передаёт зашифрованные данные
2. **Heartbeat timeout = 10 сек** — после этого трейдер считается отключённым
3. **Failover time < 5 сек** — благодаря hot standby monitoring
4. **Graceful shutdown = 30 сек** — время на завершение всех операций
5. **Глубина стакана и TTL** — вынесены в конфиг, определяются позже

---

*Этот документ создан для передачи контекста между AI-ассистентами.*
