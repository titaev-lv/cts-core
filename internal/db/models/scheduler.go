package models

import (
	"database/sql"
	"time"
)

// ==============================================================================
// SCHEDULER_TASKS MODEL
// ==============================================================================
//
// Назначение: Управление фоновыми задачами CTS-Core
//
// Зачем нужен scheduler?
//   CTS-Core - это долгоживущий сервис, который должен выполнять периодические задачи:
//     - Cleanup: удаление старых логов/сессий
//     - Re-encryption: пере-шифрование после ротации ключей
//     - Monitoring: проверка здоровья трейдеров
//     - Maintenance: сброс дневных лимитов, архивация данных
//
// Архитектура:
//   В CTS-Core будет scheduler goroutine, которая:
//     1. Каждые N секунд читает SCHEDULER_TASKS
//     2. Проверяет NEXT_RUN_AT для каждой задачи
//     3. Если время пришло → запускает задачу
//     4. Обновляет LAST_RUN_AT, STATUS, LAST_RUN_DURATION_MS
//     5. Рассчитывает NEXT_RUN_AT для следующего запуска
//
// Типы задач:
//   1. Cron-based (SCHEDULE_CRON):
//      - Пример: "0 2 * * *" = каждый день в 2:00 AM
//      - Используется для cleanup, backup, reset лимитов
//
//   2. Interval-based (SCHEDULE_INTERVAL_SEC):
//      - Пример: 60 = каждые 60 секунд
//      - Используется для мониторинга, проверки re-encryption jobs
//
// Lifecycle задачи:
//   1. STATUS='idle' - ждет следующего запуска
//   2. NEXT_RUN_AT наступает
//   3. STATUS='running' - выполняется сейчас
//   4. После завершения:
//      - Успех: STATUS='idle', LAST_RUN_STATUS='success'
//      - Ошибка: STATUS='idle', LAST_RUN_STATUS='error', ERROR_COUNT++
//      - Timeout: STATUS='idle', LAST_RUN_STATUS='timeout'
//   5. Если ENABLED=FALSE: STATUS='disabled', задача не запускается
//
// Default Tasks (создаются при миграции):
//   1. cleanup_trader_sessions - удаление старых сессий (7 дней)
//   2. cleanup_audit_logs - удаление старых логов (180 дней)
//   3. reset_daily_limits - сброс дневных лимитов (midnight)
//   4. check_reencryption_jobs - проверка re-encryption (каждую минуту)

type SchedulerTask struct {
	// Primary Key
	ID int `json:"id" db:"ID"`

	// Task Identity
	// TASK_NAME - уникальное имя задачи
	// Примеры:
	//   'cleanup_trader_sessions'
	//   'cleanup_audit_logs'
	//   'reencrypt_2fa'
	//   'check_trader_health'
	// UNIQUE constraint предотвращает дубликаты
	TaskName string `json:"task_name" db:"TASK_NAME"`

	// TASK_TYPE - категория задачи
	// Используется для группировки и фильтрации
	TaskType TaskType `json:"task_type" db:"TASK_TYPE"`

	// Schedule Configuration
	// SCHEDULE_CRON - cron expression (для периодических задач)
	// Формат: "минута час день_месяца месяц день_недели"
	// Примеры:
	//   "0 2 * * *" - каждый день в 2:00 AM
	//   "0 */6 * * *" - каждые 6 часов
	//   "0 0 1 * *" - первого числа каждого месяца в midnight
	// NULL если используется SCHEDULE_INTERVAL_SEC
	ScheduleCron sql.NullString `json:"schedule_cron" db:"SCHEDULE_CRON"`

	// SCHEDULE_INTERVAL_SEC - интервал в секундах
	// Примеры:
	//   60 - каждую минуту
	//   300 - каждые 5 минут
	//   3600 - каждый час
	// NULL если используется SCHEDULE_CRON
	//
	// Разница между cron и interval:
	//   Cron: точное время ("в 2:00 AM каждый день")
	//   Interval: относительное время ("через 60 секунд после последнего запуска")
	ScheduleIntervalSec sql.NullInt32 `json:"schedule_interval_sec" db:"SCHEDULE_INTERVAL_SEC"`

	// Task Control
	// ENABLED - включена ли задача?
	// FALSE = задача не будет запускаться (даже если пришло время)
	// Используется для временного отключения без удаления
	Enabled bool `json:"enabled" db:"ENABLED"`

	// STATUS - текущее состояние задачи
	//   'idle' - ожидает следующего запуска (нормальное состояние)
	//   'running' - выполняется прямо сейчас
	//   'failed' - последний запуск завершился ошибкой, но задача продолжит попытки
	//   'disabled' - задача выключена (ENABLED=FALSE)
	Status TaskStatus `json:"status" db:"STATUS"`

	// Last Run Information
	// LAST_RUN_AT - когда задача запускалась последний раз
	// NULL = еще ни разу не запускалась
	LastRunAt sql.NullTime `json:"last_run_at" db:"LAST_RUN_AT"`

	// LAST_RUN_DURATION_MS - сколько заняло последнее выполнение (миллисекунды)
	// Используется для:
	//   - Мониторинг производительности (если задача стала медленной)
	//   - Timeout detection (если задача висит слишком долго)
	// Пример: 1500 = 1.5 секунды
	LastRunDurationMS sql.NullInt32 `json:"last_run_duration_ms" db:"LAST_RUN_DURATION_MS"`

	// LAST_RUN_STATUS - результат последнего запуска
	//   'success' - успешно завершена
	//   'error' - ошибка во время выполнения (смотри LAST_ERROR)
	//   'timeout' - задача не завершилась за отведенное время
	LastRunStatus sql.NullString `json:"last_run_status" db:"LAST_RUN_STATUS"`

	// LAST_ERROR - текст ошибки последнего неудачного запуска
	// Пример: "Failed to connect to database"
	// NULL если LAST_RUN_STATUS='success'
	LastError sql.NullString `json:"last_error" db:"LAST_ERROR"`

	// Next Run Scheduling
	// NEXT_RUN_AT - когда задача запустится в следующий раз
	// Рассчитывается после каждого запуска на основе SCHEDULE_CRON или SCHEDULE_INTERVAL_SEC
	// Scheduler проверяет: if NEXT_RUN_AT <= NOW() AND ENABLED=TRUE → запустить
	NextRunAt sql.NullTime `json:"next_run_at" db:"NEXT_RUN_AT"`

	// Statistics
	// RUN_COUNT - сколько раз задача запускалась всего
	// Увеличивается после каждого запуска (успешного или нет)
	RunCount int `json:"run_count" db:"RUN_COUNT"`

	// ERROR_COUNT - сколько раз задача завершилась с ошибкой
	// Если ERROR_COUNT растет → нужно разбираться
	ErrorCount int `json:"error_count" db:"ERROR_COUNT"`

	// Task Configuration
	// CONFIG - JSON с настройками конкретной задачи
	// Примеры:
	//   cleanup_trader_sessions: {"retention_days": 7}
	//   reencrypt_2fa: {"batch_size": 100}
	//   check_trader_health: {"timeout_sec": 30, "alert_if_down": true}
	//
	// Зачем JSON?
	//   - Каждая задача может иметь свои уникальные параметры
	//   - Можно менять конфигурацию без ALTER TABLE
	//   - Легко добавлять новые задачи с разными настройками
	Config sql.NullString `json:"config" db:"CONFIG"` // JSON

	// Audit Fields
	DateCreate  time.Time     `json:"date_create" db:"DATE_CREATE"`
	DateModify  time.Time     `json:"date_modify" db:"DATE_MODIFY"`
	UserCreated sql.NullInt32 `json:"user_created" db:"USER_CREATED"` // USER.ID
	UserModify  sql.NullInt32 `json:"user_modify" db:"USER_MODIFY"`   // USER.ID
}

// TaskType - типы задач
type TaskType string

const (
	// Cleanup Tasks (очистка старых данных)
	// - Удаление старых сессий трейдеров
	// - Удаление старых audit logs
	// - Архивация завершенных ордеров
	TaskTypeCleanup TaskType = "cleanup"

	// Re-encryption Tasks (пере-шифрование)
	// - Проверка pending re-encryption jobs
	// - Batch re-encryption старых данных
	TaskTypeReencryption TaskType = "reencryption"

	// Monitoring Tasks (мониторинг здоровья)
	// - Проверка живых ли трейдеры
	// - Проверка доступности HSM
	// - Проверка нагрузки на биржи
	TaskTypeMonitoring TaskType = "monitoring"

	// Maintenance Tasks (обслуживание)
	// - Сброс дневных лимитов
	// - Обновление статистики
	// - Оптимизация таблиц
	TaskTypeMaintenance TaskType = "maintenance"

	// Other (прочие задачи)
	TaskTypeOther TaskType = "other"
)

// TaskStatus - статус задачи
type TaskStatus string

const (
	// Idle - ожидает следующего запуска (нормальное состояние)
	TaskStatusIdle TaskStatus = "idle"

	// Running - выполняется прямо сейчас
	// ВАЖНО: если STATUS='running' слишком долго → возможен deadlock/hang
	TaskStatusRunning TaskStatus = "running"

	// Failed - последний запуск завершился ошибкой
	// Задача продолжит попытки при следующем NEXT_RUN_AT
	TaskStatusFailed TaskStatus = "failed"

	// Disabled - задача выключена (ENABLED=FALSE)
	TaskStatusDisabled TaskStatus = "disabled"
)

// TaskRunStatus - результат выполнения задачи
type TaskRunStatus string

const (
	TaskRunStatusSuccess TaskRunStatus = "success"
	TaskRunStatusError   TaskRunStatus = "error"
	TaskRunStatusTimeout TaskRunStatus = "timeout"
)

// DefaultSchedulerTasks - задачи по умолчанию (создаются при миграции)
var DefaultSchedulerTasks = []SchedulerTask{
	{
		TaskName:     "cleanup_trader_sessions",
		TaskType:     TaskTypeCleanup,
		ScheduleCron: sql.NullString{String: "0 2 * * *", Valid: true}, // 2:00 AM daily
		Enabled:      true,
		Config:       sql.NullString{String: `{"retention_days": 7}`, Valid: true},
	},
	{
		TaskName:     "cleanup_audit_logs",
		TaskType:     TaskTypeCleanup,
		ScheduleCron: sql.NullString{String: "0 3 * * *", Valid: true}, // 3:00 AM daily
		Enabled:      true,
		Config:       sql.NullString{String: `{"retention_days": 180}`, Valid: true},
	},
	{
		TaskName:     "reset_daily_limits",
		TaskType:     TaskTypeMaintenance,
		ScheduleCron: sql.NullString{String: "0 0 * * *", Valid: true}, // Midnight daily
		Enabled:      true,
		Config:       sql.NullString{String: `{}`, Valid: true},
	},
	{
		TaskName:            "check_reencryption_jobs",
		TaskType:            TaskTypeReencryption,
		ScheduleIntervalSec: sql.NullInt32{Int32: 60, Valid: true}, // Every 60 seconds
		Enabled:             true,
		Config:              sql.NullString{String: `{"check_interval_sec": 60}`, Valid: true},
	},
}
