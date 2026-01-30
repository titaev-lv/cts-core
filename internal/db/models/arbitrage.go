package models

import (
	"database/sql"
	"time"
)

// ==============================================================================
// ARBITRAGE HIERARCHY
// ==============================================================================
//
// Иерархия арбитражных сделок (3 уровня):
//
// 1. ARBITRAGE_TRANS (TOP LEVEL) - вся арбитражная сделка целиком
//    Пример: "Купить BTC на Binance, продать на OKX"
//    Таблица уже существует, мы добавили только ALTER для BIGINT ID
//
// 2. ARBITRAGE_ORDER (MIDDLE LEVEL) - ордера на каждой бирже
//    Пример: 
//      - Order #1: BUY 0.5 BTC на Binance
//      - Order #2: SELL 0.5 BTC на OKX
//
// 3. ARBITRAGE_ORDER_TRANS (BOTTOM LEVEL) - детальные fills/partials
//    Пример для Order #1:
//      - Fill #1: 0.3 BTC @ $45,000
//      - Fill #2: 0.2 BTC @ $45,010
//
// Зачем нужна эта иерархия?
//   - Один ордер может исполниться частями (partial fills)
//   - Нужно отслеживать среднюю цену исполнения
//   - Комиссии могут быть разные для каждого fill
//   - Детальный аудит для анализа проскальзывания

// ==============================================================================
// ARBITRAGE_ORDER MODEL (MIDDLE LEVEL)
// ==============================================================================
//
// Назначение: Tracking ордеров на каждой бирже в рамках арбитражной сделки
//
// Жизненный цикл:
//   1. Создается с STATUS='pending' когда трейдер получает задачу
//   2. Трейдер размещает ордер на бирже
//   3. Записывает EXCHANGE_ORDER_ID (ID ордера от биржи)
//   4. Обновляет STATUS по мере исполнения:
//      'pending' → 'partial' (частично исполнен) → 'filled' (полностью)
//   5. Если ошибка: STATUS='error', записывает ERROR_MESSAGE
//
// Idempotency:
//   UNIQUE(arbitrage_trans_id, exchange_order_id) предотвращает дубликаты

type ArbitrageOrder struct {
	// Primary Key
	ID int64 `json:"id" db:"ID"`

	// Relations
	// ARBITRAGE_TRANS_ID - parent запись (ссылка на ARBITRAGE_TRANS.ID)
	// Это ID всей арбитражной сделки
	ArbitrageTransID int64 `json:"arbitrage_trans_id" db:"ARBITRAGE_TRANS_ID"`

	// EXCHANGE_ID - на какой бирже размещен ордер (ссылка на EXCHANGE.ID)
	// Пример: 1 = Binance, 2 = OKX
	ExchangeID int `json:"exchange_id" db:"EXCHANGE_ID"`

	// EXCHANGE_ACCOUNT_ID - какой аккаунт использован (ссылка на EXCHANGE_ACCOUNTS.ID)
	// Это нужно для:
	//   - Знать ключи API какого аккаунта использовались
	//   - Tracking расходов по аккаунтам
	//   - Compliance (какой аккаунт делал какие операции)
	ExchangeAccountID int `json:"exchange_account_id" db:"EXCHANGE_ACCOUNT_ID"`

	// EXCHANGE_ORDER_ID - ID ордера от биржи (UNIQUE вместе с ARBITRAGE_TRANS_ID!)
	// Пример для Binance: "12345678"
	// Пример для OKX: "559515699449856"
	// ВАЖНО: Это поле используется для idempotency
	ExchangeOrderID string `json:"exchange_order_id" db:"EXCHANGE_ORDER_ID"`

	// TRADER_ID - кто выполнял ордер (ссылка на TRADER.ID)
	// Это важно для:
	//   - Performance analysis (какой трейдер быстрее)
	//   - Debugging (если ордер failed, смотрим логи трейдера)
	//   - Load tracking (сколько ордеров у каждого трейдера)
	TraderID int `json:"trader_id" db:"TRADER_ID"`

	// Order Details
	// SIDE - направление ордера
	//   'buy' = покупка (забираем ликвидность, платим maker fee обычно)
	//   'sell' = продажа
	Side OrderSide `json:"side" db:"SIDE"`

	// ORDER_TYPE - тип ордера
	//   'market' - рыночный (исполняется сразу по текущей цене)
	//   'limit' - лимитный (исполняется по указанной цене или лучше)
	//   'stop_limit' - стоп-лимит (для защиты от проскальзывания)
	OrderType OrderType `json:"order_type" db:"ORDER_TYPE"`

	// PAIR_ID - торговая пара (ссылка на TRADE_PAIR.ID)
	// Пример: BTC/USDT, ETH/BTC
	// ВАЖНО: TRADE_PAIR связывает BASE_CURRENCY + QUOTE_CURRENCY + EXCHANGE
	//        Потому что разные биржи могут иметь разные пары
	PairID int `json:"pair_id" db:"PAIR_ID"`

	// Quantity & Execution
	// REQUESTED_QUANTITY - сколько хотели купить/продать (в базовой валюте)
	// Пример: 0.5 BTC
	RequestedQuantity string `json:"requested_quantity" db:"REQUESTED_QUANTITY"` // DECIMAL(30,12)

	// FILLED_QUANTITY - сколько реально исполнилось
	// Может быть меньше REQUESTED_QUANTITY (partial fill)
	// Пример: 0.48 BTC (из 0.5 запрошенных)
	FilledQuantity string `json:"filled_quantity" db:"FILLED_QUANTITY"` // DECIMAL(30,12)

	// Pricing
	// AVG_PRICE - средняя цена исполнения
	// Рассчитывается из ARBITRAGE_ORDER_TRANS (детальных fills)
	// Пример: если fill1 @ $45,000 и fill2 @ $45,010, то avg ≈ $45,005
	AvgPrice sql.NullString `json:"avg_price,omitempty" db:"AVG_PRICE"` // DECIMAL(30,12)

	// TOTAL_COST - общая стоимость в quote валюте
	// TOTAL_COST = FILLED_QUANTITY * AVG_PRICE
	// Пример: 0.48 BTC * $45,005 = $21,602.40
	TotalCost sql.NullString `json:"total_cost,omitempty" db:"TOTAL_COST"` // DECIMAL(30,12)

	// Fees
	// TOTAL_FEE - сумма всех комиссий
	// Накапливается из ARBITRAGE_ORDER_TRANS (каждый fill может иметь свою комиссию)
	TotalFee string `json:"total_fee" db:"TOTAL_FEE"` // DECIMAL(30,12)

	// FEE_CURRENCY - валюта комиссии
	// Часто = quote валюта (USDT), но может быть BNB (на Binance) или другая
	// NULL если комиссия = 0
	FeeCurrency sql.NullString `json:"fee_currency,omitempty" db:"FEE_CURRENCY"`

	// Status & Error Handling
	// STATUS - текущий статус ордера
	//   'pending' - создан, но еще не размещен на бирже
	//   'partial' - частично исполнен
	//   'filled' - полностью исполнен
	//   'cancelled' - отменен (админом или timeout)
	//   'rejected' - биржа отклонила (insufficient balance, etc)
	//   'error' - ошибка при размещении/исполнении
	Status OrderStatus `json:"status" db:"STATUS"`

	// ERROR_MESSAGE - детали ошибки если STATUS='error'
	// Пример: "Insufficient balance", "Market closed", "Rate limit exceeded"
	ErrorMessage sql.NullString `json:"error_message,omitempty" db:"ERROR_MESSAGE"`

	// Timing
	// DATE_CREATE - когда запись создана
	DateCreate time.Time `json:"date_create" db:"DATE_CREATE"`

	// FILLED_AT - когда полностью исполнился (STATUS='filled')
	// NULL если еще не исполнен
	FilledAt sql.NullTime `json:"filled_at,omitempty" db:"FILLED_AT"`
}

// OrderSide - enum для направления ордера
type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderType - enum для типа ордера
type OrderType string

const (
	OrderTypeMarket    OrderType = "market"
	OrderTypeLimit     OrderType = "limit"
	OrderTypeStopLimit OrderType = "stop_limit"
)

// OrderStatus - enum для статуса ордера
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPartial   OrderStatus = "partial"
	OrderStatusFilled    OrderStatus = "filled"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRejected  OrderStatus = "rejected"
	OrderStatusError     OrderStatus = "error"
)

// ==============================================================================
// ARBITRAGE_ORDER_TRANS MODEL (BOTTOM LEVEL)
// ==============================================================================
//
// Назначение: Детальное отслеживание каждого fill/partial в рамках ордера
//
// Зачем нужно?
//   - Один ордер может исполниться несколькими частями (частичные fills)
//   - Каждый fill может быть по разной цене
//   - Каждый fill может иметь разную комиссию
//   - Нужно для:
//     * Расчета средней цены (weighted average)
//     * Анализа проскальзывания (slippage)
//     * Детального аудита
//
// Пример:
//   Ордер на покупку 1.0 BTC исполнился так:
//     Transaction #1: 0.3 BTC @ $45,000 (fee $4.50)
//     Transaction #2: 0.5 BTC @ $45,010 (fee $7.50)
//     Transaction #3: 0.2 BTC @ $45,020 (fee $3.00)
//   Итого: 1.0 BTC, средняя цена $45,009, комиссия $15.00
//
// Idempotency:
//   UNIQUE(arbitrage_order_id, exchange_transaction_id) предотвращает дубликаты

type ArbitrageOrderTrans struct {
	// Primary Key
	ID int64 `json:"id" db:"ID"`

	// Relations
	// ARBITRAGE_ORDER_ID - parent ордер (ссылка на ARBITRAGE_ORDER.ID)
	ArbitrageOrderID int64 `json:"arbitrage_order_id" db:"ARBITRAGE_ORDER_ID"`

	// EXCHANGE_TRANSACTION_ID - ID fill/trade от биржи (UNIQUE!)
	// Пример для Binance: "123456789"
	// Пример для OKX: "559515699449856-1"
	// ВАЖНО: Это поле используется для idempotency (чтобы не записать fill дважды)
	ExchangeTransactionID string `json:"exchange_transaction_id" db:"EXCHANGE_TRANSACTION_ID"`

	// Execution Details
	// QUANTITY - сколько исполнилось в этом fill (в базовой валюте)
	// Пример: 0.3 BTC (из ордера на 1.0 BTC)
	Quantity string `json:"quantity" db:"QUANTITY"` // DECIMAL(30,12)

	// PRICE - цена fill
	// Пример: $45,000 за 1 BTC
	Price string `json:"price" db:"PRICE"` // DECIMAL(30,12)

	// COST - стоимость этого fill (QUANTITY * PRICE)
	// Пример: 0.3 BTC * $45,000 = $13,500
	Cost string `json:"cost" db:"COST"` // DECIMAL(30,12)

	// Fee
	// FEE - комиссия за этот fill
	// Пример: $4.50 (0.1% от $13,500)
	Fee string `json:"fee" db:"FEE"` // DECIMAL(30,12)

	// FEE_CURRENCY - валюта комиссии
	// Часто = quote валюта, но может быть BNB (Binance) или другая
	FeeCurrency sql.NullString `json:"fee_currency,omitempty" db:"FEE_CURRENCY"`

	// Timing
	// TIMESTAMP - когда произошло исполнение (timestamp от биржи)
	// ВАЖНО: Это время от биржи, не от нашего сервера!
	Timestamp time.Time `json:"timestamp" db:"TIMESTAMP"`

	// DATE_CREATE - когда мы записали эту информацию в БД
	DateCreate time.Time `json:"date_create" db:"DATE_CREATE"`
}

// Пример использования:
//
// 1. Создаем ордер:
//    order := ArbitrageOrder{
//        ArbitrageTransID:  123,
//        ExchangeID:        1, // Binance
//        ExchangeAccountID: 5,
//        TraderID:          2,
//        Side:              OrderSideBuy,
//        OrderType:         OrderTypeMarket,
//        PairID:            10, // BTC/USDT
//        RequestedQuantity: "1.0",
//        Status:            OrderStatusPending,
//    }
//
// 2. Трейдер размещает ордер на бирже, получает order_id:
//    order.ExchangeOrderID = "987654321"
//    order.Status = OrderStatusPending
//
// 3. Ордер исполняется частично (first fill):
//    trans1 := ArbitrageOrderTrans{
//        ArbitrageOrderID:      order.ID,
//        ExchangeTransactionID: "tx-1",
//        Quantity:              "0.3",
//        Price:                 "45000.0",
//        Cost:                  "13500.0",
//        Fee:                   "4.5",
//        FeeCurrency:           sql.NullString{String: "USDT", Valid: true},
//    }
//    order.FilledQuantity = "0.3"
//    order.Status = OrderStatusPartial
//
// 4. Ордер исполняется полностью (remaining fills):
//    // ... добавляем trans2, trans3 ...
//    order.FilledQuantity = "1.0"
//    order.Status = OrderStatusFilled
//    order.FilledAt = sql.NullTime{Time: time.Now(), Valid: true}
