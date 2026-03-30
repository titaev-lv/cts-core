package models

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

func TestTrader_JSONMarshaling(t *testing.T) {
	trader := Trader{
		ID:            1,
		TraderName:    "EU Frankfurt Trader",
		CertificateCN: "trader-eu-1.cts.internal",
		Region:        sql.NullString{String: "eu", Valid: true},
		Status:        TraderStatusActive,
		MaxTasks:      10,
		DateCreate:    time.Now(),
		DateModify:    time.Now(),
	}

	data, err := json.Marshal(trader)
	if err != nil {
		t.Fatalf("Failed to marshal Trader: %v", err)
	}

	var decoded Trader
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Trader: %v", err)
	}

	if decoded.ID != trader.ID {
		t.Errorf("Expected ID %d, got %d", trader.ID, decoded.ID)
	}
	if decoded.TraderName != trader.TraderName {
		t.Errorf("Expected TraderName %s, got %s", trader.TraderName, decoded.TraderName)
	}
}

func TestTraderStatus_Constants(t *testing.T) {
	statuses := []TraderStatus{
		TraderStatusPending,
		TraderStatusActive,
		TraderStatusSuspended,
		TraderStatusDecommissioned,
	}

	for _, status := range statuses {
		if len(string(status)) == 0 {
			t.Errorf("Status constant is empty")
		}
	}
}

func TestTraderSession_JSONMarshaling(t *testing.T) {
	now := time.Now()
	session := TraderSession{
		ID:            123,
		TraderID:      1,
		SessionID:     "550e8400-e29b-41d4-a716-446655440000",
		ConnectedAt:   now,
		LastHeartbeat: now,
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Failed to marshal TraderSession: %v", err)
	}

	var decoded TraderSession
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal TraderSession: %v", err)
	}

	if decoded.ID != session.ID {
		t.Errorf("Expected ID %d, got %d", session.ID, decoded.ID)
	}
}

func TestDisconnectReason_Constants(t *testing.T) {
	reasons := []DisconnectReason{
		DisconnectGraceful,
		DisconnectTimeout,
		DisconnectError,
		DisconnectServerShutdown,
		DisconnectKicked,
	}

	for _, reason := range reasons {
		if len(string(reason)) == 0 {
			t.Errorf("DisconnectReason constant is empty")
		}
	}
}
