# BEFORE START: Критические вопросы и архитектурные пробелы

> **Версия документа**: 1.0.0  
> **Дата**: 2026-01-27  
> **Статус**: 🔴 ТРЕБУЕТСЯ РЕШЕНИЕ  
> **Цель**: Закрыть ВСЕ архитектурные пробелы ПЕРЕД началом кодирования

---

## ⚠️ СТАТУС: НЕ ГОТОВ К РАЗРАБОТКЕ

**Готовность**: 65%  
**Блокеры**: 12 критических вопросов без ответа  
**ETA готовности**: 1-2 недели на проектирование

---

## 🔴 БЛОКЕР #1: Регистрация и lifecycle трейдеров

### Вопросы

#### 1.1 Как трейдеры регистрируются в системе?

**ВАРИАНТЫ:**

**A) Автоматическая регистрация (self-registration):**
```
1. Трейдер запускается с config:
   - trader_id: "trader-eu-1"
   - cts_core_url: "wss://cts-core:8443/ws/trader"
   - certificate: /pki/trader-eu-1.crt

2. Трейдер подключается по WebSocket
3. Отправляет: trader.register (request)
4. CTS-Core:
   - Проверяет mTLS certificate (OU=Trading)
   - Создает запись в TRADER_SESSION (MySQL)
   - Возвращает: trader.register_ack

✅ ПЛЮСЫ:
   - Простота deployment
   - Динамическое масштабирование
   - Минимум ручной работы

❌ МИНУСЫ:
   - Нужна pre-provisioning сертификатов
   - Риск несанкционированного подключения
```

**B) Ручная регистрация через admin:**
```
1. Admin через web UI:
   - Создает запись TRADER в MySQL:
     - trader_id: "trader-eu-1"
     - status: "registered"
     - certificate_cn: "trader-eu-1"
   
2. Admin генерирует сертификат для трейдера
3. Admin устанавливает и запускает трейдер
4. Трейдер подключается с pre-registered trader_id

✅ ПЛЮСЫ:
   - Полный контроль
   - Аудит trail
   - Безопасность

❌ МИНУСЫ:
   - Ручная работа
   - Сложность автоскейлинга
```

**❓ ВОПРОС:** Какой вариант выбираем? Или гибрид?

**💡 РЕКОМЕНДАЦИЯ:**
```
ГИБРИДНЫЙ ПОДХОД:

1. TRADER таблица в MySQL (pre-registration):
   CREATE TABLE TRADER (
       ID INT PRIMARY KEY AUTO_INCREMENT,
       TRADER_ID VARCHAR(100) UNIQUE NOT NULL,
       CERTIFICATE_CN VARCHAR(255) NOT NULL,
       REGION VARCHAR(50),
       STATUS ENUM('registered', 'active', 'suspended', 'decommissioned'),
       MAX_CONCURRENT_TASKS INT DEFAULT 10,
       DATE_CREATE TIMESTAMP,
       DATE_MODIFY TIMESTAMP
   );

2. Процесс:
   - Admin создает TRADER запись (pre-registration)
   - Admin генерирует certificate с CN = trader_id
   - Трейдер подключается → CTS-Core проверяет:
     * mTLS certificate valid
     * trader_id exists in TRADER table
     * STATUS = 'registered' or 'active'
   - CTS-Core создает TRADER_SESSION (active session)
   - Обновляет TRADER.STATUS = 'active'

3. TRADER_SESSION (runtime state):
   CREATE TABLE TRADER_SESSION (
       ID INT PRIMARY KEY AUTO_INCREMENT,
       TRADER_ID VARCHAR(100) NOT NULL,
       SESSION_ID VARCHAR(100) UNIQUE NOT NULL,
       WS_CONNECTION_ID VARCHAR(255),
       CONNECTED_AT TIMESTAMP NOT NULL,
       LAST_HEARTBEAT TIMESTAMP NOT NULL,
       STATUS ENUM('connected', 'disconnected', 'timeout'),
       FOREIGN KEY (TRADER_ID) REFERENCES TRADER(TRADER_ID)
   );
```

---

#### 1.2 Как отличать временный disconnect от permanent shutdown?

**СЦЕНАРИИ:**

```
A) Временный network glitch (5-30 сек):
   - Heartbeat timeout
   - Трейдер переподключается
   → Восстановить сессию, НЕ переназначать задачи

B) Graceful shutdown (maintenance):
   - CTS-Core → trader.shutdown (request)
   - Трейдер завершает работу, отключается
   → Переназначить задачи, НЕ удалять TRADER запись

C) Crash (авария):
   - Heartbeat timeout > 30 сек
   - Трейдер не отвечает
   → Failover: переназначить задачи другому трейдеру

D) Decommission (навсегда):
   - Admin через UI: TRADER.STATUS = 'decommissioned'
   - CTS-Core → trader.shutdown
   → Переназначить задачи, удалить TRADER_SESSION
```

**❓ ВОПРОС:** Какие таймауты?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
TIMEOUT POLICY:

heartbeat_interval: 5 sec          # Трейдер → CTS-Core
heartbeat_timeout: 15 sec          # Если нет heartbeat → disconnect
reconnect_grace_period: 60 sec    # Время на переподключение
failover_trigger: 60 sec           # Если не reconnect → failover

СОСТОЯНИЯ:
connected:
  - Heartbeat received < 15 sec ago
  - Задачи active

disconnected:
  - Heartbeat timeout > 15 sec
  - Но < 60 sec total
  - Задачи suspended (waiting for reconnect)

timeout:
  - No heartbeat > 60 sec
  - Trigger failover
  - Reassign все задачи
  - TRADER_SESSION.STATUS = 'timeout'

RECONNECT LOGIC:
If trader reconnects within 60 sec:
  - Restore session (same session_id)
  - Resume tasks
  - Update LAST_HEARTBEAT
Else:
  - Create new session
  - Old tasks reassigned
```

---

#### 1.3 Кто удаляет записи TRADER_SESSION?

**ВАРИАНТЫ:**

**A) Никогда не удаляем (append-only log):**
```sql
-- TRADER_SESSION становится audit log
-- Только помечаем STATUS, никогда не DELETE
-- Плюсы: полная история
-- Минусы: таблица растет бесконечно
```

**B) Cleanup старых сессий (retention policy):**
```sql
-- Удаляем сессии старше 30 дней
DELETE FROM TRADER_SESSION 
WHERE STATUS IN ('disconnected', 'timeout')
AND CONNECTED_AT < NOW() - INTERVAL 30 DAY;

-- Запускается по cron (раз в сутки)
```

**❓ ВОПРОС:** Какой подход?

**💡 РЕКОМЕНДАЦИЯ:**
```
ВАРИАНТ B + архивация:

1. TRADER_SESSION_ARCHIVE таблица (для истории)
2. Каждую ночь (02:00 UTC):
   - Переносим старые сессии (> 7 дней) в ARCHIVE
   - DELETE из TRADER_SESSION
3. TRADER_SESSION содержит только активные + последние 7 дней
4. TRADER_SESSION_ARCHIVE для audit/analytics
```

---

## 🔴 БЛОКЕР #2: State Management и Failover

### Вопросы

#### 2.1 Где хранится критический state при restart CTS-Core?

**ПРОБЛЕМА:**
```
При restart CTS-Core теряется:
├─ Список подключенных трейдеров
├─ Активные task assignments
├─ Pending requests (correlation_id tracking)
└─ WebSocket connection state
```

**ВАРИАНТЫ:**

**A) Только MySQL (медленно):**
```
+ Простота реализации
+ Нет дополнительных зависимостей
- Latency записи ~10-50ms
- Не подходит для real-time state
```

**B) Redis (in-memory with persistence):**
```
+ Latency ~1ms
+ Persistence (RDB/AOF snapshots)
+ Redis Sentinel для HA
- Еще одна зависимость
- Нужна синхронизация с MySQL
```

**C) Local state file + MySQL sync:**
```
+ Быстрое восстановление
+ Работает без Redis
- Нет sharing между CTS-Core instances
- Failover сложнее
```

**❓ ВОПРОС:** Какой вариант? Учесть future failover.

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
PHASE 1 (MVP): 
  - Local state file (daemon.state) + MySQL
  - При restart:
    1. Load from daemon.state
    2. Query MySQL для consistency check
    3. Reconcile differences
  - TRADER_SESSION table = source of truth
  
PHASE 2 (Production):
  - Redis для hot state
  - MySQL для persistent records
  - Sync: Redis → MySQL каждые 5 сек
  - Recovery: Redis AOF + MySQL fallback
```

---

#### 2.2 Failover CTS-Core: Active-Passive или Distributed?

**ВАРИАНТЫ:**

**A) Active-Passive (простое):**
```
[CTS-Core-1 ACTIVE]  ←→  [CTS-Core-2 STANDBY]
         ↓                        ↑
    Heartbeat (UDP)              |
         ↓                        |
    State replication       Takes over if
    (async, Redis)          Primary fails

ПЛЮСЫ:
+ Простая реализация
+ Failover < 10 сек
+ Подходит для 50 traders

МИНУСЫ:
- Standby простаивает (50% waste)
- Split-brain риск
```

**B) Distributed cluster (сложное):**
```
[CTS-Core-1] ←→ [CTS-Core-2] ←→ [CTS-Core-3]
      ↓              ↓              ↓
    [etcd/Consul] (leader election)
    
ПЛЮСЫ:
+ High availability
+ Load distribution
+ Масштабирование

МИНУСЫ:
- Сложная реализация
- Нужна координация (etcd/Consul)
- Оverkill для текущего масштаба
```

**❓ ВОПРОС:** Сейчас или в будущем? Budget/complexity?

**💡 РЕКОМЕНДАЦИЯ:**
```
START: Active-Passive (Phase 1.5)
FUTURE: Distributed (Phase 3+, если > 100 traders)

PHASE 1.5 DESIGN:
1. Два экземпляра CTS-Core
2. Shared Redis (Redis Sentinel 3 nodes)
3. VIP (Virtual IP) для CTS-Core endpoint
4. Keepalived для failover
5. Трейдеры подключаются к VIP → automatic redirect

IMPLEMENTATION:
- Keepalived daemon на обоих CTS-Core
- Health check: HTTP /health endpoint
- Failover time: ~5 сек
- Cost: +1 VM для standby
```

---

## 🔴 БЛОКЕР #3: Database Schema Gaps

### Недостающие таблицы

#### 3.1 TRADER (pre-registration)

**❓ ВОПРОС:** Нужна ли эта таблица вообще? Или только TRADER_SESSION?

**💡 РЕКОМЕНДАЦИЯ:**
```sql
ДА, нужны ОБЕ таблицы:

-- TRADER: static configuration (pre-registered traders)
CREATE TABLE TRADER (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    TRADER_ID VARCHAR(100) UNIQUE NOT NULL,
    CERTIFICATE_CN VARCHAR(255) NOT NULL,
    
    -- Configuration
    REGION VARCHAR(50),
    MAX_CONCURRENT_TASKS INT DEFAULT 10,
    CAPABILITIES JSON,  -- ["binance", "kucoin", ...]
    
    -- Status
    STATUS ENUM('registered', 'active', 'suspended', 'decommissioned') DEFAULT 'registered',
    
    -- Metadata
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATE_MODIFY TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    USER_CREATED INT NOT NULL,
    
    INDEX idx_status (STATUS),
    INDEX idx_region (REGION)
) ENGINE=InnoDB;

-- TRADER_SESSION: runtime state (active connections)
CREATE TABLE TRADER_SESSION (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    TRADER_ID VARCHAR(100) NOT NULL,
    SESSION_ID VARCHAR(100) UNIQUE NOT NULL,
    WS_CONNECTION_ID VARCHAR(255),
    
    -- Timestamps
    CONNECTED_AT TIMESTAMP NOT NULL,
    LAST_HEARTBEAT TIMESTAMP NOT NULL,
    DISCONNECTED_AT TIMESTAMP NULL,
    
    -- Status
    STATUS ENUM('connected', 'disconnected', 'timeout') DEFAULT 'connected',
    
    -- Metrics snapshot
    ACTIVE_TASKS INT DEFAULT 0,
    CPU_USAGE DECIMAL(5,2),
    MEMORY_USAGE DECIMAL(5,2),
    
    FOREIGN KEY (TRADER_ID) REFERENCES TRADER(TRADER_ID) ON DELETE CASCADE,
    INDEX idx_status (STATUS),
    INDEX idx_heartbeat (LAST_HEARTBEAT),
    INDEX idx_trader (TRADER_ID)
) ENGINE=InnoDB;
```

**РАЗДЕЛЕНИЕ ОТВЕТСТВЕННОСТИ:**
- `TRADER`: Who CAN connect (authorization)
- `TRADER_SESSION`: Who IS connected (authentication + runtime)

---

#### 3.2 MONITOR_PAIR и связанные

**❓ ВОПРОС:** Конфигурация MONITOR задач через БД или через config файл?

**💡 РЕКОМЕНДАЦИЯ:**
```
ЧЕРЕЗ БД (dynamic configuration):

ПРИЧИНЫ:
✅ Web UI может добавлять/удалять пары для мониторинга
✅ Не нужен restart трейдеров
✅ Централизованное управление
✅ Audit trail

SQL:
CREATE TABLE MONITOR_PAIR (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    PAIR_ID INT NOT NULL,
    ACTIVE TINYINT(1) NOT NULL DEFAULT 1,
    
    -- Settings
    DATA_TYPES JSON NOT NULL,  -- ["ticks", "orderbook", "ohlc_1m", "ohlc_5m"]
    ORDERBOOK_DEPTH INT DEFAULT 20,
    OHLC_INTERVALS JSON DEFAULT '["1m", "5m", "15m", "1h"]',
    
    -- Assignment
    ASSIGNED_TRADER_ID VARCHAR(100) NULL,
    FAILOVER_TRADER_ID VARCHAR(100) NULL,
    PRIORITY INT DEFAULT 5,
    
    -- Metadata
    DATE_CREATE TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    DATE_MODIFY TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    USER_CREATED INT NOT NULL,
    
    FOREIGN KEY (PAIR_ID) REFERENCES SPOT_TRADE_PAIR(ID),
    FOREIGN KEY (ASSIGNED_TRADER_ID) REFERENCES TRADER(TRADER_ID),
    FOREIGN KEY (FAILOVER_TRADER_ID) REFERENCES TRADER(TRADER_ID),
    INDEX idx_active (ACTIVE),
    INDEX idx_assigned (ASSIGNED_TRADER_ID)
) ENGINE=InnoDB;

CREATE TABLE MONITOR_PAIR_EXCHANGE (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    MONITOR_PAIR_ID INT NOT NULL,
    EXCHANGE_ID INT NOT NULL,
    ACTIVE TINYINT(1) NOT NULL DEFAULT 1,
    
    FOREIGN KEY (MONITOR_PAIR_ID) REFERENCES MONITOR_PAIR(ID) ON DELETE CASCADE,
    FOREIGN KEY (EXCHANGE_ID) REFERENCES EXCHANGE(ID),
    UNIQUE KEY uk_monitor_exchange (MONITOR_PAIR_ID, EXCHANGE_ID)
) ENGINE=InnoDB;
```

---

#### 3.3 EXCHANGE_LIMITS и TRADER_EXCHANGE_RESOURCE

**❓ ВОПРОС:** Обязательны ли эти таблицы или можно hardcode лимиты?

**💡 РЕКОМЕНДАЦИЯ:**
```
ОБЯЗАТЕЛЬНЫ - БЕЗ НИХ scheduler не может работать правильно

EXCHANGE_LIMITS: Source of truth для биржевых лимитов
TRADER_EXCHANGE_RESOURCE: Real-time tracking использования

CREATE TABLE EXCHANGE_LIMITS (
    EXCHANGE_ID INT PRIMARY KEY,
    MAX_WS_CONNECTIONS INT NOT NULL DEFAULT 10,
    MAX_SUBSCRIPTIONS_PER_WS INT NOT NULL DEFAULT 35,
    
    -- Rate limits
    ORDERS_PER_SECOND INT DEFAULT 10,
    REQUESTS_PER_MINUTE INT DEFAULT 1200,
    
    -- Notes
    NOTES TEXT,
    LAST_UPDATED TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    FOREIGN KEY (EXCHANGE_ID) REFERENCES EXCHANGE(ID)
) ENGINE=InnoDB;

-- Initial data (from exchange documentation)
INSERT INTO EXCHANGE_LIMITS VALUES
(1, 10, 35, 10, 1200, 'Binance - https://binance-docs.github.io/apidocs/spot/en/#limits', NOW()),
(2, 10, 30, 10, 1200, 'KuCoin', NOW()),
(3, 10, 30, 10, 1200, 'Bybit', NOW()),
(4, 10, 35, 10, 1200, 'OKX', NOW());

CREATE TABLE TRADER_EXCHANGE_RESOURCE (
    ID INT PRIMARY KEY AUTO_INCREMENT,
    TRADER_ID VARCHAR(100) NOT NULL,
    EXCHANGE_ID INT NOT NULL,
    
    -- Current usage (updated real-time)
    CURRENT_WS_COUNT INT NOT NULL DEFAULT 0,
    TOTAL_SUBSCRIPTIONS INT NOT NULL DEFAULT 0,
    
    -- Breakdown by task type
    TRADE_TASK_COUNT INT NOT NULL DEFAULT 0,
    MONITOR_TASK_COUNT INT NOT NULL DEFAULT 0,
    
    -- Last update
    LAST_UPDATED TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    FOREIGN KEY (TRADER_ID) REFERENCES TRADER(TRADER_ID) ON DELETE CASCADE,
    FOREIGN KEY (EXCHANGE_ID) REFERENCES EXCHANGE(ID),
    UNIQUE KEY uk_trader_exchange (TRADER_ID, EXCHANGE_ID),
    INDEX idx_usage (CURRENT_WS_COUNT, TOTAL_SUBSCRIPTIONS)
) ENGINE=InnoDB;
```

**ИСПОЛЬЗОВАНИЕ В SCHEDULER:**
```go
func (s *Scheduler) CanAssignTask(traderID string, task Task) bool {
    // 1. Get exchange limits
    limits := s.db.GetExchangeLimits(task.ExchangeID)
    
    // 2. Get current usage
    usage := s.db.GetTraderExchangeResource(traderID, task.ExchangeID)
    
    // 3. Calculate needed resources
    neededWS := task.EstimateWSConnections()
    neededSubs := task.EstimateSubscriptions()
    
    // 4. Check capacity
    if usage.CurrentWSCount + neededWS > limits.MaxWSConnections {
        return false
    }
    if usage.TotalSubscriptions + neededSubs > limits.MaxSubscriptionsPerWS * limits.MaxWSConnections {
        return false
    }
    
    return true
}
```

---

## 🔴 БЛОКЕР #4: Idempotency и Deduplication

### Вопросы

#### 4.1 Как гарантировать idempotency для trade.result?

**ПРОБЛЕМА:**
```
Сценарий:
1. Trader → CTS-Core: trade.result (arbitrage_id=12345)
2. CTS-Core → MySQL: INSERT ARBITRAGE_TRANS
3. Network error перед ack
4. Trader retry: trade.result (arbitrage_id=12345)
5. CTS-Core → MySQL: INSERT ARBITRAGE_TRANS (дубль!)
```

**ВАРИАНТЫ:**

**A) UNIQUE constraint в MySQL:**
```sql
ALTER TABLE ARBITRAGE_TRANS
ADD CONSTRAINT uk_arbitrage_id UNIQUE (ID);

-- Проблема: ID генерируется MySQL (AUTO_INCREMENT)
-- Нужно передавать ID от trader'а
```

**B) Application-level deduplication:**
```go
// In-memory cache последних 10K arbitrage_id
cache := make(map[int64]bool, 10000)

func (h *Handler) ProcessTradeResult(msg TradeResult) error {
    if cache[msg.ArbitrageID] {
        return nil // Already processed
    }
    
    // Insert to DB
    err := h.db.InsertArbitrageTrans(msg)
    if err != nil {
        return err
    }
    
    cache[msg.ArbitrageID] = true
    return nil
}
```

**C) MySQL ON DUPLICATE KEY UPDATE:**
```sql
INSERT INTO ARBITRAGE_TRANS (ID, TRADE_ID, STATUS, ...)
VALUES (?, ?, ?, ...)
ON DUPLICATE KEY UPDATE
    DATE_MODIFY = NOW(),
    STATUS = VALUES(STATUS);
```

**❓ ВОПРОС:** Как генерируется arbitrage_id? Кто отвечает?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
DECISION:
1. arbitrage_id генерируется CTS-CORE (не trader)
2. Процесс:
   - Trader → task.assign (trade_id=123)
   - Trader finds opportunity
   - Trader → CTS-Core: trade.intent (request)
   - CTS-Core → MySQL: INSERT ARBITRAGE_TRANS (STATUS=New)
   - CTS-Core → Trader: arbitrage_id=12345
   - Trader executes orders
   - Trader → CTS-Core: trade.result (arbitrage_id=12345)
   - CTS-Core → MySQL: UPDATE ARBITRAGE_TRANS SET STATUS=Complete

IDEMPOTENCY:
   - UPDATE по arbitrage_id (always idempotent)
   - Нет INSERT дублей
   - Application cache не нужен

NEW MESSAGE:
{
  "type": "request",
  "action": "trade.intent",
  "payload": {
    "task_id": "task-12345",
    "trade_id": 123,
    "opportunity": {
      "exchange_buy": "binance",
      "price_buy": "50000.00",
      "exchange_sell": "kucoin",
      "price_sell": "50150.00",
      "quantity": "0.5",
      "estimated_profit": "75.00"
    }
  }
}

RESPONSE:
{
  "type": "response",
  "action": "trade.intent_ack",
  "correlation_id": "...",
  "payload": {
    "arbitrage_id": 12345,
    "approved": true,
    "max_execution_time_sec": 10
  }
}
```

**ИЗМЕНЕНИЯ В АРХИТЕКТУРЕ:**
```
БЫЛО:
  Trader → finds opportunity → executes → trade.result
  
СТАЛО:
  Trader → finds opportunity → trade.intent → 
  ← arbitrage_id → executes → trade.result
  
ПЛЮСЫ:
  ✅ Idempotency гарантирована
  ✅ CTS-Core контролирует каждую сделку
  ✅ Можно reject сделку (risk limits)
  ✅ Audit trail полный
```

---

#### 4.2 Deduplication ARBITRAGE_ORDER

**❓ ВОПРОС:** Как обеспечить уникальность ордеров?

**💡 РЕКОМЕНДАЦИЯ:**
```sql
-- ARBITRAGE_ORDER должна иметь UNIQUE на (EXCHANGE_ACCOUNT_ID, EXCHANGE_ORDER_ID)
ALTER TABLE ARBITRAGE_ORDER
ADD CONSTRAINT uk_exchange_order 
UNIQUE (EXCHANGE_ACCOUNT_ID, EXCHANGE_ORDER_ID);

-- При INSERT использовать ON DUPLICATE KEY UPDATE:
INSERT INTO ARBITRAGE_ORDER (
    ARBITRAGE_ID,
    EXCHANGE_ACCOUNT_ID,
    EXCHANGE_ORDER_ID,
    ORDER_TYPE,
    SIDE,
    PRICE,
    AMOUNT,
    STATUS
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    STATUS = VALUES(STATUS),
    FILLED_AMOUNT = VALUES(FILLED_AMOUNT),
    DATE_MODIFY = NOW();
```

---

## 🔴 БЛОКЕР #5: REST API vs WebSocket

### Вопросы

#### 5.1 Какие операции через REST, какие через WebSocket?

**ПРОБЛЕМА:**
Сейчас в документации смешение: trade.result через WS, но где REST endpoints?

**❓ ВОПРОС:** Четкое разделение ответственности?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
WEBSOCKET (real-time bidirectional):
  Traders:
    - trader.register, trader.heartbeat
    - task.assign, task.cancel, task.config_update
    - trade.intent, trade.result
    - monitor.result
    - metrics.report
    - latency.test
    
  Web UI:
    - web.stats_realtime (streaming)
    - web.notifications (events)

REST API (request/response, stateless):
  Admin:
    - GET /api/v1/traders (list all traders)
    - GET /api/v1/traders/{id} (trader details)
    - POST /api/v1/traders (register new trader)
    - PUT /api/v1/traders/{id} (update config)
    - DELETE /api/v1/traders/{id} (decommission)
    
    - GET /api/v1/trades (list trades)
    - GET /api/v1/trades/{id} (trade details)
    - POST /api/v1/trades (create trade config)
    - PUT /api/v1/trades/{id} (update trade config)
    - DELETE /api/v1/trades/{id} (deactivate trade)
    
    - GET /api/v1/arbitrage (list transactions)
    - GET /api/v1/arbitrage/{id} (transaction details)
    - GET /api/v1/arbitrage/{id}/orders (orders for transaction)
    
    - GET /api/v1/monitor-pairs (list monitor configs)
    - POST /api/v1/monitor-pairs (create monitor)
    - PUT /api/v1/monitor-pairs/{id} (update monitor)
    - DELETE /api/v1/monitor-pairs/{id} (stop monitoring)
    
  Public:
    - GET /health (health check)
    - GET /metrics (Prometheus metrics)
    - GET /version (version info)

ПРАВИЛО:
  - CRUD операции → REST API
  - Real-time communication → WebSocket
  - Admin управление → REST API
  - Trader операции → WebSocket
```

---

## 🔴 БЛОКЕР #6: Error Handling и Retry Logic

### Вопросы

#### 6.1 Retry policy для каждого типа операций?

**❓ ВОПРОС:** Сколько retries, какие backoff intervals?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
RETRY POLICY:

trade.intent (request):
  max_retries: 3
  backoff: exponential (1s, 2s, 4s)
  timeout: 5 sec per attempt
  on_failure: reject trade opportunity

trade.result (event):
  max_retries: 5
  backoff: exponential (2s, 4s, 8s, 16s, 32s)
  timeout: 10 sec per attempt
  on_failure: log to FAILED_TRADES table, alert admin

task.assign (request):
  max_retries: 1
  backoff: immediate
  timeout: 3 sec
  on_failure: try another trader

monitor.result (event):
  max_retries: 0
  on_failure: log warning (data loss acceptable)

metrics.report (event):
  max_retries: 1
  backoff: 5 sec
  on_failure: skip (will retry next interval)
```

---

#### 6.2 Circuit breaker для бирж?

**❓ ВОПРОС:** Как handling массовых ошибок от биржи?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
CIRCUIT BREAKER (per exchange):

States:
  CLOSED: Normal operation
  OPEN: Errors detected, stop sending requests
  HALF_OPEN: Testing if recovered

Thresholds:
  error_threshold: 5 consecutive errors
  timeout_threshold: 3 consecutive timeouts
  open_duration: 60 sec
  half_open_test_count: 1

Logic:
  CLOSED → OPEN:
    IF 5 consecutive errors OR 3 timeouts
    THEN:
      - Mark exchange DOWN in memory
      - Stop assigning tasks for this exchange
      - Open circuit for 60 sec
      - Log alert
  
  OPEN → HALF_OPEN:
    AFTER 60 sec:
      - Send test request (ping or small order)
      - Wait for result
  
  HALF_OPEN → CLOSED:
    IF test successful:
      - Resume normal operation
      - Reset error counters
  
  HALF_OPEN → OPEN:
    IF test failed:
      - Stay OPEN for another 60 sec
      - Log persistent error

IMPLEMENTATION:
  - In-memory CircuitBreaker map per exchange
  - Shared across all CTS-Core instances (via Redis)
```

---

## 🔴 БЛОКЕР #7: Observability

### Вопросы

#### 7.1 Какие метрики собираем?

**❓ ВОПРОС:** Полный список метрик для Prometheus?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
PROMETHEUS METRICS:

# CTS-Core Server
cts_core_active_traders (gauge)
cts_core_tasks_assigned_total (counter) [labels: task_type, status]
cts_core_tasks_failed_total (counter) [labels: task_type, reason]
cts_core_websocket_connections (gauge) [labels: client_type]
cts_core_websocket_messages_total (counter) [labels: action, direction]
cts_core_api_requests_total (counter) [labels: endpoint, method, status]
cts_core_api_latency_seconds (histogram) [labels: endpoint]
cts_core_db_queries_total (counter) [labels: query_type, status]
cts_core_db_latency_seconds (histogram)

# Task Scheduler
cts_core_scheduler_queue_size (gauge)
cts_core_scheduler_assignment_latency_seconds (histogram)
cts_core_scheduler_assignment_failures_total (counter) [labels: reason]

# Arbitrage
cts_arbitrage_opportunities_total (counter) [labels: pair, exchanges]
cts_arbitrage_executed_total (counter) [labels: status]
cts_arbitrage_profit_usdt (counter)
cts_arbitrage_execution_latency_seconds (histogram)

# Traders (reported by trader, aggregated by core)
cts_trader_cpu_usage_percent (gauge) [labels: trader_id]
cts_trader_memory_usage_percent (gauge) [labels: trader_id]
cts_trader_active_tasks (gauge) [labels: trader_id, task_type]
cts_trader_orders_per_second (gauge) [labels: trader_id, exchange]
cts_trader_ws_connections (gauge) [labels: trader_id, exchange]
cts_trader_exchange_latency_ms (gauge) [labels: trader_id, exchange]

# System
go_goroutines (gauge)
go_memstats_alloc_bytes (gauge)
process_cpu_seconds_total (counter)
```

---

#### 7.2 Structured logging format?

**❓ ВОПРОС:** JSON или text? Какие обязательные поля?

**💡 РЕКОМЕНДАЦИЯ:**
```json
{
  "timestamp": "2026-01-27T15:04:05.123456Z",
  "level": "INFO",
  "component": "scheduler",
  "message": "Task assigned successfully",
  "trader_id": "trader-eu-1",
  "task_id": "task-12345",
  "task_type": "trade",
  "trade_id": 123,
  "correlation_id": "uuid",
  "latency_ms": 45,
  "error": null
}

LEVELS: DEBUG, INFO, WARN, ERROR, FATAL
REQUIRED FIELDS: timestamp, level, component, message
OPTIONAL CONTEXT: trader_id, task_id, correlation_id, error, stack_trace
```

---

## 🟡 ВОПРОСЫ СРЕДНЕЙ КРИТИЧНОСТИ

### 8. Масштабирование

#### 8.1 Сколько трейдеров максимум?

**❓ ВОПРОС:** Target capacity для Phase 1?

**💡 РЕКОМЕНДАЦИЯ:**
```
Phase 1: 10 traders (MVP)
Phase 2: 50 traders (Production)
Phase 3: 100+ traders (Scale)

Design for 100, implement for 50.
```

---

#### 8.2 Load balancing алгоритм?

**❓ ВОПРОС:** Как scheduler выбирает trader для задачи?

**💡 РЕКОМЕНДАЦИЯ:**
```go
type AssignmentScore struct {
    TraderID string
    Score    float64
}

func (s *Scheduler) CalculateScore(trader Trader, task Task) float64 {
    score := 100.0
    
    // 1. Latency to exchange (weight: 40%)
    latency := s.GetLatency(trader.ID, task.ExchangeID)
    latencyScore := 100.0 - (latency / 10.0) // 10ms = -1 point
    score += latencyScore * 0.4
    
    // 2. Current load (weight: 30%)
    loadPercent := trader.ActiveTasks / trader.MaxTasks * 100
    loadScore := 100.0 - loadPercent
    score += loadScore * 0.3
    
    // 3. Resource availability (weight: 20%)
    resourceScore := s.CheckResourceAvailability(trader.ID, task)
    score += resourceScore * 0.2
    
    // 4. Region affinity (weight: 10%)
    if trader.Region == task.PreferredRegion {
        score += 10
    }
    
    return score
}

// Выбираем trader с highest score
```

---

### 9. Security

#### 9.1 Rate limiting на API?

**❓ ВОПРОС:** Нужны ли rate limits для REST API?

**💡 РЕКОМЕНДАЦИЯ:**
```yaml
YES - защита от abuse:

LIMITS:
  - /api/v1/* : 100 requests/min per IP
  - /health, /metrics : unlimited (monitoring)
  - WebSocket connections: 5 per IP (traders pre-authorized)

IMPLEMENTATION:
  - Middleware с token bucket algorithm
  - Redis для shared state (если несколько CTS-Core)
  - HTTP 429 Too Many Requests
```

---

#### 9.2 Audit log для admin operations?

**❓ ВОПРОС:** Логировать все admin actions?

**💡 РЕКОМЕНДАЦИЯ:**
```sql
YES - compliance requirement:

CREATE TABLE AUDIT_LOG (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    USER_ID INT NOT NULL,
    ACTION VARCHAR(100) NOT NULL,
    RESOURCE_TYPE VARCHAR(50) NOT NULL,
    RESOURCE_ID VARCHAR(255),
    OLD_VALUE JSON,
    NEW_VALUE JSON,
    IP_ADDRESS VARCHAR(45),
    USER_AGENT TEXT,
    TIMESTAMP TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_user (USER_ID),
    INDEX idx_timestamp (TIMESTAMP),
    INDEX idx_resource (RESOURCE_TYPE, RESOURCE_ID)
) ENGINE=InnoDB;

LOGGED ACTIONS:
  - trader.create, trader.update, trader.delete
  - trade.create, trade.update, trade.delete
  - monitor.create, monitor.update, monitor.delete
  - system.config_change
```

---

## 📋 ЧЕКЛИСТ ГОТОВНОСТИ К РАЗРАБОТКЕ

### ✅ Должно быть решено ПЕРЕД Phase 1:

- [ ] **Trader registration mechanism** (automatic vs manual vs hybrid)
- [ ] **State persistence strategy** (MySQL only vs Redis vs local file)
- [ ] **Failover design decision** (Phase 1.5? Active-Passive?)
- [ ] **SQL migrations** для TRADER, MONITOR_PAIR, EXCHANGE_LIMITS, TRADER_EXCHANGE_RESOURCE
- [ ] **Idempotency strategy** (trade.intent flow + deduplication)
- [ ] **REST vs WebSocket split** (четкое разделение API)
- [ ] **Retry policy** для всех операций
- [ ] **Circuit breaker** для бирж
- [ ] **Metrics specification** (Prometheus exporter)
- [ ] **Logging format** (structured JSON)
- [ ] **Timeout values** (heartbeat, reconnect, failover)
- [ ] **Error codes** (standardized list)

### ⏳ Можно отложить до Phase 2:

- [ ] Distributed CTS-Core cluster (etcd/Consul)
- [ ] Advanced load balancing (ML-based)
- [ ] Auto-scaling трейдеров
- [ ] Comprehensive chaos testing
- [ ] Performance tuning (query optimization)

---

## 🎯 NEXT STEPS

1. **REVIEW этот документ** - обсудить каждый вопрос
2. **DECIDE на каждый блокер** - принять архитектурные решения
3. **UPDATE документацию**:
   - API_SPECIFICATION.md (unified REST + WebSocket)
   - ARCHITECTURE.md (добавить State Management section)
   - DATABASE_SCHEMA.sql (complete migrations)
4. **CREATE Phase 0.5** в DEVELOPMENT_PLAN.md (Architecture Hardening)
5. **START Phase 1** только после закрытия всех 🔴 блокеров

---

**❓ ВОПРОС К РАЗРАБОТЧИКУ:**
По каким пунктам нужны решения СЕЙЧАС? Давай пройдемся по блокерам 1-7 и закроем их.
