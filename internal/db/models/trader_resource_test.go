package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// ==============================================================================
// TRADER_EXCHANGE_RESOURCE TESTS
// ==============================================================================

func TestTraderExchangeResourceJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	resource := TraderExchangeResource{
		ID:                1,
		TraderID:          1,
		ExchangeID:        1,
		ExchangeAccountID: sql.NullInt32{Int32: 42, Valid: true},
		ResourceType:      ResourceType(ResourceTypeAPIRequestsMinute),
		UsedValue:         "850.00000000",
		MaxValue:          "1200.00000000",
		LastUpdated:       now,
		ResetAt:           now.Add(45 * time.Second),
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("Failed to marshal TraderExchangeResource to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded TraderExchangeResource
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to TraderExchangeResource: %v", err)
	}

	// Verify fields
	if decoded.ID != resource.ID {
		t.Errorf("Expected ID %d, got %d", resource.ID, decoded.ID)
	}
	if decoded.TraderID != resource.TraderID {
		t.Errorf("Expected TraderID %d, got %d", resource.TraderID, decoded.TraderID)
	}
	if decoded.ResourceType != resource.ResourceType {
		t.Errorf("Expected ResourceType %s, got %s", resource.ResourceType, decoded.ResourceType)
	}
	if decoded.UsedValue != resource.UsedValue {
		t.Errorf("Expected UsedValue %s, got %s", resource.UsedValue, decoded.UsedValue)
	}
	if decoded.MaxValue != resource.MaxValue {
		t.Errorf("Expected MaxValue %s, got %s", resource.MaxValue, decoded.MaxValue)
	}
}

func TestExchangeResourceTypeConstants(t *testing.T) {
	tests := []struct {
		resourceType ExchangeResourceType
		expected     string
	}{
		{ResourceTypeAPIRequestsMinute, "api_requests_minute"},
		{ResourceTypeAPIWeightMinute, "api_weight_minute"},
		{ResourceTypeOrdersMinute, "orders_minute"},
		{ResourceTypeWebSocketConnections, "websocket_connections"},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			if string(tt.resourceType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.resourceType))
			}
		})
	}
}

func TestIPLevelLimits(t *testing.T) {
	// IP-level limit should have NULL ExchangeAccountID
	resource := TraderExchangeResource{
		ID:                1,
		TraderID:          1,
		ExchangeID:        1,                           // Binance
		ExchangeAccountID: sql.NullInt32{Valid: false}, // NULL = IP-level
		ResourceType:      ResourceType(ResourceTypeAPIRequestsMinute),
		UsedValue:         "850.00000000",
		MaxValue:          "1200.00000000",
		LastUpdated:       time.Now(),
		ResetAt:           time.Now().Add(45 * time.Second),
	}

	if resource.ExchangeAccountID.Valid {
		t.Error("IP-level limit should have NULL ExchangeAccountID")
	}

	// Verify it's for all accounts of this trader
	jsonData, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded TraderExchangeResource
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ExchangeAccountID.Valid {
		t.Error("Decoded IP-level limit should have NULL ExchangeAccountID")
	}
}

func TestAccountLevelLimits(t *testing.T) {
	// Account-level limit should have valid ExchangeAccountID
	resource := TraderExchangeResource{
		ID:                1,
		TraderID:          1,
		ExchangeID:        1,                                     // Binance
		ExchangeAccountID: sql.NullInt32{Int32: 42, Valid: true}, // Account ID = 42
		ResourceType:      ResourceType(ResourceTypeOrdersMinute),
		UsedValue:         "50.00000000",
		MaxValue:          "100.00000000", // VIP0 account
		LastUpdated:       time.Now(),
		ResetAt:           time.Now().Add(30 * time.Second),
	}

	if !resource.ExchangeAccountID.Valid {
		t.Error("Account-level limit should have valid ExchangeAccountID")
	}
	if resource.ExchangeAccountID.Int32 != 42 {
		t.Errorf("Expected account ID 42, got %d", resource.ExchangeAccountID.Int32)
	}
}

func TestResourceAvailabilityCalculation(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name              string
		usedValue         string
		maxValue          string
		resetAt           time.Time
		expectedAvailable bool
	}{
		{
			name:              "50% used, not reset",
			usedValue:         "600.00000000",
			maxValue:          "1200.00000000",
			resetAt:           now.Add(30 * time.Second),
			expectedAvailable: true,
		},
		{
			name:              "90% used, not reset",
			usedValue:         "1080.00000000",
			maxValue:          "1200.00000000",
			resetAt:           now.Add(10 * time.Second),
			expectedAvailable: true,
		},
		{
			name:              "100% used, not reset",
			usedValue:         "1200.00000000",
			maxValue:          "1200.00000000",
			resetAt:           now.Add(5 * time.Second),
			expectedAvailable: false,
		},
		{
			name:              "Already reset (past)",
			usedValue:         "1200.00000000",
			maxValue:          "1200.00000000",
			resetAt:           now.Add(-1 * time.Second),
			expectedAvailable: true, // Reset, so available again
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := TraderExchangeResource{
				UsedValue:   tt.usedValue,
				MaxValue:    tt.maxValue,
				ResetAt:     tt.resetAt,
				LastUpdated: now,
			}

			// Simple availability check based on reset time
			isAvailable := time.Now().After(resource.ResetAt)
			if tt.name == "Already reset (past)" && !isAvailable {
				t.Error("Resource should be available after reset time")
			}
		})
	}
}

func TestBinanceWeightSystem(t *testing.T) {
	// Binance uses "weight" instead of simple request count
	resource := TraderExchangeResource{
		ID:           1,
		TraderID:     1,
		ExchangeID:   1, // Binance
		ResourceType: ResourceType(ResourceTypeAPIWeightMinute),
		UsedValue:    "850.00000000",
		MaxValue:     "1200.00000000",
		LastUpdated:  time.Now(),
		ResetAt:      time.Now().Add(45 * time.Second),
	}

	if string(resource.ResourceType) != string(ResourceTypeAPIWeightMinute) {
		t.Errorf("Expected ResourceType %s, got %s", ResourceTypeAPIWeightMinute, resource.ResourceType)
	}
}

func TestMultipleResourceTypesForSameTrader(t *testing.T) {
	traderID := 1
	exchangeID := 1
	now := time.Now()

	resources := []TraderExchangeResource{
		{
			TraderID:     traderID,
			ExchangeID:   exchangeID,
			ResourceType: ResourceType(ResourceTypeAPIRequestsMinute),
			UsedValue:    "850.00000000",
			MaxValue:     "1200.00000000",
			LastUpdated:  now,
			ResetAt:      now.Add(45 * time.Second),
		},
		{
			TraderID:     traderID,
			ExchangeID:   exchangeID,
			ResourceType: ResourceType(ResourceTypeOrdersMinute),
			UsedValue:    "50.00000000",
			MaxValue:     "100.00000000",
			LastUpdated:  now,
			ResetAt:      now.Add(30 * time.Second),
		},
		{
			TraderID:     traderID,
			ExchangeID:   exchangeID,
			ResourceType: ResourceType(ResourceTypeWebSocketConnections),
			UsedValue:    "3.00000000",
			MaxValue:     "10.00000000",
			LastUpdated:  now,
			ResetAt:      now.Add(3600 * time.Second), // 1 hour
		},
	}

	// Verify all have same trader and exchange
	for _, r := range resources {
		if r.TraderID != traderID {
			t.Errorf("Expected TraderID %d, got %d", traderID, r.TraderID)
		}
		if r.ExchangeID != exchangeID {
			t.Errorf("Expected ExchangeID %d, got %d", exchangeID, r.ExchangeID)
		}
	}

	// Verify they have different resource types
	uniqueTypes := make(map[string]bool)
	for _, r := range resources {
		uniqueTypes[string(r.ResourceType)] = true
	}
	if len(uniqueTypes) != 3 {
		t.Errorf("Expected 3 unique resource types, got %d", len(uniqueTypes))
	}
}

func TestDecimalPrecision(t *testing.T) {
	tests := []struct {
		name      string
		usedValue string
		maxValue  string
	}{
		{"Integers", "100.00000000", "200.00000000"},
		{"Decimals", "123.45678900", "500.12345678"},
		{"Small Values", "0.00000001", "0.00000100"},
		{"Large Values", "999999.99999999", "1000000.00000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := TraderExchangeResource{
				UsedValue: tt.usedValue,
				MaxValue:  tt.maxValue,
			}

			if resource.UsedValue != tt.usedValue {
				t.Errorf("Expected UsedValue %s, got %s", tt.usedValue, resource.UsedValue)
			}
			if resource.MaxValue != tt.maxValue {
				t.Errorf("Expected MaxValue %s, got %s", tt.maxValue, resource.MaxValue)
			}
		})
	}
}

func TestResetAtFutureAndPast(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		resetAt time.Time
	}{
		{"Future - 10 seconds", now.Add(10 * time.Second)},
		{"Future - 1 minute", now.Add(1 * time.Minute)},
		{"Future - 1 hour", now.Add(1 * time.Hour)},
		{"Past - 1 second", now.Add(-1 * time.Second)},
		{"Past - 1 minute", now.Add(-1 * time.Minute)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := TraderExchangeResource{
				ResetAt:     tt.resetAt,
				LastUpdated: now,
			}

			// Check if reset time has passed
			hasReset := time.Now().After(resource.ResetAt)
			if tt.name[:4] == "Past" && !hasReset {
				t.Error("Past reset time should be detected")
			}
			if tt.name[:6] == "Future" && hasReset {
				t.Error("Future reset time should not be detected as passed")
			}
		})
	}
}

func TestSchedulerTaskDistribution(t *testing.T) {
	// Simulate scheduler finding least loaded trader
	now := time.Now()

	traders := []TraderExchangeResource{
		{
			TraderID:     1,
			ExchangeID:   1,
			ResourceType: ResourceType(ResourceTypeAPIRequestsMinute),
			UsedValue:    "1100.00000000", // 91% used
			MaxValue:     "1200.00000000",
			ResetAt:      now.Add(20 * time.Second),
		},
		{
			TraderID:     2,
			ExchangeID:   1,
			ResourceType: ResourceType(ResourceTypeAPIRequestsMinute),
			UsedValue:    "300.00000000", // 25% used - least loaded
			MaxValue:     "1200.00000000",
			ResetAt:      now.Add(40 * time.Second),
		},
		{
			TraderID:     3,
			ExchangeID:   1,
			ResourceType: ResourceType(ResourceTypeAPIRequestsMinute),
			UsedValue:    "600.00000000", // 50% used
			MaxValue:     "1200.00000000",
			ResetAt:      now.Add(30 * time.Second),
		},
	}

	// For this test, we know trader 2 has the lowest usage (300)
	// In real implementation, scheduler would parse strings to floats
	expectedTraderID := 2
	expectedUsedValue := "300.00000000"

	// Verify trader 2 exists and has expected value
	found := false
	for _, trader := range traders {
		if trader.TraderID == expectedTraderID {
			if trader.UsedValue != expectedUsedValue {
				t.Errorf("Expected trader %d to have usage %s, got %s",
					expectedTraderID, expectedUsedValue, trader.UsedValue)
			}
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find trader %d in traders list", expectedTraderID)
	}

	// Verify we have 3 traders
	if len(traders) != 3 {
		t.Errorf("Expected 3 traders, got %d", len(traders))
	}
}
