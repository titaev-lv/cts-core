# Trader: Dual-Mode Operation

> **Версия**: 1.0.0 | **Дата**: 2026-01-27  
> **См. также**: [ARCHITECTURE.md](ARCHITECTURE.md), [API_SPECIFICATION.md](API_SPECIFICATION.md)

---

## Ключевая концепция

Trader работает в **двух НЕЗАВИСИМЫХ режимах одновременно**:

| Режим | Цель | База данных | Источник config |
|-------|------|-------------|-----------------|
| **TRADE** | Арбитражные сделки | MySQL → ARBITRAGE_TRANS | TRADE table |
| **MONITOR** | Сбор рыночных данных | ClickHouse → tick_* | MONITOR_PAIR table |

**Критично:** Режимы полностью независимы — разные БД, разные цели, могут работать на одной паре/бирже параллельно.

```
TRADER
├── TRADE:    BTC/USDT Binance↔KuCoin (арбитраж) → MySQL
└── MONITOR:  BTC/USDT на 5 биржах (котировки)   → ClickHouse
```

---

## TRADE Mode

### Назначение
Исполнение арбитражных сделок:
- Ищет профитные возможности между биржами
- Размещает ордера одновременно (buy/sell)
- Отправляет результаты в CTS-Core через WebSocket
- CTS-Core записывает в MySQL (ARBITRAGE_TRANS)

### Конфигурация
**Источник:** TRADE table в MySQL  
**Пример:** TRADE #123 (BTC/USDT Binance↔KuCoin)

### Поток выполнения
```
1. CTS-Core → task.assign (credentials_encrypted, config)
2. Trader → HSM: decrypt credentials
3. Trader → Exchanges: connect, subscribe orderbook
4. Trader finds opportunity → trade.intent (request CTS-Core)
5. CTS-Core → arbitrage_id + approval
6. Trader executes orders
7. Trader → trade.result (WebSocket event)
8. CTS-Core → MySQL: INSERT ARBITRAGE_TRANS
```

**Результат:** `ARBITRAGE_TRANS`, `ARBITRAGE_ORDER` в MySQL

---

## MONITOR Mode

### Назначение
Сбор рыночных данных для анализа:
- Ticks (каждая сделка)
- Orderbook snapshots
- OHLC candles (1m, 5m, 15m)
- Пишет **напрямую в ClickHouse**

### Конфигурация
**Источник:** MONITOR_PAIR table в MySQL  
**Пример:** BTC/USDT на 5 биржах (public data, credentials НЕ нужны)

### Поток выполнения
```
1. CTS-Core → task.assign (exchanges, data_types, config)
2. Trader → Exchanges: connect public WS streams
3. Trader collects data continuously
4. Trader → ClickHouse: batch insert every 5 min
5. Trader → CTS-Core: monitor.result (metrics only)
```

**Результат:** `tick_*`, `ohlc_*_1m`, `orderbook_snapshot_*` в ClickHouse

---

## Resource Pool Management

### Проблема
Биржи лимитируют:
- **Max 10 WS connections** per IP
- **Max 35 subscriptions** per WS connection

### Решение: Tracking в CTS-Core

```sql
-- Лимиты
CREATE TABLE EXCHANGE_LIMITS (
    exchange_id INT PRIMARY KEY,
    max_ws_connections INT DEFAULT 10,
    max_subscriptions_per_ws INT DEFAULT 35
);

-- Использование
CREATE TABLE TRADER_EXCHANGE_RESOURCE (
    trader_id VARCHAR(100),
    exchange_id INT,
    current_ws_count INT DEFAULT 0,
    total_subscriptions INT DEFAULT 0
);
```

**Scheduler проверяет перед assignment:**
```
IF (current_ws + needed_ws <= max_ws) 
AND (total_subs + needed_subs <= max_subs * max_ws)
THEN assign task
ELSE find another trader OR reject
```

### Переиспользование WS
```
Binance WS #1 на trader-1:
├── TRADE #1: 5 subscriptions
├── MONITOR #1: 25 subscriptions
= 30 total (< 35 limit) ✓
```

---

## Multitasking

Один trader может обслуживать **несколько TRADE + MONITOR одновременно**:

```
trader-1:
├── TRADE #1 (BTC Binance↔KuCoin)
├── TRADE #2 (ETH OKX↔Bybit)  
├── MONITOR #1 (BTC на 5 биржах)
└── MONITOR #2 (ETH на 3 биржах)

CONSTRAINT: WS resource limits per exchange
```

---

## Критические решения

1. **Результаты TRADE → MySQL через CTS-Core** (не напрямую от trader)
   - WebSocket: trade.result event
   - CTS-Core пишет в ARBITRAGE_TRANS
   - Idempotency через trade.intent flow

2. **Результаты MONITOR → ClickHouse напрямую** (не через CTS-Core)
   - Низкая latency записи
   - Не перегружает CTS-Core

3. **Failover отличается:**
   - TRADE: критично (открытые ордера нужно отменить)
   - MONITOR: некритично (потеря нескольких тиков acceptable)

---

**Подробности API:** См. [API_SPECIFICATION.md](API_SPECIFICATION.md)  
**Архитектура:** См. [ARCHITECTURE.md](ARCHITECTURE.md)  
**Статус:** ✅ Все решения приняты - см. [ARCHITECTURE.md](ARCHITECTURE.md) и [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)
