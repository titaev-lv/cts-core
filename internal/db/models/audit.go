package models

import (
	"database/sql"
	"time"
)

// ==============================================================================
// AUDIT_LOG MODEL
// ==============================================================================
//
// Назначение: Аудит всех критических действий в системе
//
// Зачем нужен audit log?
//   - Compliance: Регуляторы требуют логи всех операций (кто, когда, что изменил)
//   - Security: Расследование инцидентов (кто удалил трейдера? кто изменил конфиг?)
//   - Debugging: "Почему этот трейдер перестал работать?" → смотрим audit log
//   - Analytics: "Кто самый активный админ?"
//
// Архитектура (ВАЖНО!):
//   Phase 2: Primary storage будет JSON файлы (высокая производительность)
//   Phase 1: Пока только MySQL для простоты
//
//   Преимущества JSON файлов:
//     - Не нагружают основную БД
//     - Можно хранить неограниченно долго
//     - Легко архивировать (gzip, move to S3)
//
//   MySQL audit_log используется для:
//     - Быстрые запросы (последние N дней)
//     - Связь с USER.ID через foreign key
//     - Удобные индексы для поиска
//
// Что логируется?
//   - Критические операции:
//     * Создание/удаление/изменение трейдера
//     * Изменение конфигурации системы
//     * Удаление/деактивация пользователей
//     * Изменение мониторов (assign/unassign трейдера)
//
//   - НЕ логируются обычные операции:
//     * Чтение данных (SELECT)
//     * Heartbeat трейдеров
//     * Метрики rate limits
//     * Обычная торговля (она логируется в ARBITRAGE_* таблицах)
//
// Retention policy:
//   - MySQL: 180 дней (настраивается в SCHEDULER_TASKS.cleanup_audit_logs)
//   - JSON файлы (Phase 2): неограниченно (архивируем по годам)
//
// Настройка retention:
//   UPDATE SCHEDULER_TASKS
//   SET CONFIG = '{"retention_days": 180}'
//   WHERE TASK_NAME = 'cleanup_audit_logs';
//
// Data Flow:
//   1. Админ делает критическое действие (DELETE /traders/123)
//   2. API middleware записывает в audit log:
//      - UID: кто сделал (USER.ID)
//      - ACTION: что сделал (TRADER_DELETE)
//      - OLD_VALUE: {"status": "active", "name": "EU Trader 1"}
//      - NEW_VALUE: {"status": "decommissioned"}
//      - IP_ADDRESS: откуда (192.168.1.10)
//   3. Background job (каждые 10 минут) копирует в JSON файл
//   4. Cleanup job (раз в сутки) удаляет записи старше 90 дней

type AuditLog struct {
	// Primary Key
	// ID - уникальный ID записи (BIGINT для долговременного хранения)
	ID int64 `json:"id" db:"ID"`

	// Timestamp
	// TIMESTAMP - когда произошло действие
	// TIMESTAMP(6) = microseconds precision для точного порядка событий
	// Пример: 2026-01-30 15:30:45.123456
	Timestamp time.Time `json:"timestamp" db:"TIMESTAMP"`

	// Actor (кто выполнил действие)
	// UID - ID пользователя из таблицы USER
	// NULL = system action (например, cleanup job)
	// NOT NULL = человек сделал действие через UI/API
	UID sql.NullInt32 `json:"uid" db:"UID"`

	// Action Details
	// ACTION - что было сделано
	// Примеры:
	//   'TRADER_CREATE' - создан новый трейдер
	//   'TRADER_DELETE' - удален трейдер
	//   'TRADER_SUSPEND' - трейдер приостановлен
	//   'CONFIG_UPDATE' - обновлена конфигурация
	//   'MONITOR_ASSIGN' - трейдер назначен на монитор
	//   'USER_DELETE' - удален пользователь
	//   'KEY_ROTATION' - ротация HSM ключей
	Action string `json:"action" db:"ACTION"`

	// Resource Identification
	// RESOURCE_TYPE - тип ресурса
	// Примеры: 'trader', 'monitor', 'config', 'user', 'exchange_account'
	ResourceType sql.NullString `json:"resource_type" db:"RESOURCE_TYPE"`

	// RESOURCE_ID - ID или название ресурса
	// Примеры:
	//   '123' (TRADER.ID)
	//   'system.max_tasks' (config key)
	//   'eu-trader-1' (TRADER.CERTIFICATE_CN)
	ResourceID sql.NullString `json:"resource_id" db:"RESOURCE_ID"`

	// Change Tracking
	// OLD_VALUE - состояние ДО изменения (JSON)
	// Пример для TRADER_SUSPEND:
	//   {"status": "active", "max_tasks": 10}
	//
	// NEW_VALUE - состояние ПОСЛЕ изменения (JSON)
	// Пример для TRADER_SUSPEND:
	//   {"status": "suspended", "max_tasks": 0}
	//
	// Зачем JSON?
	//   - Гибкость: можем хранить любые данные
	//   - Легко читать/парсить
	//   - Можно делать JSON_EXTRACT для поиска
	OldValue sql.NullString `json:"old_value" db:"OLD_VALUE"` // JSON
	NewValue sql.NullString `json:"new_value" db:"NEW_VALUE"` // JSON

	// Request Metadata
	// IP_ADDRESS - IP адрес клиента
	// Поддержка IPv4 (15 chars) и IPv6 (45 chars)
	// Примеры:
	//   '192.168.1.10' (IPv4)
	//   '2001:0db8:85a3:0000:0000:8a2e:0370:7334' (IPv6)
	IPAddress sql.NullString `json:"ip_address" db:"IP_ADDRESS"`

	// USER_AGENT - браузер/клиент
	// Пример: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0'
	// Нужно для:
	//   - Понять откуда пришел запрос (UI/API/script)
	//   - Security analysis (подозрительные user agents)
	UserAgent sql.NullString `json:"user_agent" db:"USER_AGENT"`

	// Result
	// SUCCESS - успешно ли выполнено действие?
	// TRUE = успех, FALSE = ошибка
	// Даже failed операции логируются!
	Success bool `json:"success" db:"SUCCESS"`

	// ERROR_MESSAGE - текст ошибки если SUCCESS=FALSE
	// Пример: 'Trader eu-1 not found'
	// NULL если SUCCESS=TRUE
	ErrorMessage sql.NullString `json:"error_message" db:"ERROR_MESSAGE"`
}

// AuditAction - типы действий для audit log
type AuditAction string

const (
	// Trader Operations
	AuditActionTraderCreate  AuditAction = "TRADER_CREATE"
	AuditActionTraderDelete  AuditAction = "TRADER_DELETE"
	AuditActionTraderSuspend AuditAction = "TRADER_SUSPEND"
	AuditActionTraderResume  AuditAction = "TRADER_RESUME"
	AuditActionTraderUpdate  AuditAction = "TRADER_UPDATE"

	// Monitor Operations
	AuditActionMonitorAssign   AuditAction = "MONITOR_ASSIGN"
	AuditActionMonitorUnassign AuditAction = "MONITOR_UNASSIGN"
	AuditActionMonitorPause    AuditAction = "MONITOR_PAUSE"
	AuditActionMonitorResume   AuditAction = "MONITOR_RESUME"

	// User Operations
	AuditActionUserCreate AuditAction = "USER_CREATE"
	AuditActionUserDelete AuditAction = "USER_DELETE"
	AuditActionUserUpdate AuditAction = "USER_UPDATE"
	AuditActionUserLogin  AuditAction = "USER_LOGIN"
	AuditActionUserLogout AuditAction = "USER_LOGOUT"

	// User Group Operations (www-go)
	AuditActionUserGroupCreate   AuditAction = "USER_GROUP_CREATE"
	AuditActionUserGroupUpdate   AuditAction = "USER_GROUP_UPDATE"
	AuditActionUserGroupDelete   AuditAction = "USER_GROUP_DELETE"
	AuditActionUserGroupAssign   AuditAction = "USER_GROUP_ASSIGN"
	AuditActionUserGroupUnassign AuditAction = "USER_GROUP_UNASSIGN"

	// Config Operations
	AuditActionConfigUpdate AuditAction = "CONFIG_UPDATE"

	// HSM Operations
	AuditActionKeyRotation AuditAction = "KEY_ROTATION"
	AuditActionReencrypt   AuditAction = "REENCRYPTION_JOB"

	// Exchange Account Operations
	AuditActionExchangeAccountCreate AuditAction = "EXCHANGE_ACCOUNT_CREATE"
	AuditActionExchangeAccountDelete AuditAction = "EXCHANGE_ACCOUNT_DELETE"
	AuditActionExchangeAccountUpdate AuditAction = "EXCHANGE_ACCOUNT_UPDATE"
)

// ResourceType - типы ресурсов
type ResourceType string

const (
	ResourceTypeTrader          ResourceType = "trader"
	ResourceTypeMonitor         ResourceType = "monitor"
	ResourceTypeUser            ResourceType = "user"
	ResourceTypeUserGroup       ResourceType = "user_group"
	ResourceTypeUserGroupMember ResourceType = "user_group_member"
	ResourceTypeConfig          ResourceType = "config"
	ResourceTypeExchangeAccount ResourceType = "exchange_account"
	ResourceTypeExchange        ResourceType = "exchange"
	ResourceTypeHSM             ResourceType = "hsm"
	ResourceTypeScheduler       ResourceType = "scheduler"
)
