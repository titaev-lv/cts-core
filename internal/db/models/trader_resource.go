package models

import (
	"database/sql"
	"time"
)

// ==============================================================================
// TRADER_EXCHANGE_RESOURCE MODEL
// ==============================================================================
//
// Назначение: Tracking использования rate limits трейдерами
//
// Архитектура (ВАЖНО!):
//   Биржи имеют ДВА типа rate limits:
//
//   1. IP-LEVEL LIMITS (на уровне IP адреса):
//      - Пример: Binance 1200 запросов/минуту с одного IP
//      - НЕ зависит от аккаунта (даже если 10 аккаунтов с одного IP - лимит общий)
//      - Трейдер сам следит за своим IP лимитом
//      - В этой таблице: EXCHANGE_ACCOUNT_ID = NULL
//
//   2. ACCOUNT-LEVEL LIMITS (на уровне аккаунта):
//      - Пример: Binance VIP0 = 100 ордеров/секунду, VIP9 = 300 ордеров/секунду
//      - Зависит от верификации/объемов аккаунта
//      - Трейдер получает лимиты из API headers биржи
//      - В этой таблице: EXCHANGE_ACCOUNT_ID = ID аккаунта
//
// Data Flow:
//   1. Трейдер делает запрос к бирже
//   2. Биржа возвращает headers с rate limit info (X-RateLimit-Used, X-RateLimit-Limit)
//   3. Трейдер обновляет свои локальные счетчики
//   4. Каждые 10-30 секунд трейдер отправляет метрики в CTS-Core
//   5. CTS-Core записывает в эту таблицу
//   6. Scheduler использует данные для smart task distribution
//
// Use Cases:
//   - Scheduler: "Найти трейдера с наименьшей нагрузкой на Binance"
//   - Monitoring: "Какие трейдеры близки к rate limit?"
//   - Analytics: "Как распределена нагрузка между трейдерами?"

type TraderExchangeResource struct {
	// Primary Key
	ID int64 `json:"id" db:"ID"`

	// Relations
	// TRADER_ID - какой трейдер (ссылка на TRADER.ID)
	TraderID int `json:"trader_id" db:"TRADER_ID"`

	// EXCHANGE_ID - какая биржа (ссылка на EXCHANGE.ID)
	// Пример: 1 = Binance, 2 = OKX, 3 = Bybit
	ExchangeID int `json:"exchange_id" db:"EXCHANGE_ID"`

	// EXCHANGE_ACCOUNT_ID - аккаунт на бирже (ссылка на EXCHANGE_ACCOUNTS.ID)
	// NULL = IP-level лимит (не привязан к аккаунту)
	// NOT NULL = Account-level лимит (для конкретного аккаунта)
	ExchangeAccountID sql.NullInt32 `json:"exchange_account_id,omitempty" db:"EXCHANGE_ACCOUNT_ID"`

	// Resource Type
	// RESOURCE_TYPE - тип ресурса/лимита
	//   'api_requests_minute' - общие API запросы (IP-level обычно)
	//   'api_weight_minute' - "вес" запросов (Binance использует weight вместо count)
	//   'orders_minute' - ордеры в минуту (account-level обычно)
	//   'websocket_connections' - активные WebSocket соединения
	ResourceType ResourceType `json:"resource_type" db:"RESOURCE_TYPE"`

	// Usage Metrics
	// USED_VALUE - текущее использование (сколько использовано)
	// Пример: 850 из 1200 запросов
	// ВАЖНО: Это значение от трейдера (self-reported)
	UsedValue string `json:"used_value" db:"USED_VALUE"` // DECIMAL(20,8) as string

	// MAX_VALUE - максимальный лимит
	// Пример: 1200 запросов
	// Трейдер узнает это значение из API headers биржи
	MaxValue string `json:"max_value" db:"MAX_VALUE"` // DECIMAL(20,8) as string

	// Timing
	// LAST_UPDATED - когда трейдер последний раз обновил эти метрики
	// Автоматически обновляется MySQL при UPDATE
	LastUpdated time.Time `json:"last_updated" db:"LAST_UPDATED"`

	// RESET_AT - когда счетчик USED_VALUE обнулится
	// Рассчитывается трейдером на основе информации от биржи
	// Пример: для минутных лимитов = начало следующей минуты
	ResetAt time.Time `json:"reset_at" db:"RESET_AT"`
}

// ExchangeResourceType - enum для типов ресурсов/лимитов бирж
type ExchangeResourceType string

const (
	// API запросы в минуту (общий счетчик, обычно IP-level)
	ResourceTypeAPIRequestsMinute ExchangeResourceType = "api_requests_minute"

	// API "вес" в минуту (Binance-specific, более сложные запросы = больше веса)
	ResourceTypeAPIWeightMinute ExchangeResourceType = "api_weight_minute"

	// Ордера в минуту (обычно account-level, зависит от VIP уровня)
	ResourceTypeOrdersMinute ExchangeResourceType = "orders_minute"

	// WebSocket соединения (лимит одновременных подключений)
	ResourceTypeWebSocketConnections ExchangeResourceType = "websocket_connections"
)

// Примеры использования:
//
// 1. IP-level лимит (для всех аккаунтов трейдера на Binance):
//    TraderExchangeResource{
//        TraderID:          1,
//        ExchangeID:        1,  // Binance
//        ExchangeAccountID: sql.NullInt32{Valid: false}, // NULL = IP-level
//        ResourceType:      "api_requests_minute",
//        UsedValue:         "850.00000000",
//        MaxValue:          "1200.00000000",
//        ResetAt:           time.Now().Add(45 * time.Second),
//    }
//
// 2. Account-level лимит (для конкретного аккаунта):
//    TraderExchangeResource{
//        TraderID:          1,
//        ExchangeID:        1,  // Binance
//        ExchangeAccountID: sql.NullInt32{Int32: 42, Valid: true}, // Account ID = 42
//        ResourceType:      "orders_minute",
//        UsedValue:         "50.00000000",
//        MaxValue:          "100.00000000",  // VIP0 аккаунт
//        ResetAt:           time.Now().Add(30 * time.Second),
//    }

// CalculateAvailability вычисляет процент доступности ресурса
// Возвращает значение от 0.0 (полностью занято) до 1.0 (полностью свободно)
// Используется Scheduler для выбора наименее загруженного трейдера
func (r *TraderExchangeResource) CalculateAvailability() float64 {
	// Если ResetAt в прошлом - ресурс уже сброшен, полностью доступен
	if time.Now().After(r.ResetAt) {
		return 1.0
	}

	// Парсим используемое и максимальное значения
	// (в реальном коде нужна обработка ошибок)
	var used, max float64
	// ... парсинг строк в float64 ...

	if max == 0 {
		return 0.0
	}

	available := max - used
	return available / max
}
