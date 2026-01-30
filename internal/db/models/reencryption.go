package models

import (
	"database/sql"
	"time"
)

// ==============================================================================
// REENCRYPTION_JOBS MODEL
// ==============================================================================
//
// Назначение: Управление заданиями по пере-шифрованию после ротации ключей HSM
//
// Контекст проблемы:
//   hsm-service поддерживает key rotation (смена KEK - Key Encryption Keys)
//   После ротации нужно пере-шифровать все данные:
//     - USER_2FA.SECRET_ENC (2FA secrets)
//     - EXCHANGE_ACCOUNTS.API_KEY_ENC, API_SECRET_ENC (API ключи бирж)
//     - Другие зашифрованные поля
//
// Как работает ротация:
//   1. Админ создает новый KEK в hsm-service (key_id: kek-2fa-v2)
//   2. hsm-service НЕ удаляет старый ключ (kek-2fa-v1) - он нужен для расшифровки
//   3. CTS-Core создает REENCRYPTION_JOBS запись
//   4. Background scheduler task берет задание и обрабатывает по батчам
//   5. Для каждой записи:
//      a) Расшифровывает старым ключом (kek-2fa-v1)
//      b) Шифрует новым ключом (kek-2fa-v2)
//      c) Обновляет ENC_KEY_VERSION в таблице
//   6. После завершения можно деактивировать старый ключ
//
// Зачем батчи?
//   - Может быть миллионы записей
//   - Обработка всех сразу перегрузит HSM и БД
//   - Батчи по 100-1000 записей позволяют контролировать нагрузку

type ReencryptionJob struct {
	// Primary Key
	ID int `json:"id" db:"ID"`

	// Job Configuration
	// JOB_TYPE - что пере-шифровываем
	//   'user_2fa' - USER_2FA.SECRET_ENC
	//   'exchange_accounts' - EXCHANGE_ACCOUNTS API keys
	//   'other' - другие типы данных
	JobType ReencryptionJobType `json:"job_type" db:"JOB_TYPE"`

	// OLD_KEY_VERSION - версия KEK, С которой мигрируем
	// Пример: 1 (kek-2fa-v1)
	OldKeyVersion int `json:"old_key_version" db:"OLD_KEY_VERSION"`

	// NEW_KEY_VERSION - версия KEK, НА которую мигрируем
	// Пример: 2 (kek-2fa-v2)
	NewKeyVersion int `json:"new_key_version" db:"NEW_KEY_VERSION"`

	// CONTEXT - контекст HSM для key_id
	// Примеры:
	//   "2fa" → key_id будет "kek-2fa-v1", "kek-2fa-v2"
	//   "exchange-key" → key_id будет "kek-exchange-key-v1"
	Context string `json:"context" db:"CONTEXT"`

	// Job Status
	// STATUS - текущее состояние задания
	//   'pending' - создано, ожидает обработки
	//   'in_progress' - обрабатывается сейчас
	//   'completed' - успешно завершено
	//   'failed' - критическая ошибка
	//   'cancelled' - отменено админом
	Status ReencryptionJobStatus `json:"status" db:"STATUS"`

	// Progress Tracking
	// TOTAL_RECORDS - сколько всего записей нужно обработать
	// Рассчитывается при создании job:
	//   SELECT COUNT(*) FROM USER_2FA WHERE ENC_KEY_VERSION = 1
	TotalRecords int `json:"total_records" db:"TOTAL_RECORDS"`

	// PROCESSED_RECORDS - сколько уже обработано успешно
	ProcessedRecords int `json:"processed_records" db:"PROCESSED_RECORDS"`

	// FAILED_RECORDS - сколько не удалось обработать
	// Записи с ошибками не блокируют обработку остальных
	FailedRecords int `json:"failed_records" db:"FAILED_RECORDS"`

	// Processing Configuration
	// BATCH_SIZE - сколько записей обрабатывать за один раз
	// По умолчанию 100. Можно увеличить для быстрой обработки
	// или уменьшить при высокой нагрузке
	BatchSize int `json:"batch_size" db:"BATCH_SIZE"`

	// Timing
	// STARTED_AT - когда начали обработку
	// NULL если еще не начали (STATUS='pending')
	StartedAt sql.NullTime `json:"started_at,omitempty" db:"STARTED_AT"`

	// COMPLETED_AT - когда завершили
	// NULL если еще не завершено
	CompletedAt sql.NullTime `json:"completed_at,omitempty" db:"COMPLETED_AT"`

	// LAST_PROCESSED_AT - timestamp последней обработанной батчи
	// Используется scheduler для отслеживания "зависших" jobs
	// Если NOW() - LAST_PROCESSED_AT > 10 минут, job считается "застрявшим"
	LastProcessedAt sql.NullTime `json:"last_processed_at,omitempty" db:"LAST_PROCESSED_AT"`

	// Error Handling
	// ERROR_MESSAGE - описание критической ошибки (если STATUS='failed')
	ErrorMessage sql.NullString `json:"error_message,omitempty" db:"ERROR_MESSAGE"`

	// Audit
	// DATE_CREATE - когда задание создано
	DateCreate time.Time `json:"date_create" db:"DATE_CREATE"`
}

// ReencryptionJobType - enum для типов re-encryption заданий
type ReencryptionJobType string

const (
	ReencryptionJobTypeUser2FA          ReencryptionJobType = "user_2fa"
	ReencryptionJobTypeExchangeAccounts ReencryptionJobType = "exchange_accounts"
	ReencryptionJobTypeOther            ReencryptionJobType = "other"
)

// ReencryptionJobStatus - enum для статусов заданий
type ReencryptionJobStatus string

const (
	ReencryptionJobStatusPending    ReencryptionJobStatus = "pending"
	ReencryptionJobStatusInProgress ReencryptionJobStatus = "in_progress"
	ReencryptionJobStatusCompleted  ReencryptionJobStatus = "completed"
	ReencryptionJobStatusFailed     ReencryptionJobStatus = "failed"
	ReencryptionJobStatusCancelled  ReencryptionJobStatus = "cancelled"
)

// ==============================================================================
// REENCRYPTION_PROGRESS MODEL
// ==============================================================================
//
// Назначение: Отслеживание прогресса пере-шифрования для КАЖДОЙ записи
//
// Зачем это нужно?
//   - Позволяет retry failed records без повторной обработки успешных
//   - Позволяет продолжить с места остановки при сбое
//   - Audit trail: видим какие конкретно записи не удалось обработать
//
// Как работает:
//   1. Scheduler берет REENCRYPTION_JOBS с STATUS='pending'
//   2. Для каждой записи создает REENCRYPTION_PROGRESS с STATUS='pending'
//   3. Обрабатывает батчами (BATCH_SIZE записей)
//   4. Для каждой записи:
//      - Пытается пере-шифровать
//      - Если успех: STATUS='completed'
//      - Если ошибка: STATUS='failed', увеличивает ATTEMPT_COUNT
//   5. Retry failed records с exponential backoff

type ReencryptionProgress struct {
	// Primary Key
	ID int64 `json:"id" db:"ID"`

	// Relations
	// JOB_ID - parent задание (ссылка на REENCRYPTION_JOBS.ID)
	JobID int `json:"job_id" db:"JOB_ID"`

	// Record Identity
	// TABLE_NAME - имя таблицы с зашифрованными данными
	// Примеры: "USER_2FA", "EXCHANGE_ACCOUNTS"
	TableName string `json:"table_name" db:"TABLE_NAME"`

	// RECORD_ID - PRIMARY KEY записи в таблице
	// Хранится как VARCHAR потому что PK может быть разного типа:
	//   - INT для USER_2FA.ID
	//   - BIGINT для EXCHANGE_ACCOUNTS.ID
	// Пример: "123", "456789"
	RecordID string `json:"record_id" db:"RECORD_ID"`

	// Processing Status
	// STATUS - состояние обработки этой конкретной записи
	//   'pending' - ожидает обработки
	//   'completed' - успешно пере-шифрована
	//   'failed' - ошибка при пере-шифровании
	//   'skipped' - пропущена (например, уже была обработана)
	Status ReencryptionProgressStatus `json:"status" db:"STATUS"`

	// Retry Tracking
	// ATTEMPT_COUNT - сколько раз пытались обработать
	// По умолчанию 0. После каждой неудачной попытки увеличивается.
	// Если ATTEMPT_COUNT >= 3, запись помечается как failed окончательно
	AttemptCount int `json:"attempt_count" db:"ATTEMPT_COUNT"`

	// ERROR_MESSAGE - описание последней ошибки
	// Примеры:
	//   - "HSM decrypt failed: invalid ciphertext"
	//   - "Database update failed: connection timeout"
	ErrorMessage sql.NullString `json:"error_message,omitempty" db:"ERROR_MESSAGE"`

	// Timing
	// PROCESSED_AT - когда успешно обработана (STATUS='completed')
	// NULL если еще не обработана
	ProcessedAt sql.NullTime `json:"processed_at,omitempty" db:"PROCESSED_AT"`

	// DATE_CREATE - когда запись создана
	DateCreate time.Time `json:"date_create" db:"DATE_CREATE"`
}

// ReencryptionProgressStatus - enum для статусов обработки записей
type ReencryptionProgressStatus string

const (
	ReencryptionProgressStatusPending   ReencryptionProgressStatus = "pending"
	ReencryptionProgressStatusCompleted ReencryptionProgressStatus = "completed"
	ReencryptionProgressStatusFailed    ReencryptionProgressStatus = "failed"
	ReencryptionProgressStatusSkipped   ReencryptionProgressStatus = "skipped"
)

// Пример использования:
//
// 1. Создаем задание ротации ключей 2FA:
//    job := ReencryptionJob{
//        JobType:        ReencryptionJobTypeUser2FA,
//        OldKeyVersion:  1,
//        NewKeyVersion:  2,
//        Context:        "2fa",
//        Status:         ReencryptionJobStatusPending,
//        TotalRecords:   5000, // Есть 5000 пользователей с 2FA
//        BatchSize:      100,
//    }
//
// 2. Scheduler начинает обработку:
//    job.Status = ReencryptionJobStatusInProgress
//    job.StartedAt = sql.NullTime{Time: time.Now(), Valid: true}
//
// 3. Создаем progress записи для первого батча (100 пользователей):
//    for _, userID := range firstBatchUserIDs {
//        progress := ReencryptionProgress{
//            JobID:     job.ID,
//            TableName: "USER_2FA",
//            RecordID:  strconv.Itoa(userID),
//            Status:    ReencryptionProgressStatusPending,
//        }
//        // INSERT...
//    }
//
// 4. Обрабатываем каждую запись:
//    for _, progress := range progressRecords {
//        // Decrypt with old key
//        plaintext, err := hsmClient.Decrypt(oldCiphertext, "kek-2fa-v1")
//        if err != nil {
//            progress.Status = ReencryptionProgressStatusFailed
//            progress.ErrorMessage = sql.NullString{String: err.Error(), Valid: true}
//            progress.AttemptCount++
//            continue
//        }
//
//        // Encrypt with new key
//        newCiphertext, err := hsmClient.Encrypt(plaintext, "kek-2fa-v2")
//        if err != nil {
//            progress.Status = ReencryptionProgressStatusFailed
//            // ...
//            continue
//        }
//
//        // Update database
//        _, err = db.Exec("UPDATE USER_2FA SET SECRET_ENC=?, ENC_KEY_VERSION=2 WHERE ID=?",
//            newCiphertext, progress.RecordID)
//
//        progress.Status = ReencryptionProgressStatusCompleted
//        progress.ProcessedAt = sql.NullTime{Time: time.Now(), Valid: true}
//        job.ProcessedRecords++
//    }
//
// 5. После всех батчей:
//    job.Status = ReencryptionJobStatusCompleted
//    job.CompletedAt = sql.NullTime{Time: time.Now(), Valid: true}
