# CTS-Core Complete API Specification

> **Версия документа**: 1.1.0  
> **Обновлено**: 2026-03-10  
> **Статус**: Целевая спецификация (реализация частичная)  
> **Связь с**: ARCHITECTURE.md, TRADER_MODES.md, DEVELOPMENT_PLAN.md

---

## Оглавление

0. [Статус реализации](#0-статус-реализации)
1. [Обзор API](#1-обзор-api)
2. [WebSocket Protocol](#2-websocket-protocol)
3. [REST API](#3-rest-api)
4. [Authentication & Authorization](#4-authentication--authorization)
5. [Error Handling](#5-error-handling)
6. [Rate Limiting](#6-rate-limiting)
7. [Versioning](#7-versioning)

---

## 0. Статус реализации

Срез по коду на 2026-03-10:

- Реализовано: REST `/health`, `/ready`, `/live`; базовый WS handler (stub).
- Не завершено: `/metrics` endpoint и Prometheus wiring.
- Не завершено: полный runtime WS protocol (`trader.register`, `trader.heartbeat`, lifecycle/session orchestration).

Ниже описан целевой API-контракт. Для текущего runtime-поведения приоритет у кода.

---

## 1. Обзор API

CTS-Core предоставляет два типа API:

| Протокол | Назначение | Клиенты |
|----------|-----------|---------|
| **WebSocket** | Real-time bidirectional communication | Traders, Web UI (realtime) |
| **REST API** | CRUD operations, admin management | Web UI (admin), External systems |

### 1.1 Endpoints

```
WebSocket:
  wss://cts-core:8443/ws/trader    - Для трейдеров
  wss://cts-core:8443/ws/admin     - Для web admin UI

REST API:
  https://cts-core:8443/api/v1/*   - REST endpoints
  https://cts-core:8443/health     - Health check (public)
  https://cts-core:8443/metrics    - Prometheus metrics (public, planned)
```

### 1.2 Разделение ответственности

```yaml
WebSocket (real-time):
  - target: Trader registration & heartbeat
  - target: Task assignment & cancellation
  - target: Trade execution & results
  - target: Monitor data streaming
  - target: Metrics reporting
  - target: Real-time notifications

REST API (stateless):
  - target: CRUD для traders, trades, monitors
  - target: Query historical data
  - target: Admin configuration
  - target: Bulk operations
  - target: Reporting & analytics
```

---

## 2. WebSocket Protocol

### 2.1 Connection Flow

```
1. Client → Server: TLS handshake (mTLS for traders)
2. Client → Server: WebSocket upgrade request
3. Server → Client: 101 Switching Protocols
4. Client → Server: trader.register (request)
5. Server → Client: trader.register_ack (response)
6. ⟳ Active communication (heartbeat, tasks, results)
7. Server → Client: trader.shutdown (request) [graceful]
8. Client → Server: trader.shutdown_ack (response)
9. Connection closed
```

### 2.2 Base Message Format

**Все сообщения в JSON:**

```json
{
  "id": "uuid-v4",
  "type": "request|response|event",
  "action": "string",
  "payload": {},
  "timestamp": 1737823200000,
  "correlation_id": "uuid",
  "version": "1.0"
}
```

| Поле | Тип | Обязательное | Описание |
|------|-----|-------------|----------|
| `id` | string (UUID) | ✅ | Уникальный ID сообщения |
| `type` | enum | ✅ | request, response, event |
| `action` | string | ✅ | Действие (см. 2.3) |
| `payload` | object | ✅ | Данные (может быть {}) |
| `timestamp` | number | ✅ | Unix milliseconds |
| `correlation_id` | string (UUID) | ❌ | Для response/event в ответ на request |
| `version` | string | ❌ | Default: "1.0" |

### 2.3 Message Types

```
request:
  - Отправитель ожидает response
  - Timeout: 30 сек
  - Требует correlation_id в ответе

response:
  - Ответ на request
  - Содержит correlation_id из request
  - Payload включает результат или error

event:
  - Односторонняя коммуникация
  - Отправитель НЕ ожидает ответ
  - Может содержать correlation_id (для контекста)
```

---

### 2.4 Trader → CTS-Core Messages

#### 2.4.1 trader.register (request)

Регистрация трейдера при подключении.

**Request:**
```json
{
  "type": "request",
  "action": "trader.register",
  "request_id": "req-1",
  "payload": {
    "trader_id": "trader-eu-1",
    "version": "1.0.0",
    "region": "eu-frankfurt",
    "capabilities": ["binance", "kucoin", "bybit", "okx"],
    "resources": {
      "cpu_cores": 4,
      "memory_gb": 16,
      "network_bandwidth_mbps": 1000
    },
    "current_load": {
      "cpu_usage_percent": 0,
      "memory_usage_percent": 0,
      "active_tasks": 0
    }
  }
}
```

**Response (success):**
```json
{
  "type": "response",
  "action": "trader.register_ack",
  "request_id": "req-1",
  "ts": 1737823200000,
  "payload": {
    "status": "ok",
    "trader_id": "trader-eu-1",
    "session_id": "session-uuid",
    "session_timeout_sec": 30,
    "server_time": 1737823200000
  }
}
```

**Response (error):**
```json
{
  "type": "response",
  "action": "error",
  "request_id": "req-1",
  "ts": 1737823200001,
  "payload": {
    "code": "INVALID_PAYLOAD",
    "message": "trader_id is required",
    "details": {
      "field": "trader_id"
    }
  }
}
```

`request_id` behavior:
- If trader sends `request_id`, CTS-Core mirrors it in response.
- If trader omits `request_id`, CTS-Core generates server id (`srv-<msg_id>`).

**Errors:**
- `INVALID_MESSAGE` - malformed JSON, non-text frame, or unsupported message type
- `INVALID_PAYLOAD` - payload validation error (missing `trader_id`, `version`, or bad JSON)
- `UNKNOWN_ACTION` - unsupported WS action
- `DUPLICATE_CONNECTION` - duplicate `trader.register` in same WS session

---

#### 2.4.2 trader.heartbeat (event)

Периодический heartbeat (каждые 5 сек).

**Event:**
```json
{
  "type": "event",
  "action": "trader.heartbeat",
  "payload": {
    "trader_id": "trader-eu-1",
    "session_id": "session-uuid",
    "status": "active",
    "task_stats": {
      "active_tasks": 3,
      "active_trades": 1,
      "active_monitors": 2,
      "completed_today": 15,
      "failed_today": 1
    },
    "exchange_stats": {
      "binance": {
        "ws_connections": 2,
        "subscriptions": 25,
        "latency_ms": 45,
        "status": "connected"
      }
    },
    "system_metrics": {
      "cpu_usage_percent": 45.2,
      "memory_usage_percent": 62.5,
      "network_tx_mbps": 5.3,
      "network_rx_mbps": 12.7,
      "orders_per_second": 2.5
    }
  }
}
```

**Status values:**
- `idle` - Нет активных задач
- `active` - Есть активные задачи, нормальная загрузка
- `busy` - Высокая загрузка (> 80% CPU или > 90% tasks)

**Note:** Нет response. Если CTS-Core не получает heartbeat > 15 сек → disconnect.

---

#### 2.4.3 trade.intent (request) 🆕

Trader нашел арбитражную возможность, запрашивает разрешение на исполнение.

**Request:**
```json
{
  "type": "request",
  "action": "trade.intent",
  "payload": {
    "task_id": "task-12345",
    "trade_id": 123,
    "opportunity": {
      "exchange_buy": "binance",
      "exchange_buy_account_id": 10,
      "price_buy": "50000.00",
      "exchange_sell": "kucoin",
      "exchange_sell_account_id": 11,
      "price_sell": "50150.00",
      "quantity": "0.5",
      "estimated_profit_usdt": "75.00",
      "estimated_profit_percent": "0.15"
    }
  }
}
```

**Response (approved):**
```json
{
  "type": "response",
  "action": "trade.intent_ack",
  "correlation_id": "...",
  "payload": {
    "approved": true,
    "arbitrage_id": 12345,
    "max_execution_time_sec": 10,
    "server_time": 1737823200000
  }
}
```

**Response (rejected):**
```json
{
  "type": "response",
  "action": "trade.intent_ack",
  "correlation_id": "...",
  "payload": {
    "approved": false,
    "reason": "RISK_LIMIT_EXCEEDED",
    "message": "User daily loss limit exceeded",
    "details": {
      "daily_loss_limit_usdt": "1000.00",
      "current_loss_usdt": "1050.00"
    }
  }
}
```

**Rejection reasons:**
- `RISK_LIMIT_EXCEEDED` - Превышен лимит потерь
- `INSUFFICIENT_BALANCE` - Недостаточно баланса
- `MARKET_MOVED` - Цены изменились (stale opportunity)
- `EXCHANGE_UNAVAILABLE` - Биржа недоступна (circuit breaker)

---

#### 2.4.4 trade.result (event)

Результат исполнения арбитражной сделки с трехуровневой структурой данных.

**Трехуровневая структура:**
```
ARBITRAGE_TRANS (top level)
  ↓ has many
ARBITRAGE_ORDER (middle level - orders per exchange)
  ↓ has many  
ORDER_TRANSACTION (bottom level - individual fills/partials)
```

**Event:**
```json
{
  "type": "event",
  "action": "trade.result",
  "payload": {
    "task_id": "task-12345",
    "trader_id": "trader-eu-1",
    "execution_time_ms": 850,
    
    "arbitrage_trans": {
      "id": 12345,
      "trade_id": 123,
      "user_id": 1,
      "strategy": "cross_exchange",
      "pair": "BTC/USDT",
      "status": "completed",
      "gross_profit_usdt": "69.88",
      "net_profit_usdt": "57.38",
      "profit_percent": "0.11",
      "total_fees_usdt": "12.50",
      "started_at": "2026-01-28T15:04:05Z",
      "completed_at": "2026-01-28T15:04:05.850Z",
      "orders": [
        {
          "arbitrage_order_id": 24001,
          "exchange": "binance",
          "exchange_account_id": 10,
          "exchange_order_id": "abc123456",
          "side": "buy",
          "order_type": "market",
          "pair": "BTC/USDT",
          "requested_quantity": "0.5",
          "filled_quantity": "0.5",
          "avg_price": "50005.50",
          "total_cost": "25002.75",
          "total_fee": "0.00025",
          "fee_currency": "BTC",
          "status": "filled",
          "created_at": "2026-01-28T15:04:05.100Z",
          "filled_at": "2026-01-28T15:04:05.450Z",
          "transactions": [
            {
              "order_transaction_id": 48001,
              "exchange_transaction_id": "binance-tx-123456",
              "quantity": "0.3",
              "price": "50005.00",
              "cost": "15001.50",
              "fee": "0.00015",
              "fee_currency": "BTC",
              "timestamp": "2026-01-28T15:04:05.250Z"
            },
            {
              "order_transaction_id": 48002,
              "exchange_transaction_id": "binance-tx-123457",
              "quantity": "0.2",
              "price": "50006.25",
              "cost": "10001.25",
              "fee": "0.0001",
              "fee_currency": "BTC",
              "timestamp": "2026-01-28T15:04:05.450Z"
            }
          ]
        },
        {
          "arbitrage_order_id": 24002,
          "exchange": "kucoin",
          "exchange_account_id": 11,
          "exchange_order_id": "xyz789012",
          "side": "sell",
          "order_type": "market",
          "pair": "BTC/USDT",
          "requested_quantity": "0.5",
          "filled_quantity": "0.5",
          "avg_price": "50145.25",
          "total_cost": "25072.63",
          "total_fee": "12.50",
          "fee_currency": "USDT",
          "status": "filled",
          "created_at": "2026-01-28T15:04:05.200Z",
          "filled_at": "2026-01-28T15:04:05.700Z",
          "transactions": [
            {
              "order_transaction_id": 48003,
              "exchange_transaction_id": "kucoin-tx-789012",
              "quantity": "0.5",
              "price": "50145.25",
              "cost": "25072.63",
              "fee": "12.50",
              "fee_currency": "USDT",
              "timestamp": "2026-01-28T15:04:05.700Z"
            }
          ]
        }
      ]
    }
  }
}
```

**Status values:**
- `completed` - Все ордера исполнены успешно
- `partial` - Один или несколько ордеров исполнены частично
- `failed` - Ошибка при исполнении
- `cancelled` - Отменено (по команде или timeout)

**Сохранение в БД:**

CTS-Core при получении trade.result:
1. INSERT/UPDATE `ARBITRAGE_TRANS` (если еще не создан - хотя обычно создается при task.assign)
2. INSERT `ARBITRAGE_ORDER` для каждого ордера (если не partial update)
3. INSERT `ORDER_TRANSACTION` для каждой транзакции (fill/partial)

**Deduplication:** 
- UNIQUE constraint на `ARBITRAGE_ORDER(arbitrage_trans_id, exchange_order_id)`
- UNIQUE constraint на `ORDER_TRANSACTION(arbitrage_order_id, exchange_transaction_id)`

**Note:** Нет response, но CTS-Core acknowledgement через heartbeat.

---

#### 2.4.5 monitor.result (event)

Метрики от MONITOR задач.

**Event:**
```json
{
  "type": "event",
  "action": "monitor.result",
  "payload": {
    "task_id": "task-12346",
    "monitor_pair_id": 5,
    "pair": "BTC/USDT",
    "exchanges_count": 5,
    "data_points_collected": 1250,
    "time_range": {
      "start": 1737823000000,
      "end": 1737823300000
    },
    "statistics": {
      "ticks_collected": 850,
      "orderbook_snapshots": 300,
      "ohlc_1m_candles": 5,
      "avg_spread_percent": "0.02",
      "max_price_deviation_percent": "0.15"
    },
    "exchanges": {
      "binance": {
        "ticks": 250,
        "avg_latency_ms": 45,
        "errors": 0
      },
      "kucoin": {
        "ticks": 180,
        "avg_latency_ms": 120,
        "errors": 2
      }
    }
  }
}
```

**Note:** Отправляется batch каждые 5 минут. Данные также пишутся напрямую в ClickHouse.

---

#### 2.4.6 metrics.report (event)

Детальные метрики трейдера (для Prometheus).

**Event:**
```json
{
  "type": "event",
  "action": "metrics.report",
  "payload": {
    "trader_id": "trader-eu-1",
    "timestamp": 1737823200000,
    "system": {
      "cpu_usage_percent": 45.2,
      "memory_usage_bytes": 1073741824,
      "memory_total_bytes": 17179869184,
      "goroutines": 1250,
      "uptime_seconds": 86400
    },
    "tasks": {
      "active_total": 5,
      "active_trade": 2,
      "active_monitor": 3,
      "completed_total": 1523,
      "failed_total": 12
    },
    "exchanges": {
      "binance": {
        "ws_connections": 2,
        "subscriptions": 25,
        "orders_sent": 150,
        "orders_filled": 148,
        "orders_failed": 2,
        "avg_latency_ms": 45.5,
        "p99_latency_ms": 120.0
      }
    },
    "trading": {
      "arbitrage_completed_total": 45,
      "arbitrage_profit_usdt": "1523.50",
      "arbitrage_loss_usdt": "85.20",
      "net_profit_usdt": "1438.30"
    }
  }
}
```

**Frequency:** Каждые 30 сек.

---

### 2.5 CTS-Core → Trader Messages

#### 2.5.1 task.assign (request)

Назначение задачи трейдеру.

**Request (TRADE):**
```json
{
  "type": "request",
  "action": "task.assign",
  "payload": {
    "task_id": "task-12345",
    "task_type": "trade",
    "trade": {
      "trade_id": 123,
      "user_id": 1,
      "strategy": "cross_exchange",
      "pairs": [
        {
          "pair_id": 10,
          "pair": "BTC/USDT",
          "exchanges": [
            {
              "exchange_id": 1,
              "exchange": "binance",
              "exchange_account_id": 10,
              "credentials_encrypted": "base64-encrypted-data",
              "side": "buy"
            },
            {
              "exchange_id": 2,
              "exchange": "kucoin",
              "exchange_account_id": 11,
              "credentials_encrypted": "base64-encrypted-data",
              "side": "sell"
            }
          ]
        }
      ],
      "config": {
        "min_profit_percent": "0.1",
        "max_amount_usdt": "10000.00",
        "max_slippage_percent": "0.05",
        "order_type": "market",
        "timeout_sec": 10
      }
    }
  }
}
```

**Request (MONITOR):**
```json
{
  "type": "request",
  "action": "task.assign",
  "payload": {
    "task_id": "task-12346",
    "task_type": "monitor",
    "monitor": {
      "monitor_pair_id": 5,
      "pair_id": 10,
      "pair": "BTC/USDT",
      "exchanges": [1, 2, 3, 4, 5],
      "data_types": ["ticks", "orderbook", "ohlc_1m", "ohlc_5m"],
      "config": {
        "orderbook_depth": 20,
        "ohlc_intervals": ["1m", "5m", "15m"],
        "batch_interval_sec": 300
      }
    }
  }
}
```

**Response (success):**
```json
{
  "type": "response",
  "action": "task.ack",
  "correlation_id": "...",
  "payload": {
    "status": "ok",
    "task_id": "task-12345",
    "task_type": "trade",
    "accepted_at": 1737823200000,
    "estimated_start_time_ms": 50
  }
}
```

**Response (error):**
```json
{
  "type": "response",
  "action": "error",
  "correlation_id": "...",
  "payload": {
    "code": "INSUFFICIENT_RESOURCES",
    "message": "Cannot accept task: WS connection limit reached",
    "details": {
      "current_ws_count": 10,
      "max_ws_count": 10,
      "exchange": "binance"
    }
  }
}
```

---

#### 2.5.2 task.cancel (request)

Отмена задачи.

**Request:**
```json
{
  "type": "request",
  "action": "task.cancel",
  "payload": {
    "task_id": "task-12345",
    "reason": "user_request",
    "immediate": false
  }
}
```

**Reason values:**
- `user_request` - Пользователь отменил
- `failover` - Переназначение другому трейдеру
- `rebalance` - Load rebalancing
- `system_shutdown` - Shutdown системы

**Response:**
```json
{
  "type": "response",
  "action": "task.cancel_ack",
  "correlation_id": "...",
  "payload": {
    "status": "ok",
    "task_id": "task-12345",
    "cancelled_at": 1737823200000,
    "cancelled_orders": 0,
    "pending_actions": "none"
  }
}
```

---

#### 2.5.3 task.config_update (event)

Обновление конфигурации активной задачи.

**Event:**
```json
{
  "type": "event",
  "action": "task.config_update",
  "payload": {
    "task_id": "task-12345",
    "config": {
      "min_profit_percent": "0.15",
      "max_amount_usdt": "5000.00"
    }
  }
}
```

**Note:** Trader применяет изменения на лету, без перезапуска задачи.

---

#### 2.5.4 trader.shutdown (request)

Graceful shutdown трейдера.

**Request:**
```json
{
  "type": "request",
  "action": "trader.shutdown",
  "payload": {
    "grace_period_sec": 30,
    "cancel_pending_orders": true,
    "reason": "maintenance"
  }
}
```

**Reason values:**
- `maintenance` - Плановое обслуживание
- `failover` - Переключение на другой трейдер
- `update` - Обновление версии
- `decommission` - Удаление из системы

**Response:**
```json
{
  "type": "response",
  "action": "trader.shutdown_ack",
  "correlation_id": "...",
  "payload": {
    "status": "ok",
    "cancelled_orders": 5,
    "completed_trades": 2,
    "shutdown_time_sec": 12,
    "final_metrics": {
      "uptime_seconds": 86400,
      "trades_completed": 1523,
      "profit_usdt": "1438.30"
    }
  }
}
```

---

#### 2.5.5 latency.test (request)

Тест latency к бирже.

**Request:**
```json
{
  "type": "request",
  "action": "latency.test",
  "payload": {
    "exchange_id": 1,
    "exchange": "binance",
    "test_type": "ping"
  }
}
```

**Response:**
```json
{
  "type": "response",
  "action": "latency.test_result",
  "correlation_id": "...",
  "payload": {
    "exchange": "binance",
    "ping_ms": 45,
    "order_latency_ms": 120,
    "ws_latency_ms": 15,
    "timestamp": 1737823200000
  }
}
```

---

### 2.6 Web UI ↔ CTS-Core Messages

#### 2.6.1 web.login (request)

Аутентификация web admin.

**Request:**
```json
{
  "type": "request",
  "action": "web.login",
  "payload": {
    "user_id": 101,
    "session_token": "jwt-token",
    "client_id": "web-session-uuid"
  }
}
```

**Response:**
```json
{
  "type": "response",
  "action": "web.login_ack",
  "correlation_id": "...",
  "payload": {
    "status": "ok",
    "ws_session_id": "ws-session-uuid",
    "permissions": ["view_traders", "manage_trades"]
  }
}
```

---

#### 2.6.2 web.traders_list (request)

Список трейдеров.

**Request:**
```json
{
  "type": "request",
  "action": "web.traders_list",
  "payload": {
    "filter": "active"
  }
}
```

**Filter values:** `all`, `active`, `idle`, `offline`, `suspended`

**Response:**
```json
{
  "type": "response",
  "action": "web.traders_list",
  "correlation_id": "...",
  "payload": {
    "traders": [
      {
        "trader_id": "trader-eu-1",
        "status": "active",
        "region": "eu-frankfurt",
        "active_tasks": 5,
        "cpu_usage": 45.2,
        "connected_at": 1737823000000,
        "last_heartbeat": 1737823200000
      }
    ],
    "total": 25,
    "active": 20,
    "idle": 3,
    "offline": 2
  }
}
```

---

#### 2.6.3 web.stats_realtime (subscribe)

Подписка на real-time статистику.

**Request:**
```json
{
  "type": "request",
  "action": "web.stats_realtime",
  "payload": {
    "subscribe_to": ["trades", "monitors", "system_metrics"],
    "interval_sec": 1
  }
}
```

**Event stream:**
```json
{
  "type": "event",
  "action": "web.stats_update",
  "payload": {
    "timestamp": 1737823200000,
    "trades": {
      "completed_last_minute": 5,
      "profit_last_hour_usdt": "523.50"
    },
    "monitors": {
      "active_pairs": 50,
      "data_points_per_second": 1250
    },
    "system": {
      "active_traders": 20,
      "cpu_usage_avg": 42.5
    }
  }
}
```

---

## 3. REST API

### 3.1 Base URL

```
https://cts-core:8443/api/v1
```

### 3.2 Authentication

**Все REST endpoints требуют authentication (кроме /health, /metrics):**

```http
Authorization: Bearer <JWT-token>
```

### 3.3 Response Format

**Success:**
```json
{
  "success": true,
  "data": { /* resource */ },
  "timestamp": 1737823200000
}
```

**Error:**
```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "Trader not found",
    "details": {}
  },
  "timestamp": 1737823200000
}
```

---

### 3.4 Traders Management

#### GET /api/v1/traders

Список всех трейдеров.

**Query params:**
- `status` (optional): `registered`, `active`, `suspended`, `decommissioned`
- `region` (optional): `eu-frankfurt`, `us-east`, etc.
- `limit` (optional): default 100
- `offset` (optional): default 0

**Response:**
```json
{
  "success": true,
  "data": {
    "traders": [
      {
        "id": 1,
        "trader_id": "trader-eu-1",
        "certificate_cn": "trader-eu-1",
        "region": "eu-frankfurt",
        "status": "active",
        "max_concurrent_tasks": 10,
        "capabilities": ["binance", "kucoin"],
        "date_create": "2026-01-20T10:00:00Z",
        "date_modify": "2026-01-27T15:00:00Z"
      }
    ],
    "total": 25,
    "limit": 100,
    "offset": 0
  }
}
```

---

#### GET /api/v1/traders/{trader_id}

Детали трейдера.

**Response:**
```json
{
  "success": true,
  "data": {
    "trader": {
      "id": 1,
      "trader_id": "trader-eu-1",
      "status": "active",
      "region": "eu-frankfurt",
      "current_session": {
        "session_id": "session-uuid",
        "connected_at": "2026-01-27T12:00:00Z",
        "last_heartbeat": "2026-01-27T15:04:55Z",
        "active_tasks": 5,
        "cpu_usage": 45.2,
        "memory_usage": 62.5
      },
      "statistics": {
        "uptime_seconds": 86400,
        "trades_completed_total": 1523,
        "trades_failed_total": 12,
        "profit_total_usdt": "1438.30"
      }
    }
  }
}
```

---

#### POST /api/v1/traders

Регистрация нового трейдера (pre-registration).

**Request:**
```json
{
  "trader_id": "trader-us-1",
  "certificate_cn": "trader-us-1",
  "region": "us-east",
  "max_concurrent_tasks": 15,
  "capabilities": ["binance", "coinbase", "kraken"]
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "trader": {
      "id": 26,
      "trader_id": "trader-us-1",
      "status": "registered",
      "date_create": "2026-01-27T15:05:00Z"
    }
  }
}
```

---

#### PUT /api/v1/traders/{trader_id}

Обновление конфигурации трейдера.

**Request:**
```json
{
  "max_concurrent_tasks": 20,
  "status": "suspended"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "trader": { /* updated trader */ }
  }
}
```

---

#### DELETE /api/v1/traders/{trader_id}

Decommission трейдера.

**Response:**
```json
{
  "success": true,
  "data": {
    "trader_id": "trader-us-1",
    "status": "decommissioned",
    "decommissioned_at": "2026-01-27T15:10:00Z"
  }
}
```

---

### 3.5 Trades Management

#### GET /api/v1/trades

Список торговых конфигураций.

**Query params:**
- `user_id` (optional)
- `status` (optional): `active`, `inactive`
- `type` (optional): trade type ID

**Response:**
```json
{
  "success": true,
  "data": {
    "trades": [
      {
        "id": 123,
        "user_id": 1,
        "type": 6,
        "type_name": "Arbitrage",
        "active": true,
        "description": "BTC/USDT arbitrage",
        "max_amount_trade": "10000.00",
        "assigned_trader_id": "trader-eu-1",
        "date_create": "2026-01-20T10:00:00Z"
      }
    ],
    "total": 8
  }
}
```

---

#### GET /api/v1/trades/{trade_id}

Детали торговой конфигурации.

**Response:**
```json
{
  "success": true,
  "data": {
    "trade": {
      "id": 123,
      "user_id": 1,
      "type": 6,
      "active": true,
      "pairs": [
        {
          "pair": "BTC/USDT",
          "exchanges": ["binance", "kucoin"]
        }
      ],
      "config": {
        "min_profit_percent": "0.1",
        "max_amount_usdt": "10000.00"
      },
      "statistics": {
        "arbitrage_completed": 45,
        "profit_total_usdt": "523.50"
      }
    }
  }
}
```

---

#### POST /api/v1/trades

Создание новой торговой конфигурации.

**Request:**
```json
{
  "user_id": 1,
  "type": 6,
  "description": "ETH/USDT arbitrage",
  "active": true,
  "max_amount_trade": "5000.00",
  "pairs": [
    {
      "pair_id": 15,
      "exchanges": [
        {
          "exchange_id": 1,
          "exchange_account_id": 10,
          "side": "buy"
        },
        {
          "exchange_id": 2,
          "exchange_account_id": 11,
          "side": "sell"
        }
      ]
    }
  ],
  "config": {
    "min_profit_percent": "0.12",
    "max_slippage_percent": "0.05"
  }
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "trade": {
      "id": 124,
      "user_id": 1,
      "status": "active",
      "date_create": "2026-01-27T15:15:00Z"
    }
  }
}
```

---

#### PUT /api/v1/trades/{trade_id}

Обновление конфигурации.

#### DELETE /api/v1/trades/{trade_id}

Деактивация торговой конфигурации.

---

### 3.6 Arbitrage Transactions

#### GET /api/v1/arbitrage

Список арбитражных транзакций.

**Query params:**
- `trade_id` (optional)
- `status` (optional)
- `date_from` (optional): ISO 8601
- `date_to` (optional): ISO 8601
- `limit`, `offset`

**Response:**
```json
{
  "success": true,
  "data": {
    "transactions": [
      {
        "id": 12345,
        "trade_id": 123,
        "status": "Complete",
        "amount": "25000.00",
        "calc_profit": "57.38",
        "date_create": "2026-01-27T14:50:00Z"
      }
    ],
    "total": 1523
  }
}
```

---

#### GET /api/v1/arbitrage/{arbitrage_id}

Детали арбитражной транзакции.

**Response:**
```json
{
  "success": true,
  "data": {
    "transaction": {
      "id": 12345,
      "trade_id": 123,
      "status": "Complete",
      "orders": [
        {
          "exchange": "binance",
          "side": "buy",
          "price": "50005.50",
          "quantity": "0.5",
          "fee": "0.00025 BTC"
        },
        {
          "exchange": "kucoin",
          "side": "sell",
          "price": "50145.25",
          "quantity": "0.5",
          "fee": "12.50 USDT"
        }
      ],
      "profit": {
        "gross": "69.88",
        "net": "57.38",
        "percent": "0.11"
      }
    }
  }
}
```

---

#### GET /api/v1/arbitrage/{arbitrage_id}/orders

Ордера транзакции.

---

### 3.7 Monitor Pairs Management

#### GET /api/v1/monitor-pairs

Список конфигураций мониторинга.

#### POST /api/v1/monitor-pairs

Создание новой конфигурации.

**Request:**
```json
{
  "pair_id": 10,
  "active": true,
  "data_types": ["ticks", "orderbook", "ohlc_1m"],
  "orderbook_depth": 20,
  "exchanges": [1, 2, 3, 4, 5],
  "priority": 5
}
```

#### PUT /api/v1/monitor-pairs/{id}

Обновление конфигурации.

#### DELETE /api/v1/monitor-pairs/{id}

Удаление конфигурации.

---

### 3.8 System Endpoints

#### GET /health

Health check (public, no auth).

**Response:**
```json
{
  "status": "healthy",
  "timestamp": 1737823200000,
  "version": "1.0.0",
  "uptime_seconds": 86400,
  "checks": {
    "database": "ok",
    "redis": "ok",
    "websocket": "ok"
  }
}
```

---

#### GET /metrics

Prometheus metrics (public, no auth).

**Response:** Prometheus text format

```
# HELP cts_core_active_traders Number of active traders
# TYPE cts_core_active_traders gauge
cts_core_active_traders 20

# HELP cts_arbitrage_profit_usdt Total arbitrage profit in USDT
# TYPE cts_arbitrage_profit_usdt counter
cts_arbitrage_profit_usdt 15234.50
```

---

#### GET /api/v1/version

Version info.

**Response:**
```json
{
  "success": true,
  "data": {
    "version": "1.0.0",
    "build_time": "2026-01-27T10:00:00Z",
    "git_commit": "abc123",
    "go_version": "1.24.9"
  }
}
```

---

## 4. Authentication & Authorization

### 4.1 WebSocket Authentication

**Traders:**
- **mTLS certificate** with `OU=Trading` in subject
- Certificate CN must match `trader_id` in database
- CTS-Core validates certificate during TLS handshake

**Web UI:**
- **JWT token** in `Authorization` header during WS upgrade
- Token validated against session store
- Permission check for admin actions

### 4.2 REST API Authentication

**JWT Bearer token:**
```http
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Token payload:**
```json
{
  "user_id": 101,
  "username": "admin",
  "roles": ["admin", "trader"],
  "exp": 1737826800,
  "iat": 1737823200
}
```

### 4.3 Authorization Levels

| Endpoint | Required Role |
|----------|---------------|
| GET /api/v1/traders | `admin` or `trader` (own data) |
| POST /api/v1/traders | `admin` |
| PUT /api/v1/traders/* | `admin` |
| DELETE /api/v1/traders/* | `admin` |
| GET /api/v1/trades | `trader` (own data) |
| POST /api/v1/trades | `trader` |
| GET /health, /metrics | public (no auth) |

---

## 5. Error Handling

### 5.1 WebSocket Error Response

```json
{
  "type": "response",
  "action": "error",
  "correlation_id": "...",
  "payload": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": {}
  }
}
```

### 5.2 Error Codes

| Code | HTTP | Описание |
|------|------|----------|
| `INVALID_MESSAGE` | 400 | Некорректный формат сообщения |
| `VALIDATION_ERROR` | 400 | Ошибка валидации данных |
| `UNAUTHORIZED` | 401 | Нет или невалидный token |
| `INVALID_CERTIFICATE` | 401 | Невалидный mTLS certificate |
| `FORBIDDEN` | 403 | Недостаточно прав |
| `TRADER_SUSPENDED` | 403 | Trader suspended |
| `NOT_FOUND` | 404 | Ресурс не найден |
| `CONFLICT` | 409 | Конфликт (duplicate) |
| `DUPLICATE_CONNECTION` | 409 | Trader уже подключен |
| `ARBITRAGE_EXPIRED` | 410 | Арбитраж устарел (цены изменились) |
| `INSUFFICIENT_RESOURCES` | 429 | Недостаточно ресурсов |
| `RATE_LIMIT_EXCEEDED` | 429 | Превышен rate limit |
| `RISK_LIMIT_EXCEEDED` | 429 | Превышен risk limit |
| `EXCHANGE_LIMIT_EXCEEDED` | 429 | Превышен лимит биржи (orders/volume) |
| `TRADER_NOT_REGISTERED` | 400 | Trader не pre-registered в системе |
| `TRADER_OFFLINE` | 503 | Трейдер отключен/недоступен |
| `TASK_ASSIGNMENT_FAILED` | 500 | Не удалось назначить задачу |
| `INSUFFICIENT_BALANCE` | 400 | Недостаточно баланса на бирже |
| `ORDER_REJECTED` | 400 | Биржа отклонила ордер |
| `INTERNAL_ERROR` | 500 | Внутренняя ошибка сервера |
| `DATABASE_ERROR` | 500 | Ошибка базы данных |
| `CONFIGURATION_ERROR` | 500 | Ошибка конфигурации |
| `NETWORK_ERROR` | 502 | Сетевая ошибка (connection failed) |
| `EXCHANGE_ERROR` | 502 | Ошибка от биржи |
| `SERVICE_UNAVAILABLE` | 503 | Сервис недоступен |
| `TIMEOUT` | 504 | Timeout операции |
| `MARKET_MOVED` | 410 | Цены изменились (stale data) |

**Группировка по категориям:**

```yaml
CLIENT_ERRORS (4xx):
  - INVALID_MESSAGE, VALIDATION_ERROR
  - UNAUTHORIZED, INVALID_CERTIFICATE
  - FORBIDDEN, TRADER_SUSPENDED
  - NOT_FOUND
  - CONFLICT, DUPLICATE_CONNECTION
  - ARBITRAGE_EXPIRED, MARKET_MOVED
  - INSUFFICIENT_RESOURCES, RATE_LIMIT_EXCEEDED, RISK_LIMIT_EXCEEDED, EXCHANGE_LIMIT_EXCEEDED
  - TRADER_NOT_REGISTERED, INSUFFICIENT_BALANCE, ORDER_REJECTED

SERVER_ERRORS (5xx):
  - INTERNAL_ERROR, DATABASE_ERROR, CONFIGURATION_ERROR
  - TASK_ASSIGNMENT_FAILED
  - NETWORK_ERROR, EXCHANGE_ERROR
  - SERVICE_UNAVAILABLE, TRADER_OFFLINE
  - TIMEOUT
```

---

## 6. Rate Limiting

### 6.1 REST API Limits

```yaml
Default: 100 requests/min per IP
Admin endpoints: 50 requests/min per IP
Public endpoints (/health, /metrics): unlimited

Headers:
  X-RateLimit-Limit: 100
  X-RateLimit-Remaining: 95
  X-RateLimit-Reset: 1737823260
```

**Response (rate limit exceeded):**
```json
{
  "success": false,
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Too many requests",
    "retry_after": 60
  }
}
```

### 6.2 WebSocket Limits

```yaml
Max connections per IP: 5
Max traders per IP: unlimited (pre-authorized via mTLS)
Message rate limit: 1000 messages/min per connection
```

---

## 7. Versioning

### 7.1 REST API Versioning

```
/api/v1/*  - Current version
/api/v2/*  - Future version (backward compatible)
```

**Version negotiation:**
```http
Accept: application/vnd.cts.v1+json
```

### 7.2 WebSocket Protocol Versioning

```json
{
  "version": "1.0"  // in every message
}
```

**Supported versions:**
- `1.0` - Current (2026-01-27)

**Version compatibility:**
- Server поддерживает last 2 major versions
- Client должен указывать version в каждом сообщении
- Server отвечает в той же version

---

## 8. Changelog

### v1.0.0 (2026-01-27)

**Initial specification:**
- WebSocket protocol для traders
- WebSocket protocol для web UI
- REST API для CRUD операций
- mTLS authentication для traders
- JWT authentication для web/REST
- Error handling
- Rate limiting
- Versioning

**Added:**
- `trade.intent` message (idempotency)
- `latency.test` message
- Complete REST endpoints for traders, trades, arbitrage, monitors

---

*Этот документ - единый источник истины для всех API в CTS-Core.*
