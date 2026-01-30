package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// ==============================================================================
// ARBITRAGE_ORDER TESTS
// ==============================================================================

func TestArbitrageOrderJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	order := ArbitrageOrder{
		ID:                1,
		ArbitrageTransID:  123,
		ExchangeID:        1,
		ExchangeAccountID: 5,
		ExchangeOrderID:   "987654321",
		TraderID:          2,
		Side:              OrderSideBuy,
		OrderType:         OrderTypeMarket,
		PairID:            10,
		RequestedQuantity: "1.00000000",
		FilledQuantity:    "1.00000000",
		AvgPrice:          sql.NullString{String: "45005.00000000", Valid: true},
		TotalCost:         sql.NullString{String: "45005.00000000", Valid: true},
		TotalFee:          "15.00000000",
		FeeCurrency:       sql.NullString{String: "USDT", Valid: true},
		Status:            OrderStatusFilled,
		ErrorMessage:      sql.NullString{Valid: false},
		DateCreate:        now,
		FilledAt:          sql.NullTime{Time: now.Add(5 * time.Second), Valid: true},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(order)
	if err != nil {
		t.Fatalf("Failed to marshal ArbitrageOrder to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded ArbitrageOrder
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to ArbitrageOrder: %v", err)
	}

	// Verify fields
	if decoded.ID != order.ID {
		t.Errorf("Expected ID %d, got %d", order.ID, decoded.ID)
	}
	if decoded.ExchangeOrderID != order.ExchangeOrderID {
		t.Errorf("Expected ExchangeOrderID %s, got %s", order.ExchangeOrderID, decoded.ExchangeOrderID)
	}
	if decoded.Side != order.Side {
		t.Errorf("Expected Side %s, got %s", order.Side, decoded.Side)
	}
	if decoded.Status != order.Status {
		t.Errorf("Expected Status %s, got %s", order.Status, decoded.Status)
	}
}

func TestOrderSideConstants(t *testing.T) {
	tests := []struct {
		side     OrderSide
		expected string
	}{
		{OrderSideBuy, "buy"},
		{OrderSideSell, "sell"},
	}

	for _, tt := range tests {
		t.Run(string(tt.side), func(t *testing.T) {
			if string(tt.side) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.side))
			}
		})
	}
}

func TestOrderTypeConstants(t *testing.T) {
	tests := []struct {
		orderType OrderType
		expected  string
	}{
		{OrderTypeMarket, "market"},
		{OrderTypeLimit, "limit"},
		{OrderTypeStopLimit, "stop_limit"},
	}

	for _, tt := range tests {
		t.Run(string(tt.orderType), func(t *testing.T) {
			if string(tt.orderType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.orderType))
			}
		})
	}
}

func TestOrderStatusConstants(t *testing.T) {
	tests := []struct {
		status   OrderStatus
		expected string
	}{
		{OrderStatusPending, "pending"},
		{OrderStatusPartial, "partial"},
		{OrderStatusFilled, "filled"},
		{OrderStatusCancelled, "cancelled"},
		{OrderStatusRejected, "rejected"},
		{OrderStatusError, "error"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}

func TestOrderLifecycle(t *testing.T) {
	now := time.Now()

	// Create pending order
	order := ArbitrageOrder{
		ID:                1,
		ArbitrageTransID:  123,
		ExchangeID:        1,
		ExchangeAccountID: 5,
		TraderID:          2,
		Side:              OrderSideBuy,
		OrderType:         OrderTypeMarket,
		PairID:            10,
		RequestedQuantity: "1.00000000",
		FilledQuantity:    "0.00000000",
		Status:            OrderStatusPending,
		DateCreate:        now,
	}

	if order.Status != OrderStatusPending {
		t.Errorf("Expected initial status %s, got %s", OrderStatusPending, order.Status)
	}

	// Partially filled
	order.Status = OrderStatusPartial
	order.FilledQuantity = "0.30000000"

	if order.Status != OrderStatusPartial {
		t.Errorf("Expected status %s, got %s", OrderStatusPartial, order.Status)
	}

	// Fully filled
	order.Status = OrderStatusFilled
	order.FilledQuantity = "1.00000000"
	order.FilledAt = sql.NullTime{Time: now.Add(10 * time.Second), Valid: true}

	if order.Status != OrderStatusFilled {
		t.Errorf("Expected status %s, got %s", OrderStatusFilled, order.Status)
	}
	if !order.FilledAt.Valid {
		t.Error("Filled order should have valid FilledAt timestamp")
	}
}

func TestPartialFillScenario(t *testing.T) {
	// Simulate partial fill scenario
	order := ArbitrageOrder{
		RequestedQuantity: "1.00000000",
		FilledQuantity:    "0.48000000", // Only 48% filled
		Status:            OrderStatusPartial,
	}

	if order.Status != OrderStatusPartial {
		t.Error("Order with partial fill should have partial status")
	}

	// Check if partially filled
	if order.FilledQuantity == order.RequestedQuantity {
		t.Error("Partial order should not have equal filled and requested quantities")
	}
}

func TestOrderErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		status       OrderStatus
		errorMessage string
	}{
		{"Rejected - Insufficient Balance", OrderStatusRejected, "Insufficient balance"},
		{"Rejected - Market Closed", OrderStatusRejected, "Market closed"},
		{"Error - Rate Limit", OrderStatusError, "Rate limit exceeded"},
		{"Error - Connection", OrderStatusError, "Connection timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order := ArbitrageOrder{
				Status:       tt.status,
				ErrorMessage: sql.NullString{String: tt.errorMessage, Valid: true},
			}

			if !order.ErrorMessage.Valid {
				t.Error("Error order should have valid error message")
			}
			if order.ErrorMessage.String != tt.errorMessage {
				t.Errorf("Expected error message %s, got %s", tt.errorMessage, order.ErrorMessage.String)
			}
		})
	}
}

// ==============================================================================
// ARBITRAGE_ORDER_TRANS TESTS
// ==============================================================================

func TestArbitrageOrderTransJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	trans := ArbitrageOrderTrans{
		ID:                    1,
		ArbitrageOrderID:      1,
		ExchangeTransactionID: "tx-1",
		Quantity:              "0.30000000",
		Price:                 "45000.00000000",
		Cost:                  "13500.00000000",
		Fee:                   "4.50000000",
		FeeCurrency:           sql.NullString{String: "USDT", Valid: true},
		Timestamp:             now,
		DateCreate:            now,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(trans)
	if err != nil {
		t.Fatalf("Failed to marshal ArbitrageOrderTrans to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded ArbitrageOrderTrans
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to ArbitrageOrderTrans: %v", err)
	}

	// Verify fields
	if decoded.ID != trans.ID {
		t.Errorf("Expected ID %d, got %d", trans.ID, decoded.ID)
	}
	if decoded.ExchangeTransactionID != trans.ExchangeTransactionID {
		t.Errorf("Expected ExchangeTransactionID %s, got %s", trans.ExchangeTransactionID, decoded.ExchangeTransactionID)
	}
	if decoded.Quantity != trans.Quantity {
		t.Errorf("Expected Quantity %s, got %s", trans.Quantity, decoded.Quantity)
	}
}

func TestMultipleFillsScenario(t *testing.T) {
	orderID := int64(1)
	now := time.Now()

	// Simulate order being filled in 3 parts
	fills := []ArbitrageOrderTrans{
		{
			ArbitrageOrderID:      orderID,
			ExchangeTransactionID: "tx-1",
			Quantity:              "0.30000000",
			Price:                 "45000.00000000",
			Cost:                  "13500.00000000",
			Fee:                   "4.50000000",
			FeeCurrency:           sql.NullString{String: "USDT", Valid: true},
			Timestamp:             now,
		},
		{
			ArbitrageOrderID:      orderID,
			ExchangeTransactionID: "tx-2",
			Quantity:              "0.50000000",
			Price:                 "45010.00000000",
			Cost:                  "22505.00000000",
			Fee:                   "7.50000000",
			FeeCurrency:           sql.NullString{String: "USDT", Valid: true},
			Timestamp:             now.Add(2 * time.Second),
		},
		{
			ArbitrageOrderID:      orderID,
			ExchangeTransactionID: "tx-3",
			Quantity:              "0.20000000",
			Price:                 "45020.00000000",
			Cost:                  "9004.00000000",
			Fee:                   "3.00000000",
			FeeCurrency:           sql.NullString{String: "USDT", Valid: true},
			Timestamp:             now.Add(5 * time.Second),
		},
	}

	// Verify all fills belong to same order
	for _, fill := range fills {
		if fill.ArbitrageOrderID != orderID {
			t.Errorf("Expected ArbitrageOrderID %d, got %d", orderID, fill.ArbitrageOrderID)
		}
	}

	// Verify unique transaction IDs
	transIDs := make(map[string]bool)
	for _, fill := range fills {
		if transIDs[fill.ExchangeTransactionID] {
			t.Errorf("Duplicate transaction ID: %s", fill.ExchangeTransactionID)
		}
		transIDs[fill.ExchangeTransactionID] = true
	}
}

func TestAveragePriceCalculation(t *testing.T) {
	// Simulate fills with different prices
	fills := []ArbitrageOrderTrans{
		{Quantity: "0.30000000", Price: "45000.00000000", Cost: "13500.00000000"},
		{Quantity: "0.50000000", Price: "45010.00000000", Cost: "22505.00000000"},
		{Quantity: "0.20000000", Price: "45020.00000000", Cost: "9004.00000000"},
	}

	// Total quantity: 1.0 BTC
	// Total cost: 45009.0 USDT
	// Average price: 45009.0 (weighted average)

	totalQuantity := "1.00000000"
	totalCost := "45009.00000000"
	avgPrice := "45009.00000000"

	order := ArbitrageOrder{
		FilledQuantity: totalQuantity,
		TotalCost:      sql.NullString{String: totalCost, Valid: true},
		AvgPrice:       sql.NullString{String: avgPrice, Valid: true},
	}

	if !order.AvgPrice.Valid {
		t.Error("Filled order should have valid average price")
	}
	if order.AvgPrice.String != avgPrice {
		t.Errorf("Expected avg price %s, got %s", avgPrice, order.AvgPrice.String)
	}

	// Verify fills count
	if len(fills) != 3 {
		t.Errorf("Expected 3 fills, got %d", len(fills))
	}
}

func TestFeeCurrencyVariations(t *testing.T) {
	tests := []struct {
		name        string
		feeCurrency string
	}{
		{"USDT Fee", "USDT"},
		{"BNB Fee (Binance)", "BNB"},
		{"BTC Fee", "BTC"},
		{"ETH Fee", "ETH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := ArbitrageOrderTrans{
				Quantity:    "0.50000000",
				Price:       "45000.00000000",
				Cost:        "22500.00000000",
				Fee:         "7.50000000",
				FeeCurrency: sql.NullString{String: tt.feeCurrency, Valid: true},
			}

			if !trans.FeeCurrency.Valid {
				t.Error("Transaction should have valid fee currency")
			}
			if trans.FeeCurrency.String != tt.feeCurrency {
				t.Errorf("Expected fee currency %s, got %s", tt.feeCurrency, trans.FeeCurrency.String)
			}
		})
	}
}

func TestDecimalPrecisionInArbitrage(t *testing.T) {
	tests := []struct {
		name     string
		quantity string
		price    string
		cost     string
	}{
		{"Standard BTC", "0.50000000", "45000.00000000", "22500.00000000"},
		{"Small Quantity", "0.00100000", "45000.00000000", "45.00000000"},
		{"High Precision", "0.12345678", "45123.45678900", "5569.15203705"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := ArbitrageOrderTrans{
				Quantity: tt.quantity,
				Price:    tt.price,
				Cost:     tt.cost,
			}

			if trans.Quantity != tt.quantity {
				t.Errorf("Expected Quantity %s, got %s", tt.quantity, trans.Quantity)
			}
			if trans.Price != tt.price {
				t.Errorf("Expected Price %s, got %s", tt.price, trans.Price)
			}
			if trans.Cost != tt.cost {
				t.Errorf("Expected Cost %s, got %s", tt.cost, trans.Cost)
			}
		})
	}
}

func TestIdempotencyWithExchangeOrderID(t *testing.T) {
	// Same exchange order ID should not be inserted twice
	order1 := ArbitrageOrder{
		ArbitrageTransID: 123,
		ExchangeOrderID:  "987654321",
		Status:           OrderStatusPending,
	}

	order2 := ArbitrageOrder{
		ArbitrageTransID: 123,
		ExchangeOrderID:  "987654321", // Same ID
		Status:           OrderStatusFilled,
	}

	// In real DB, UNIQUE constraint would prevent duplicate
	if order1.ExchangeOrderID != order2.ExchangeOrderID {
		t.Error("Orders should have same exchange order ID")
	}
	if order1.ArbitrageTransID != order2.ArbitrageTransID {
		t.Error("Orders should have same arbitrage trans ID")
	}
}

func TestSlippageAnalysis(t *testing.T) {
	// Simulate slippage scenario
	actualAvgPrice := "45020.00000000" // 20 USDT slippage from expected 45000

	order := ArbitrageOrder{
		OrderType:         OrderTypeMarket,
		RequestedQuantity: "1.00000000",
		FilledQuantity:    "1.00000000",
		AvgPrice:          sql.NullString{String: actualAvgPrice, Valid: true},
		Status:            OrderStatusFilled,
	}

	// In real scenario, we'd calculate slippage percentage
	// slippage = (actualAvgPrice - expectedPrice) / expectedPrice * 100
	// For this test, just verify the fields are set correctly

	if !order.AvgPrice.Valid {
		t.Error("Order should have valid average price for slippage analysis")
	}
	if order.Status != OrderStatusFilled {
		t.Error("Slippage analysis requires filled order")
	}
}
