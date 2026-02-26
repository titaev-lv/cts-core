# Rate Limits Architecture

**Date:** 2026-01-29  
**Decision:** Rate limits managed by TRADERS, not Core

---

## Problem

Exchange rate limits (API requests, order frequency) are **IP-bound**:
- Binance: 1200 requests/minute per IP
- OKX: 100 requests/2 seconds per IP  
- Bybit: varies by endpoint + IP

**CTS-Core does NOT know trader IP addresses** (traders connect via WebSocket, but work with exchanges from their own IPs).

---

## Solution: Autonomous Trader Rate Limit Management

### Architecture

```
┌─────────────────┐
│   CTS-Core      │
│                 │
│  - Distributes  │──── Task ────▶ ┌──────────────┐
│    tasks        │                │  Trader-EU   │
│  - Receives     │◀─── Metrics ───│              │
│    metrics      │                │ IP: X.X.X.X  │
└─────────────────┘                │              │
                                   │ Rate Limits: │
                                   │ - 800/1200   │──┐
                                   │   req/min    │  │
                                   └──────────────┘  │
                                                     │ Trader
                                                     │ manages
                                   ┌──────────────┐  │ limits
                                   │  Binance API │◀─┘
                                   │              │
                                   │ Headers:     │
                                   │ X-MBX-USED-  │
                                   │ WEIGHT: 800  │
                                   └──────────────┘
```

### How It Works

**1. Trader discovers limits** (from exchange API headers):
```http
X-MBX-USED-WEIGHT: 800
X-MBX-USED-WEIGHT-1M: 800
X-RATELIMIT-LIMIT: 1200
```

**2. Trader reports metrics to Core** (every 10-30s via WebSocket):
```json
{
  "type": "metrics",
  "trader_id": "trader-eu-1",
  "resources": [
    {
      "exchange_id": 1,  // Binance
      "resource_type": "api_requests_minute",
      "used_value": 800,
      "max_value": 1200,
      "reset_at": "2026-01-29T10:35:00Z"
    }
  ]
}
```

**3. Core stores in `TRADER_EXCHANGE_RESOURCE` table:**
```sql
INSERT INTO TRADER_EXCHANGE_RESOURCE 
(TRADER_ID, EXCHANGE_ID, RESOURCE_TYPE, USED_VALUE, MAX_VALUE, RESET_AT)
VALUES (1, 1, 'api_requests_minute', 800, 1200, '2026-01-29 10:35:00')
ON DUPLICATE KEY UPDATE 
  USED_VALUE = VALUES(USED_VALUE),
  LAST_UPDATED = CURRENT_TIMESTAMP;
```

**4. Core uses metrics for task distribution:**
```javascript
// Scheduler scoring algorithm
traders.forEach(trader => {
  const availability = (trader.max_value - trader.used_value) / trader.max_value;
  score += availability * 0.3;  // 30% weight for load
});
```

**5. Trader makes FINAL decision:**
```javascript
// Trader receives task from Core
onTask(task) {
  if (this.rateLimitExceeded()) {
    return { status: "rejected", reason: "rate_limit", retry_after: 30 };
  }
  
  // Execute task
  return this.executeOrder(task);
}
```

---

## Why NOT Store Limits in Core

❌ **What we DON'T do:**
```sql
-- NO: EXCHANGE_LIMITS table (centralized config)
CREATE TABLE EXCHANGE_LIMITS (
  EXCHANGE_ID INT,
  LIMIT_TYPE ENUM('orders_per_day', 'api_requests_minute'),
  MAX_VALUE DECIMAL(20, 8)
);
```

**Problems:**
1. Limits vary by **IP** (each trader has different IP)
2. Limits vary by **account level** (VIP 0 vs VIP 9 on Binance)
3. Limits vary by **endpoint** (order endpoint vs market data)
4. Limits change **dynamically** (exchange adjusts in real-time)
5. Core doesn't know when limits reset (timezone, DST issues)

---

## Database Schema

### TRADER_EXCHANGE_RESOURCE (Metrics Storage)

```sql
CREATE TABLE TRADER_EXCHANGE_RESOURCE (
    ID BIGINT PRIMARY KEY AUTO_INCREMENT,
    TRADER_ID INT NOT NULL COMMENT 'TRADER.ID',
    EXCHANGE_ID INT NOT NULL COMMENT 'EXCHANGE.ID',
    
    -- Resource types trader can report
    RESOURCE_TYPE ENUM(
        'api_requests_minute',    -- General API rate limit
        'api_weight_minute',      -- Binance weight-based limit
        'orders_minute',          -- Order frequency limit
        'websocket_connections'   -- Max WS connections
    ) NOT NULL,
    
    -- Current usage (from trader)
    USED_VALUE DECIMAL(20, 8) NOT NULL DEFAULT 0,
    MAX_VALUE DECIMAL(20, 8) NOT NULL,
    
    -- Timing
    LAST_UPDATED TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    RESET_AT TIMESTAMP NOT NULL COMMENT 'When limit resets (from trader)',
    
    UNIQUE KEY uk_resource (TRADER_ID, EXCHANGE_ID, RESOURCE_TYPE),
    
    CONSTRAINT fk_trader_resource_trader 
        FOREIGN KEY (TRADER_ID) REFERENCES TRADER(ID) 
        ON DELETE CASCADE ON UPDATE CASCADE
);
```

**Key Points:**
- ✅ Stores **actual usage** from traders (not theoretical limits)
- ✅ `RESET_AT` provided by trader (they know timezone/DST)
- ✅ Trader can report multiple resource types
- ✅ Core uses for load balancing only

---

## Benefits

1. **Autonomy:** Traders manage their own limits (no coordination needed)
2. **Accuracy:** Real-time data from exchange headers
3. **Resilience:** If Core loses metrics, trader still respects limits
4. **Flexibility:** Different traders can have different limits (VIP levels, IPs)
5. **Simplicity:** Core doesn't need to know exchange limit rules

---

## Example Flow

**Scenario:** 3 traders, 1 task to execute

```
Core: "I have a BUY order task. Who's available?"

Trader-EU:  "800/1200 requests used (67%) - I can take it"
Trader-US:  "1150/1200 requests used (96%) - I'm busy"
Trader-Asia: "200/1200 requests used (17%) - I can take it"

Core: "Assigning to Trader-Asia (least loaded)"

[Task sent to Trader-Asia]

Trader-Asia: "Executing... done. New metrics: 205/1200"
```

If Trader-Asia was **actually** at limit (but metrics stale):
```
Trader-Asia: "Rejected - rate limit. Retry in 30s"
Core: "OK, trying Trader-EU instead"
```

---

## Implementation Phases

**Phase 1.3 (State Management):**
- [ ] Core receives metrics via WebSocket
- [ ] Store in `TRADER_EXCHANGE_RESOURCE`
- [ ] Basic metrics query API

**Phase 1.4 (Scheduler):**
- [ ] Scoring algorithm uses metrics
- [ ] Prefer traders with lower load
- [ ] Handle "rate_limit" rejections (fallback to other trader)

**Phase 2 (Monitoring):**
- [ ] Metrics visualization dashboard
- [ ] Alert if trader consistently near limit
- [ ] Historical metrics tracking

---

## Rejected Alternatives

### ❌ Alternative 1: Core manages all limits
- **Problem:** Core doesn't have trader IPs, can't predict rate limits
- **Problem:** Requires manual config updates when exchanges change limits

### ❌ Alternative 2: Trader requests permission before each API call
- **Problem:** Too much latency (round-trip to Core)
- **Problem:** Core becomes bottleneck

### ✅ **Chosen:** Trader autonomy + periodic metrics reporting
- **Benefit:** Low latency (trader decides locally)
- **Benefit:** Core gets visibility for optimization
- **Benefit:** Resilient (works even if Core unavailable)

---

## Future Considerations

**If needed, Core can implement soft limits:**
```sql
-- Optional: Core-side limits (in addition to trader autonomy)
CREATE TABLE CORE_RESOURCE_POLICY (
    EXCHANGE_ID INT,
  POLICY_TYPE ENUM('max_concurrent_tasks_per_trader', 'max_total_orders_per_second'),
    THRESHOLD DECIMAL(20, 8),
    ACTION ENUM('warn', 'throttle', 'block')
);
```

But even then, **trader always has final say** (they know real-time limit status).
