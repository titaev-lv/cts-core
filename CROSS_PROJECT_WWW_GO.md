# Cross-Project Integration: cts-core ↔ www-go

**Последнее обновление:** 2026-01-30  
**Статус Phase 1.2:** Models созданы в cts-core, требуется Admin UI в www-go

---

## 🎯 Архитектурное разделение

### cts-core (Runtime Engine)
- **Роль:** Управление трейдерами в реальном времени
- **Операции:** READ конфиг, WRITE логи/сессии, UPDATE статусы
- **Язык:** Go
- **Расположение:** `/home/dev/docker/cts-core`

### www-go (Admin Panel)
- **Роль:** Административное управление конфигурацией
- **Операции:** CRUD для всех конфигурационных таблиц
- **Язык:** PHP + Go backend
- **Расположение:** `/home/dev/docker/cts-core/other-sub-system/www-go`

---

## 📋 TODO для www-go (Admin UI)

### ✅ Уже реализовано в www-go:
- [x] CRUD для USER
- [x] CRUD для EXCHANGE
- [x] CRUD для EXCHANGE_ACCOUNTS
- [x] Просмотр MONITORING

### 🔴 ТРЕБУЕТСЯ реализовать для Phase 1.2:

#### 1. **TRADER Management** (Приоритет: HIGH)

**Локация в www-go:** `www-go/internal/controllers/trader_controller.go` (создать)

**CRUD операции:**

```go
// CREATE - Регистрация нового трейдера
POST /api/traders
Body: {
    "trader_name": "EU Frankfurt Trader",
    "certificate_cn": "trader-eu-1.cts.internal",  // UNIQUE! Должен совпадать с CN в сертификате
    "region": "eu",                                 // null, "eu", "us", "asia"
    "status": "registered",                         // всегда "registered" при создании
    "max_tasks": 10,                                // по умолчанию 10
    "notes": "Running on dedicated server 10.0.1.5"
}

// READ - Список трейдеров
GET /api/traders
Query params: ?status=active&region=eu
Response: [
    {
        "id": 1,
        "trader_name": "EU Frankfurt Trader",
        "certificate_cn": "trader-eu-1.cts.internal",
        "region": "eu",
        "status": "active",        // registered | active | suspended | decommissioned
        "max_tasks": 10,
        "date_create": "2026-01-30T10:00:00Z",
        "date_modify": "2026-01-30T15:30:00Z",
        "notes": "..."
    }
]

// UPDATE - Редактирование трейдера
PUT /api/traders/{id}
Body: {
    "trader_name": "EU Frankfurt Trader (Updated)",
    "region": "eu",
    "max_tasks": 15,              // можно изменить
    "status": "suspended",         // можно приостановить
    "notes": "Increased capacity"
}

// DELETE - Деактивация трейдера (soft delete)
DELETE /api/traders/{id}
Action: UPDATE TRADER SET STATUS='decommissioned' WHERE ID={id}
```

**Валидация (важно!):**
- `certificate_cn` - UNIQUE, формат: `trader-{region}-{number}.cts.internal`
- `status` - только: registered, active, suspended, decommissioned
- `max_tasks` - минимум 1, максимум 100
- При DELETE проверить: нет активных сессий (TRADER_SESSION.ENDED_AT IS NULL)

**UI requirements:**
- Таблица со всеми трейдерами
- Фильтры: по статусу, региону
- Кнопки: Create, Edit, Suspend, Decommission
- Показывать: сколько активных сессий у трейдера
- Предупреждение: "Трейдер имеет N активных сессий" при попытке удаления

**Ссылка на модель cts-core:** `internal/db/models/trader.go:30-81` (struct Trader)

---

#### 2. **TRADER_SESSION Monitoring** (Приоритет: MEDIUM)

**Локация в www-go:** `www-go/internal/controllers/trader_session_controller.go` (создать)

**READ-ONLY операции** (cts-core создает сессии):

```go
// READ - История сессий трейдера
GET /api/traders/{trader_id}/sessions
Response: [
    {
        "id": 1234,
        "trader_id": 1,
        "session_id": "550e8400-e29b-41d4-a716-446655440000",
        "ip_address": "192.168.1.10",
        "connected_at": "2026-01-30T10:00:00Z",
        "last_heartbeat": "2026-01-30T15:29:45Z",
        "ended_at": null,                         // null = активная сессия
        "disconnect_reason": null,
        "error_message": null
    }
]

// READ - Активные сессии (все трейдеры)
GET /api/sessions/active
Response: [...] // где ended_at IS NULL

// ADMIN ACTION - Принудительное отключение (kick)
POST /api/sessions/{session_id}/kick
Body: {
    "reason": "Maintenance required"
}
Action: 
  1. UPDATE TRADER_SESSION SET ended_at=NOW(), disconnect_reason='kicked', error_message='{reason}'
  2. Отправить WebSocket команду в cts-core для закрытия соединения
```

**UI requirements:**
- Показывать в карточке трейдера: список активных сессий
- Индикатор "мертвых" сессий: last_heartbeat < NOW() - 2 минуты
- Кнопка "Kick session" для админов
- История: фильтр по дате, причине отключения

**Ссылка на модель cts-core:** `internal/db/models/trader.go:118-166` (struct TraderSession)

---

#### 3. **TRADER_EXCHANGE_RESOURCE Monitoring** (Приоритет: LOW)

**Локация в www-go:** `www-go/internal/controllers/trader_resource_controller.go` (создать)

**READ-ONLY** (cts-core пишет метрики):

```go
// READ - Метрики трейдера по биржам
GET /api/traders/{trader_id}/resources
Response: [
    {
        "id": 1,
        "trader_id": 1,
        "exchange_id": 1,              // Binance
        "exchange_account_id": null,   // null = IP-level limit
        "resource_type": "api_requests_minute",
        "current_value": 850,
        "max_value": 1200,
        "usage_percent": 70.8,         // рассчитывается
        "reset_at": "2026-01-30T15:31:00Z",
        "last_update": "2026-01-30T15:30:45Z"
    }
]

// READ - Dashboard: нагрузка на биржи
GET /api/dashboard/exchange-load
Response: {
    "binance": {
        "total_traders": 3,
        "avg_load": 65.2,              // средняя нагрузка в %
        "peak_load": 89.5,             // максимальная
        "overloaded_traders": []       // trader_id с нагрузкой >90%
    }
}
```

**UI requirements:**
- Dashboard: тепловая карта нагрузки (трейдер × биржа)
- Предупреждения: если current_value > max_value * 0.9
- График: динамика нагрузки во времени (last 1 hour)

**Ссылка на модель cts-core:** `internal/db/models/trader_resource.go:46-87`

---

#### 4. **ARBITRAGE_ORDER Monitoring** (Приоритет: MEDIUM)

**Локация в www-go:** Уже есть в `www-go/php/arbitrage/` ?

**Проверить:**
- [ ] Отображается ли `TRADER_ID` в списке ордеров?
- [ ] Можно ли фильтровать ордера по трейдеру?
- [ ] Показывается ли статистика: какой трейдер исполнил больше ордеров?

**Если НЕТ - добавить:**
```php
// www-go/php/arbitrage/orders.php
SELECT 
    ao.*,
    t.TRADER_NAME,
    t.REGION
FROM ARBITRAGE_ORDER ao
LEFT JOIN TRADER t ON ao.TRADER_ID = t.ID
WHERE ao.STATUS IN ('filled', 'partial')
ORDER BY ao.DATE_CREATE DESC
```

**Ссылка на модель cts-core:** `internal/db/models/arbitrage.go:50-134`

---

#### 5. **AUDIT_LOG Viewer** (Приоритет: HIGH)

**Локация в www-go:** `www-go/internal/controllers/audit_controller.go` (создать)

**READ-ONLY** (cts-core пишет):

```go
// READ - Audit log
GET /api/audit
Query: ?action=TRADER_DELETE&uid=5&from=2026-01-01&to=2026-01-31
Response: [
    {
        "id": 1234,
        "timestamp": "2026-01-30T15:30:00.123456Z",
        "uid": 5,
        "user_name": "admin@example.com",        // JOIN с USER
        "action": "TRADER_DELETE",
        "resource_type": "trader",
        "resource_id": "123",
        "old_value": {"status": "active", "max_tasks": 10},
        "new_value": {"status": "decommissioned"},
        "ip_address": "192.168.1.5",
        "success": true,
        "error_message": null
    }
]
```

**UI requirements:**
- Таблица с фильтрами: action, user, date range, success/error
- Diff view: показать изменения old_value → new_value
- Export в CSV для compliance
- Retention warning: "Записи старше 180 дней (6 месяцев) удаляются автоматически (настраивается в SCHEDULER_TASKS)"

**Ссылка на модель cts-core:** `internal/db/models/audit.go:68-120`

**⚠️ ВАЖНО: www-go ДОЛЖЕН писать в AUDIT_LOG!**

При любых операциях CRUD в www-go необходимо записывать в `AUDIT_LOG`:

```php
// www-go: После успешного CREATE/UPDATE/DELETE
$auditData = [
    'uid' => $_SESSION['user_id'],           // кто сделал
    'action' => 'TRADER_CREATE',             // что сделал
    'resource_type' => 'trader',
    'resource_id' => (string)$traderId,
    'old_value' => null,                     // для CREATE
    'new_value' => json_encode($traderData), // новые данные
    'ip_address' => $_SERVER['REMOTE_ADDR'],
    'user_agent' => $_SERVER['HTTP_USER_AGENT'],
    'success' => true
];

// INSERT INTO AUDIT_LOG (timestamp, uid, action, ...) VALUES (...)
$db->insert('AUDIT_LOG', $auditData);
```

**Все операции требующие аудита в www-go:**

| Операция | ACTION | RESOURCE_TYPE | OLD_VALUE | NEW_VALUE |
|----------|--------|---------------|-----------|-----------|
| Создать трейдера | `TRADER_CREATE` | trader | null | {все поля} |
| Изменить трейдера | `TRADER_UPDATE` | trader | {старые поля} | {новые поля} |
| Удалить трейдера | `TRADER_DELETE` | trader | {все поля} | null |
| Приостановить | `TRADER_SUSPEND` | trader | {status: active} | {status: suspended} |
| Возобновить | `TRADER_RESUME` | trader | {status: suspended} | {status: active} |
| Создать пользователя | `USER_CREATE` | user | null | {login, role, ...} |
| Изменить пользователя | `USER_UPDATE` | user | {старые поля} | {новые поля} |
| Удалить пользователя | `USER_DELETE` | user | {все поля} | null |
| Создать биржу | `EXCHANGE_CREATE` | exchange | null | {name, code, ...} |
| Изменить биржу | `EXCHANGE_UPDATE` | exchange | {старые поля} | {новые поля} |
| Удалить биржу | `EXCHANGE_DELETE` | exchange | {все поля} | null |
| Создать аккаунт | `EXCHANGE_ACCOUNT_CREATE` | exchange_account | null | {name, exchange_id, ...} |
| Изменить аккаунт | `EXCHANGE_ACCOUNT_UPDATE` | exchange_account | {старые поля} | {новые поля} |
| Удалить аккаунт | `EXCHANGE_ACCOUNT_DELETE` | exchange_account | {все поля} | null |
| Создать группу | `USER_GROUP_CREATE` | user_group | null | {name, permissions} |
| Изменить группу | `USER_GROUP_UPDATE` | user_group | {старые поля} | {новые поля} |
| Удалить группу | `USER_GROUP_DELETE` | user_group | {все поля} | null |
| Добавить в группу | `USER_GROUP_ASSIGN` | user_group_member | null | {user_id, group_id} |
| Убрать из группы | `USER_GROUP_UNASSIGN` | user_group_member | {user_id, group_id} | null |
| Kick трейдера | `TRADER_KICK` | trader_session | {session_id} | {reason} |

**Пример реализации в www-go:**

```php
// www-go/internal/services/audit_service.php
class AuditService {
    private $db;
    
    public function log(array $params): void {
        $data = [
            'timestamp' => microtime(true),  // TIMESTAMP(6) с микросекундами
            'uid' => $params['uid'] ?? $_SESSION['user_id'] ?? null,
            'action' => $params['action'],
            'resource_type' => $params['resource_type'] ?? null,
            'resource_id' => $params['resource_id'] ?? null,
            'old_value' => isset($params['old_value']) ? json_encode($params['old_value']) : null,
            'new_value' => isset($params['new_value']) ? json_encode($params['new_value']) : null,
            'ip_address' => $_SERVER['REMOTE_ADDR'] ?? null,
            'user_agent' => $_SERVER['HTTP_USER_AGENT'] ?? null,
            'success' => $params['success'] ?? true,
            'error_message' => $params['error_message'] ?? null
        ];
        
        $this->db->insert('AUDIT_LOG', $data);
    }
}

// Использование в контроллере
// www-go/internal/controllers/trader_controller.php
class TraderController {
    public function create(Request $request): Response {
        $audit = new AuditService($this->db);
        
        try {
            $traderData = $request->validated();
            $traderId = $this->traderRepo->create($traderData);
            
            // Логируем успех
            $audit->log([
                'action' => 'TRADER_CREATE',
                'resource_type' => 'trader',
                'resource_id' => (string)$traderId,
                'new_value' => $traderData,
                'success' => true
            ]);
            
            return response()->json(['id' => $traderId], 201);
            
        } catch (Exception $e) {
            // Логируем ошибку
            $audit->log([
                'action' => 'TRADER_CREATE',
                'resource_type' => 'trader',
                'new_value' => $traderData,
                'success' => false,
                'error_message' => $e->getMessage()
            ]);
            
            throw $e;
        }
    }
    
    public function update(int $id, Request $request): Response {
        $audit = new AuditService($this->db);
        
        // Сохраняем старое состояние ДО изменения
        $oldData = $this->traderRepo->getById($id);
        $newData = $request->validated();
        
        try {
            $this->traderRepo->update($id, $newData);
            
            // Логируем с old_value и new_value
            $audit->log([
                'action' => 'TRADER_UPDATE',
                'resource_type' => 'trader',
                'resource_id' => (string)$id,
                'old_value' => $oldData,  // ВАЖНО: сохранить ДО изменения
                'new_value' => $newData,
                'success' => true
            ]);
            
            return response()->json(['success' => true]);
            
        } catch (Exception $e) {
            $audit->log([
                'action' => 'TRADER_UPDATE',
                'resource_type' => 'trader',
                'resource_id' => (string)$id,
                'old_value' => $oldData,
                'new_value' => $newData,
                'success' => false,
                'error_message' => $e->getMessage()
            ]);
            
            throw $e;
        }
    }
}
```

**Добавить в www-go модели:**

```php
// www-go/internal/models/audit_log.php
class AuditLog {
    public const ACTION_USER_CREATE = 'USER_CREATE';
    public const ACTION_USER_UPDATE = 'USER_UPDATE';
    public const ACTION_USER_DELETE = 'USER_DELETE';
    public const ACTION_USER_LOGIN = 'USER_LOGIN';
    public const ACTION_USER_LOGOUT = 'USER_LOGOUT';
    
    public const ACTION_TRADER_CREATE = 'TRADER_CREATE';
    public const ACTION_TRADER_UPDATE = 'TRADER_UPDATE';
    public const ACTION_TRADER_DELETE = 'TRADER_DELETE';
    public const ACTION_TRADER_SUSPEND = 'TRADER_SUSPEND';
    public const ACTION_TRADER_RESUME = 'TRADER_RESUME';
    public const ACTION_TRADER_KICK = 'TRADER_KICK';
    
    public const ACTION_EXCHANGE_CREATE = 'EXCHANGE_CREATE';
    public const ACTION_EXCHANGE_UPDATE = 'EXCHANGE_UPDATE';
    public const ACTION_EXCHANGE_DELETE = 'EXCHANGE_DELETE';
    
    public const ACTION_EXCHANGE_ACCOUNT_CREATE = 'EXCHANGE_ACCOUNT_CREATE';
    public const ACTION_EXCHANGE_ACCOUNT_UPDATE = 'EXCHANGE_ACCOUNT_UPDATE';
    public const ACTION_EXCHANGE_ACCOUNT_DELETE = 'EXCHANGE_ACCOUNT_DELETE';
    
    public const ACTION_USER_GROUP_CREATE = 'USER_GROUP_CREATE';
    public const ACTION_USER_GROUP_UPDATE = 'USER_GROUP_UPDATE';
    public const ACTION_USER_GROUP_DELETE = 'USER_GROUP_DELETE';
    public const ACTION_USER_GROUP_ASSIGN = 'USER_GROUP_ASSIGN';
    public const ACTION_USER_GROUP_UNASSIGN = 'USER_GROUP_UNASSIGN';
    
    public const ACTION_CONFIG_UPDATE = 'CONFIG_UPDATE';
    public const ACTION_KEY_ROTATION = 'KEY_ROTATION';
    
    public const RESOURCE_TYPE_USER = 'user';
    public const RESOURCE_TYPE_TRADER = 'trader';
    public const RESOURCE_TYPE_EXCHANGE = 'exchange';
    public const RESOURCE_TYPE_EXCHANGE_ACCOUNT = 'exchange_account';
    public const RESOURCE_TYPE_USER_GROUP = 'user_group';
    public const RESOURCE_TYPE_USER_GROUP_MEMBER = 'user_group_member';
    public const RESOURCE_TYPE_CONFIG = 'config';
    public const RESOURCE_TYPE_HSM = 'hsm';
}
```

**Синхронизация констант между проектами:**

Константы ACTION и RESOURCE_TYPE должны совпадать с `internal/db/models/audit.go`:
- ✅ cts-core: `AuditActionTraderCreate` = "TRADER_CREATE"
- ✅ www-go: `AuditLog::ACTION_TRADER_CREATE` = "TRADER_CREATE"

**UI для Audit Log в www-go:**

Добавить фильтр по всем типам операций:
```html
<!-- www-go/templates/audit/list.php -->
<select name="action">
    <option value="">All Actions</option>
    <optgroup label="User Operations">
        <option value="USER_CREATE">User Create</option>
        <option value="USER_UPDATE">User Update</option>
        <option value="USER_DELETE">User Delete</option>
        <option value="USER_LOGIN">User Login</option>
    </optgroup>
    <optgroup label="Trader Operations">
        <option value="TRADER_CREATE">Trader Create</option>
        <option value="TRADER_UPDATE">Trader Update</option>
        <option value="TRADER_DELETE">Trader Delete</option>
        <option value="TRADER_SUSPEND">Trader Suspend</option>
        <option value="TRADER_KICK">Trader Kick</option>
    </optgroup>
    <optgroup label="Exchange Operations">
        <option value="EXCHANGE_CREATE">Exchange Create</option>
        <option value="EXCHANGE_UPDATE">Exchange Update</option>
        <option value="EXCHANGE_DELETE">Exchange Delete</option>
    </optgroup>
    <optgroup label="Exchange Account Operations">
        <option value="EXCHANGE_ACCOUNT_CREATE">Account Create</option>
        <option value="EXCHANGE_ACCOUNT_UPDATE">Account Update</option>
        <option value="EXCHANGE_ACCOUNT_DELETE">Account Delete</option>
    </optgroup>
    <optgroup label="Group Operations">
        <option value="USER_GROUP_CREATE">Group Create</option>
        <option value="USER_GROUP_DELETE">Group Delete</option>
        <option value="USER_GROUP_ASSIGN">Assign to Group</option>
        <option value="USER_GROUP_UNASSIGN">Remove from Group</option>
    </optgroup>
</select>
```

---

#### 6. **SCHEDULER_TASKS Management** (Приоритет: LOW)

**Локация в www-go:** `www-go/internal/controllers/scheduler_controller.go` (создать)

**CRUD операции:**

```go
// READ - Список задач
GET /api/scheduler/tasks
Response: [
    {
        "id": 1,
        "task_name": "cleanup_trader_sessions",
        "task_type": "cleanup",
        "schedule_cron": "0 2 * * *",          // 2:00 AM daily
        "enabled": true,
        "status": "idle",                       // idle | running | failed
        "last_run_at": "2026-01-30T02:00:00Z",
        "last_run_duration_ms": 1234,
        "last_run_status": "success",
        "next_run_at": "2026-01-31T02:00:00Z",
        "run_count": 45,
        "error_count": 0
    }
]

// UPDATE - Включить/выключить задачу
PUT /api/scheduler/tasks/{id}/toggle
Body: {"enabled": false}

// UPDATE - Изменить расписание (осторожно!)
PUT /api/scheduler/tasks/{id}/schedule
Body: {
    "schedule_cron": "0 3 * * *"  // изменить на 3:00 AM
}
```

**UI requirements:**
- Список всех задач с цветовой индикацией статуса
- Кнопка Enable/Disable
- Warning: "Task is running" при попытке выключить
- Показывать: next run in X minutes/hours
- История запусков: last 10 runs

**Ссылка на модель cts-core:** `internal/db/models/scheduler.go:63-156`

---

## 🔄 Синхронизация моделей данных

### Дополнения в audit.go (для www-go операций):

Нужно добавить в `internal/db/models/audit.go` константы для www-go операций:

```go
// User Group Operations (для www-go)
AuditActionUserGroupCreate   AuditAction = "USER_GROUP_CREATE"
AuditActionUserGroupUpdate   AuditAction = "USER_GROUP_UPDATE"
AuditActionUserGroupDelete   AuditAction = "USER_GROUP_DELETE"
AuditActionUserGroupAssign   AuditAction = "USER_GROUP_ASSIGN"
AuditActionUserGroupUnassign AuditAction = "USER_GROUP_UNASSIGN"

// Resource Types
ResourceTypeUserGroup       ResourceType = "user_group"
ResourceTypeUserGroupMember ResourceType = "user_group_member"
```

### Обязательные поля (не менять в www-go):

```sql
-- TRADER
CERTIFICATE_CN VARCHAR(255) UNIQUE NOT NULL  -- cts-core использует для mTLS auth
STATUS ENUM(...)                              -- cts-core обновляет на 'active'
MAX_TASKS INT NOT NULL                        -- cts-core читает для распределения

-- TRADER_SESSION
SESSION_ID VARCHAR(100) UNIQUE NOT NULL       -- cts-core генерирует UUID
LAST_HEARTBEAT TIMESTAMP                      -- cts-core обновляет каждые 30 сек
ENDED_AT TIMESTAMP NULL                       -- cts-core закрывает сессию

-- TRADER_EXCHANGE_RESOURCE
CURRENT_VALUE INT                             -- cts-core обновляет в реальном времени
RESET_AT TIMESTAMP                            -- cts-core рассчитывает
```

### Enum синхронизация:

**TraderStatus:**
- `registered` - создан в www-go, трейдер еще не подключался
- `active` - cts-core установил при успешном подключении
- `suspended` - www-go приостановил (трейдер не сможет подключиться)
- `decommissioned` - www-go удалил (soft delete)

**DisconnectReason:**
- `graceful` - трейдер корректно завершился
- `timeout` - нет heartbeat >2 минут
- `error` - ошибка в cts-core или трейдере
- `server_shutdown` - cts-core перезагружается
- `kicked` - админ из www-go принудительно отключил

**TaskStatus:**
- `idle` - ожидает запуска
- `running` - выполняется прямо сейчас
- `failed` - последний запуск с ошибкой
- `disabled` - выключена в www-go

---

## 📡 API между cts-core и www-go (Phase 2)

### Scenario 1: Админ принудительно отключает трейдера

**www-go → cts-core:**
```http
POST http://cts-core:8080/api/internal/traders/{trader_id}/disconnect
Authorization: Bearer {internal_token}
Body: {
    "reason": "Maintenance required",
    "grace_period_sec": 30  // дать трейдеру 30 сек на graceful shutdown
}
```

**cts-core:**
1. Находит активные WebSocket сессии трейдера
2. Отправляет WS message: `{"type": "shutdown", "grace_period": 30}`
3. Через 30 сек закрывает соединение принудительно
4. Записывает в AUDIT_LOG: ACTION='TRADER_KICKED'

### Scenario 2: cts-core уведомляет www-go о событиях (optional)

**cts-core → www-go webhook:**
```http
POST http://www-go/api/webhooks/cts-events
Body: {
    "event_type": "trader_connected",
    "timestamp": "2026-01-30T15:30:00Z",
    "data": {
        "trader_id": 1,
        "session_id": "550e8400-...",
        "ip_address": "192.168.1.10"
    }
}
```

---

## 📦 Миграции БД

### www-go должен применить:

Если www-go еще не применил миграцию `001_phase1_schema.sql`:

```bash
cd /home/dev/docker/cts-core
mysql -h 192.168.50.5 -u titaev -p ct_system < migrations/001_phase1_schema.sql
```

После применения будут созданы таблицы:
- TRADER
- TRADcts-core: Добавить константы для www-go операций в audit.go
- [ ] www-go: Реализовать AuditService для логирования всех операций
- [ ] www-go: TRADER CRUD UI + audit logging
- [ ] www-go: USER CRUD + audit logging
- [ ] www-go: EXCHANGE CRUD + audit logging
- [ ] www-go: EXCHANGE_ACCOUNT CRUD + audit logging
- [ ] www-go: USER_GROUP management + audit logging
- [ ] www-go: TRADER_SESSION monitoring UI
- [ ] www-go: AUDIT_LOG viewer UI (с фильтрами по всем операциям)
- [ ] www-go: SCHEDULER_TASKS management UI
- [ ] Тестирование: создать/изменить/удалить объекты → проверить записи в AUDIT_LOG
- AUDIT_LOG
- REENCRYPTION_JOBS
- REENCRYPTION_PROGRESS
- SCHEDULER_TASKS

---

## ✅ Чеклист интеграции

### Phase 1.2 (текущая):

- [x] cts-core: Модели созданы (`internal/db/models/*.go`)
- [x] cts-core: MySQL подключение с mTLS работает
- [ ] www-go: TRADER CRUD UI
- [ ] www-go: TRADER_SESSION monitoring UI
- [ ] www-go: AUDIT_LOG viewer UI
- [ ] www-go: SCHEDULER_TASKS management UI
- [ ] Тестирование: создать трейдера в www-go → запустить cts-core → проверить подключение

### Phase 1.3 (HSM):

- [ ] cts-core: HSM client для decrypt API keys
- [ ] www-go: Key rotation UI (создать REENCRYPTION_JOBS)

### Phase 1.4 (WebSocket):

- [ ] cts-core: WebSocket server для трейдеров
- [ ] www-go: Live monitoring dashboard (активные сессии в реальном времени)
- [ ] API: www-go → cts-core (kick trader)

---

## 🔧 Полезные команды

### Проверить структуру таблиц:
```sql
SHOW CREATE TABLE TRADER;
SHOW CREATE TABLE TRADER_SESSION;
SELECT * FROM TRADER LIMIT 1;
```

### Проверить активные сессии:
```sql
SELECT 
    t.TRADER_NAME,
    ts.SESSION_ID,
    ts.IP_ADDRESS,
    ts.CONNECTED_AT,
    TIMESTAMPDIFF(SECOND, ts.LAST_HEARTBEAT, NOW()) AS heartbeat_lag_sec
FROM TRADER_SESSION ts
JOIN TRADER t ON ts.TRADER_ID = t.ID
WHERE ts.ENDED_AT IS NULL;
```

### Проверить нагрузку на биржи:
```sql
SELECT 
    t.TRADER_NAME,
    e.NAME AS exchange,
    ter.RESOURCE_TYPE,
    ter.CURRENT_VALUE,
    ter.MAX_VALUE,
    ROUND(ter.CURRENT_VALUE * 100.0 / ter.MAX_VALUE, 1) AS usage_percent
FROM TRADER_EXCHANGE_RESOURCE ter
JOIN TRADER t ON ter.TRADER_ID = t.ID
JOIN EXCHANGE e ON ter.EXCHANGE_ID = e.ID
ORDER BY usage_percent DESC;
```

---

## 📞 Контакты для синхронизации

**Вопросы по cts-core:** @cts-core-team  
**Вопросы по www-go:** @www-go-team  
**Обсуждение архитектуры:** #architecture-discussions  

**Этот файл обновляется:** После завершения каждой Phase  
**Следующий update:** Phase 1.3 (HSM Integration)
