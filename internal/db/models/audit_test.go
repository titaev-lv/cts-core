package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// ==============================================================================
// AUDIT_LOG TESTS
// ==============================================================================

func TestAuditLogJSONMarshaling(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)

	audit := AuditLog{
		ID:           1,
		Timestamp:    now,
		UID:          sql.NullInt32{Int32: 42, Valid: true},
		Action:       string(AuditActionTraderCreate),
		ResourceType: sql.NullString{String: string(ResourceTypeTrader), Valid: true},
		ResourceID:   sql.NullString{String: "1", Valid: true},
		OldValue:     sql.NullString{Valid: false},
		NewValue:     sql.NullString{String: `{"status": "active", "name": "EU Trader 1"}`, Valid: true},
		IPAddress:    sql.NullString{String: "192.168.1.10", Valid: true},
		UserAgent:    sql.NullString{String: "Mozilla/5.0", Valid: true},
		Success:      true,
		ErrorMessage: sql.NullString{Valid: false},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("Failed to marshal AuditLog to JSON: %v", err)
	}

	// Unmarshal from JSON
	var decoded AuditLog
	err = json.Unmarshal(jsonData, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON to AuditLog: %v", err)
	}

	// Verify fields
	if decoded.ID != audit.ID {
		t.Errorf("Expected ID %d, got %d", audit.ID, decoded.ID)
	}
	if decoded.Action != audit.Action {
		t.Errorf("Expected Action %s, got %s", audit.Action, decoded.Action)
	}
	if decoded.Success != audit.Success {
		t.Errorf("Expected Success %v, got %v", audit.Success, decoded.Success)
	}
}

func TestAuditActionConstants(t *testing.T) {
	tests := []struct {
		action   AuditAction
		expected string
	}{
		// Trader Operations
		{AuditActionTraderCreate, "TRADER_CREATE"},
		{AuditActionTraderDelete, "TRADER_DELETE"},
		{AuditActionTraderSuspend, "TRADER_SUSPEND"},
		{AuditActionTraderResume, "TRADER_RESUME"},
		{AuditActionTraderUpdate, "TRADER_UPDATE"},
		// Monitor Operations
		{AuditActionMonitorAssign, "MONITOR_ASSIGN"},
		{AuditActionMonitorUnassign, "MONITOR_UNASSIGN"},
		{AuditActionMonitorPause, "MONITOR_PAUSE"},
		{AuditActionMonitorResume, "MONITOR_RESUME"},
		// User Operations
		{AuditActionUserCreate, "USER_CREATE"},
		{AuditActionUserDelete, "USER_DELETE"},
		{AuditActionUserUpdate, "USER_UPDATE"},
		{AuditActionUserLogin, "USER_LOGIN"},
		{AuditActionUserLogout, "USER_LOGOUT"},
		// Config Operations
		{AuditActionConfigUpdate, "CONFIG_UPDATE"},
		// HSM Operations
		{AuditActionKeyRotation, "KEY_ROTATION"},
		{AuditActionReencrypt, "REENCRYPTION_JOB"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			if string(tt.action) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.action))
			}
		})
	}
}

func TestResourceTypeConstants(t *testing.T) {
	tests := []struct {
		resourceType ResourceType
		expected     string
	}{
		{ResourceTypeTrader, "trader"},
		{ResourceTypeMonitor, "monitor"},
		{ResourceTypeUser, "user"},
		{ResourceTypeConfig, "config"},
		{ResourceTypeExchangeAccount, "exchange_account"},
		{ResourceTypeHSM, "hsm"},
		{ResourceTypeScheduler, "scheduler"},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			if string(tt.resourceType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.resourceType))
			}
		})
	}
}

func TestAuditLogWithUser(t *testing.T) {
	now := time.Now()

	// Admin action
	audit := AuditLog{
		ID:           1,
		Timestamp:    now,
		UID:          sql.NullInt32{Int32: 42, Valid: true},
		Action:       string(AuditActionTraderCreate),
		ResourceType: sql.NullString{String: string(ResourceTypeTrader), Valid: true},
		ResourceID:   sql.NullString{String: "1", Valid: true},
		Success:      true,
	}

	if !audit.UID.Valid {
		t.Error("Admin action should have valid UID")
	}
	if audit.UID.Int32 != 42 {
		t.Errorf("Expected UID 42, got %d", audit.UID.Int32)
	}
}

func TestAuditLogSystemAction(t *testing.T) {
	now := time.Now()

	// System action (no user)
	audit := AuditLog{
		ID:           1,
		Timestamp:    now,
		UID:          sql.NullInt32{Valid: false}, // NULL = system action
		Action:       string(AuditActionReencrypt),
		ResourceType: sql.NullString{String: string(ResourceTypeHSM), Valid: true},
		Success:      true,
	}

	if audit.UID.Valid {
		t.Error("System action should have NULL UID")
	}
}

func TestChangeTracking(t *testing.T) {
	oldValue := `{"status": "active", "max_tasks": 10}`
	newValue := `{"status": "suspended", "max_tasks": 0}`

	audit := AuditLog{
		Action:       string(AuditActionTraderSuspend),
		ResourceType: sql.NullString{String: string(ResourceTypeTrader), Valid: true},
		ResourceID:   sql.NullString{String: "1", Valid: true},
		OldValue:     sql.NullString{String: oldValue, Valid: true},
		NewValue:     sql.NullString{String: newValue, Valid: true},
		Success:      true,
	}

	if !audit.OldValue.Valid {
		t.Error("Change tracking should have valid OldValue")
	}
	if !audit.NewValue.Valid {
		t.Error("Change tracking should have valid NewValue")
	}

	// Verify JSON format
	var oldData, newData map[string]interface{}
	err := json.Unmarshal([]byte(audit.OldValue.String), &oldData)
	if err != nil {
		t.Errorf("OldValue should be valid JSON: %v", err)
	}
	err = json.Unmarshal([]byte(audit.NewValue.String), &newData)
	if err != nil {
		t.Errorf("NewValue should be valid JSON: %v", err)
	}
}

func TestIPAddressTracking(t *testing.T) {
	tests := []struct {
		name      string
		ipAddress string
	}{
		{"IPv4", "192.168.1.10"},
		{"IPv6", "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
		{"IPv6 Short", "2001:db8:85a3::8a2e:370:7334"},
		{"Localhost IPv4", "127.0.0.1"},
		{"Localhost IPv6", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audit := AuditLog{
				Action:    string(AuditActionUserLogin),
				IPAddress: sql.NullString{String: tt.ipAddress, Valid: true},
				Success:   true,
			}

			if !audit.IPAddress.Valid {
				t.Error("Audit log should have valid IP address")
			}
			if audit.IPAddress.String != tt.ipAddress {
				t.Errorf("Expected IP %s, got %s", tt.ipAddress, audit.IPAddress.String)
			}
		})
	}
}

func TestUserAgentTracking(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
	}{
		{"Chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"},
		{"Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"},
		{"Safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15"},
		{"API Client", "CTS-Admin-CLI/1.0"},
		{"curl", "curl/7.68.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audit := AuditLog{
				Action:    string(AuditActionConfigUpdate),
				UserAgent: sql.NullString{String: tt.userAgent, Valid: true},
				Success:   true,
			}

			if !audit.UserAgent.Valid {
				t.Error("Audit log should have valid User-Agent")
			}
			if audit.UserAgent.String != tt.userAgent {
				t.Errorf("Expected User-Agent %s, got %s", tt.userAgent, audit.UserAgent.String)
			}
		})
	}
}

func TestSuccessfulVsFailedActions(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		success  bool
		hasError bool
	}{
		{"Successful Action", true, false},
		{"Failed Action", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audit := AuditLog{
				Timestamp: now,
				Action:    string(AuditActionTraderDelete),
				Success:   tt.success,
			}

			if tt.hasError {
				audit.ErrorMessage = sql.NullString{String: "Trader not found", Valid: true}
			}

			if audit.Success != tt.success {
				t.Errorf("Expected Success %v, got %v", tt.success, audit.Success)
			}

			if tt.hasError && !audit.ErrorMessage.Valid {
				t.Error("Failed action should have error message")
			}
			if !tt.hasError && audit.ErrorMessage.Valid {
				t.Error("Successful action should not have error message")
			}
		})
	}
}

func TestCriticalOperationsAudit(t *testing.T) {
	now := time.Now()

	criticalOps := []struct {
		action       AuditAction
		resourceType ResourceType
		description  string
	}{
		{AuditActionTraderDelete, ResourceTypeTrader, "Deleting trader"},
		{AuditActionUserDelete, ResourceTypeUser, "Deleting user"},
		{AuditActionKeyRotation, ResourceTypeHSM, "Rotating HSM keys"},
		{AuditActionConfigUpdate, ResourceTypeConfig, "Updating system config"},
	}

	for _, op := range criticalOps {
		t.Run(op.description, func(t *testing.T) {
			audit := AuditLog{
				Timestamp:    now,
				UID:          sql.NullInt32{Int32: 1, Valid: true},
				Action:       string(op.action),
				ResourceType: sql.NullString{String: string(op.resourceType), Valid: true},
				Success:      true,
			}

			if audit.Action != string(op.action) {
				t.Errorf("Expected action %s, got %s", op.action, audit.Action)
			}
			if audit.ResourceType.String != string(op.resourceType) {
				t.Errorf("Expected resource type %s, got %s", op.resourceType, audit.ResourceType.String)
			}
		})
	}
}

func TestTimestampPrecision(t *testing.T) {
	// Test microsecond precision
	now := time.Now()

	audit := AuditLog{
		ID:        1,
		Timestamp: now,
		Action:    string(AuditActionUserLogin),
		Success:   true,
	}

	// Truncate to microseconds (MySQL TIMESTAMP(6))
	truncated := audit.Timestamp.Truncate(time.Microsecond)

	// Verify microsecond precision is preserved
	if audit.Timestamp.Before(truncated.Add(-time.Microsecond)) ||
		audit.Timestamp.After(truncated.Add(time.Microsecond)) {
		t.Error("Timestamp should preserve microsecond precision")
	}
}

func TestCompleteAuditTrail(t *testing.T) {
	now := time.Now()

	// Simulate complete audit trail for trader lifecycle
	auditTrail := []AuditLog{
		{
			Timestamp:    now,
			Action:       string(AuditActionTraderCreate),
			ResourceType: sql.NullString{String: string(ResourceTypeTrader), Valid: true},
			ResourceID:   sql.NullString{String: "1", Valid: true},
			NewValue:     sql.NullString{String: `{"name": "EU Trader", "status": "registered"}`, Valid: true},
			Success:      true,
		},
		{
			Timestamp:    now.Add(1 * time.Hour),
			Action:       string(AuditActionTraderUpdate),
			ResourceType: sql.NullString{String: string(ResourceTypeTrader), Valid: true},
			ResourceID:   sql.NullString{String: "1", Valid: true},
			OldValue:     sql.NullString{String: `{"status": "registered"}`, Valid: true},
			NewValue:     sql.NullString{String: `{"status": "active"}`, Valid: true},
			Success:      true,
		},
		{
			Timestamp:    now.Add(2 * time.Hour),
			Action:       string(AuditActionTraderSuspend),
			ResourceType: sql.NullString{String: string(ResourceTypeTrader), Valid: true},
			ResourceID:   sql.NullString{String: "1", Valid: true},
			OldValue:     sql.NullString{String: `{"status": "active"}`, Valid: true},
			NewValue:     sql.NullString{String: `{"status": "suspended"}`, Valid: true},
			Success:      true,
		},
	}

	// Verify trail is in chronological order
	for i := 1; i < len(auditTrail); i++ {
		if !auditTrail[i].Timestamp.After(auditTrail[i-1].Timestamp) {
			t.Error("Audit trail should be in chronological order")
		}
	}

	// Verify all actions relate to same resource
	resourceID := auditTrail[0].ResourceID.String
	for _, audit := range auditTrail {
		if audit.ResourceID.String != resourceID {
			t.Error("All audit entries should relate to same resource")
		}
	}
}

func TestJSONValueExtraction(t *testing.T) {
	jsonValue := `{"status": "active", "max_tasks": 10, "region": "eu"}`

	audit := AuditLog{
		NewValue: sql.NullString{String: jsonValue, Valid: true},
	}

	// Parse JSON
	var data map[string]interface{}
	err := json.Unmarshal([]byte(audit.NewValue.String), &data)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify JSON fields
	if data["status"] != "active" {
		t.Errorf("Expected status 'active', got %v", data["status"])
	}
	if data["max_tasks"] != float64(10) {
		t.Errorf("Expected max_tasks 10, got %v", data["max_tasks"])
	}
}

func TestUserGroupOperationsAudit(t *testing.T) {
	now := time.Now()

	tests := []struct {
		action       AuditAction
		resourceType ResourceType
	}{
		{AuditActionUserGroupCreate, ResourceTypeUserGroup},
		{AuditActionUserGroupUpdate, ResourceTypeUserGroup},
		{AuditActionUserGroupDelete, ResourceTypeUserGroup},
		{AuditActionUserGroupAssign, ResourceTypeUserGroupMember},
		{AuditActionUserGroupUnassign, ResourceTypeUserGroupMember},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			audit := AuditLog{
				Timestamp:    now,
				Action:       string(tt.action),
				ResourceType: sql.NullString{String: string(tt.resourceType), Valid: true},
				Success:      true,
			}

			if audit.Action != string(tt.action) {
				t.Errorf("Expected action %s, got %s", tt.action, audit.Action)
			}
		})
	}
}

func TestExchangeAccountAudit(t *testing.T) {
	now := time.Now()

	// Audit exchange account creation (sensitive!)
	audit := AuditLog{
		Timestamp:    now,
		UID:          sql.NullInt32{Int32: 1, Valid: true},
		Action:       string(AuditActionExchangeAccountCreate),
		ResourceType: sql.NullString{String: string(ResourceTypeExchangeAccount), Valid: true},
		ResourceID:   sql.NullString{String: "123", Valid: true},
		NewValue:     sql.NullString{String: `{"exchange": "Binance", "account_name": "Trading Account 1"}`, Valid: true},
		Success:      true,
	}

	// Verify sensitive data is NOT in NewValue
	var data map[string]interface{}
	json.Unmarshal([]byte(audit.NewValue.String), &data)

	// API keys should NOT be in audit log
	if _, exists := data["api_key"]; exists {
		t.Error("API key should not be in audit log")
	}
	if _, exists := data["api_secret"]; exists {
		t.Error("API secret should not be in audit log")
	}
}
