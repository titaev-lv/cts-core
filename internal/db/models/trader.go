package models

import (
	"database/sql"
	"time"
)

// ==============================================================================
// TRADER MODEL
// ==============================================================================
//
// Назначение: Регистрация трейдер-демонов в системе
//
// Lifecycle:
//   1. При первом mTLS подключении создается запись (STATUS='pending')
//   2. Трейдер подключается по mTLS (проверка CERTIFICATE_CN)
//   3. После ручного approve статус переводится в 'active'
//   4. При отключении может стать 'suspended' или 'decommissioned'
//
// Архитектура:
//   - Identity берется только из CN клиентского сертификата
//   - Запись трейдера создается автоматически при первом подключении
//   - Каждый трейдер имеет уникальный CN в mTLS сертификате
//   - Трейдеры самостоятельно управляют rate limits своих IP
//   - CTS-Core использует эту таблицу для:
//     * Аутентификации по mTLS (проверка CN)
//     * Распределения задач (MAX_TASKS - лимит параллельных задач)
//     * Мониторинга статуса (active/suspended)

type Trader struct {
	// Primary Key
	ID int `json:"id" db:"ID"`

	// Identification
	// TRADER_NAME - человекочитаемое имя для UI и логов
	// Пример: "EU Frankfurt Trader", "US East Trader"
	TraderName string `json:"trader_name" db:"TRADER_NAME"`

	// CERTIFICATE_CN - Subject CN из mTLS сертификата (UNIQUE!)
	// Используется для аутентификации при WebSocket подключении
	// Пример: "trader-eu-1.cts.internal"
	// ВАЖНО: должно совпадать с CN в сертификате pki/client/trader-*.crt
	CertificateCN string `json:"certificate_cn" db:"CERTIFICATE_CN"`

	// Configuration
	// REGION - географический регион (для выбора ближайших бирж)
	// Примеры: "eu" (Europe), "us" (US), "asia" (Asia-Pacific)
	// Может быть NULL, если регион не важен
	Region sql.NullString `json:"region,omitempty" db:"REGION"`

	// STATUS - текущее состояние трейдера
	//   'pending' - создан автоматически, подключение разрешено, но задачи не выдаются
	//   'active' - подключен и работает
	//   'suspended' - временно приостановлен (админом или из-за ошибок)
	//   'decommissioned' - выведен из эксплуатации
	Status TraderStatus `json:"status" db:"STATUS"`

	// Capacity Management
	// MAX_TASKS - максимум параллельных задач для этого трейдера
	// По умолчанию 10. Ограничивает нагрузку на трейдер-демон.
	// Scheduler учитывает это значение при распределении задач
	MaxTasks int `json:"max_tasks" db:"MAX_TASKS"`

	// Audit Trail
	// DATE_CREATE - когда запись создана
	DateCreate time.Time `json:"date_create" db:"DATE_CREATE"`

	// DATE_MODIFY - последнее изменение (авто-обновляется MySQL)
	DateModify time.Time `json:"date_modify" db:"DATE_MODIFY"`

	// USER_CREATED - кто создал запись (ссылка на USER.ID)
	// Может быть NULL, если создано автоматически
	UserCreated sql.NullInt32 `json:"user_created,omitempty" db:"USER_CREATED"`

	// USER_MODIFY - кто последний раз изменил запись
	UserModify sql.NullInt32 `json:"user_modify,omitempty" db:"USER_MODIFY"`

	// NOTES - произвольные заметки админа
	// Пример: "Running on dedicated server 10.0.1.5"
	Notes sql.NullString `json:"notes,omitempty" db:"NOTES"`
}

// TraderStatus - enum для статуса трейдера
type TraderStatus string

const (
	TraderStatusPending        TraderStatus = "pending"        // Создан автоматически, ожидает активации
	TraderStatusActive         TraderStatus = "active"         // Подключен и работает
	TraderStatusSuspended      TraderStatus = "suspended"      // Приостановлен
	TraderStatusDecommissioned TraderStatus = "decommissioned" // Выведен из эксплуатации
)

// ==============================================================================
// TRADER_SESSION MODEL
// ==============================================================================
//
// Назначение: История подключений трейдеров к CTS-Core
//
// Использование:
//   - Аудит подключений (кто, когда, откуда)
//   - Troubleshooting (почему отключился, ошибки)
//   - Мониторинг активных сессий
//
// Retention: 7 дней (автоматическая очистка scheduler задачей)
//
// Архитектура:
//   - Одна запись = одна WebSocket сессия
//   - ENDED_AT = NULL означает "сессия активна"
//   - LAST_HEARTBEAT обновляется каждые ~30 секунд
//   - Если LAST_HEARTBEAT протух (нет обновлений >2 минут), сессия "мертва"

type TraderSession struct {
	// Primary Key
	ID int64 `json:"id" db:"ID"`

	// Relations
	// TRADER_ID - кто подключился (ссылка на TRADER.ID)
	TraderID int `json:"trader_id" db:"TRADER_ID"`

	// Session Identity
	// SESSION_ID - UUID этой сессии (генерируется при подключении)
	// Используется для различения нескольких сессий одного трейдера
	SessionID string `json:"session_id" db:"SESSION_ID"`

	// WS_CONNECTION_ID - ID WebSocket соединения (из WebSocket библиотеки)
	// Может быть пустым, если не используется
	WSConnectionID sql.NullString `json:"ws_connection_id,omitempty" db:"WS_CONNECTION_ID"`

	// Network Info
	// IP_ADDRESS - IP адрес клиента (IPv4 или IPv6)
	// Используется для troubleshooting и безопасности
	IPAddress sql.NullString `json:"ip_address,omitempty" db:"IP_ADDRESS"`

	// Timing
	// CONNECTED_AT - время подключения (timestamp)
	ConnectedAt time.Time `json:"connected_at" db:"CONNECTED_AT"`

	// LAST_HEARTBEAT - последний heartbeat от трейдера
	// Обновляется каждые ~30 секунд
	// Если протух (NOW() - LAST_HEARTBEAT > 2 минуты), сессия считается мертвой
	LastHeartbeat time.Time `json:"last_heartbeat" db:"LAST_HEARTBEAT"`

	// ENDED_AT - когда сессия завершилась
	// NULL = сессия еще активна
	// NOT NULL = сессия завершена
	EndedAt sql.NullTime `json:"ended_at,omitempty" db:"ENDED_AT"`

	// Disconnect Tracking
	// DISCONNECT_REASON - причина отключения
	//   'graceful' - нормальное завершение (SIGTERM, shutdown)
	//   'timeout' - таймаут (нет heartbeat)
	//   'error' - ошибка на стороне клиента или сервера
	//   'server_shutdown' - CTS-Core перезагружается
	//   'kicked' - админ принудительно отключил
	DisconnectReason sql.NullString `json:"disconnect_reason,omitempty" db:"DISCONNECT_REASON"`

	// ERROR_MESSAGE - детали ошибки, если DISCONNECT_REASON='error'
	ErrorMessage sql.NullString `json:"error_message,omitempty" db:"ERROR_MESSAGE"`
}

// DisconnectReason - enum для причин отключения
type DisconnectReason string

const (
	DisconnectGraceful       DisconnectReason = "graceful"
	DisconnectTimeout        DisconnectReason = "timeout"
	DisconnectError          DisconnectReason = "error"
	DisconnectServerShutdown DisconnectReason = "server_shutdown"
	DisconnectKicked         DisconnectReason = "kicked"
)
