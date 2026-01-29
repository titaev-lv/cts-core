package db

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestNewMySQLClient_NoTLS(t *testing.T) {
	// This test requires MySQL running on localhost
	// Skip if MYSQL_HOST is not set
	host := os.Getenv("MYSQL_HOST")
	if host == "" {
		t.Skip("Skipping MySQL test: MYSQL_HOST not set")
	}

	cfg := MySQLConfig{
		Host:            host,
		Port:            3306,
		User:            os.Getenv("MYSQL_USER"),
		Password:        os.Getenv("MYSQL_PASSWORD"),
		Database:        os.Getenv("MYSQL_DATABASE"),
		TLSEnabled:      false,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 1 * time.Minute,
		ConnMaxIdleTime: 30 * time.Second,
	}

	logger := slog.Default()

	client, err := NewMySQLClient(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create MySQL client: %v", err)
	}
	defer client.Close()

	// Test connection
	if err := client.Ping(); err != nil {
		t.Fatalf("Failed to ping MySQL: %v", err)
	}

	t.Log("MySQL connection successful (no TLS)")
}

func TestNewMySQLClient_WithRetry(t *testing.T) {
	// Test retry logic with invalid config (should fail fast)
	cfg := MySQLConfig{
		Host:            "invalid-host-12345",
		Port:            3306,
		User:            "test",
		Password:        "test",
		Database:        "test",
		TLSEnabled:      false,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 1 * time.Minute,
		ConnMaxIdleTime: 30 * time.Second,
	}

	logger := slog.Default()

	start := time.Now()
	_, err := NewMySQLClient(cfg, logger)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected error for invalid host, got nil")
	}

	// Should retry and fail (at least 100ms for first retry)
	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected retry delay, but completed too fast: %v", elapsed)
	}

	t.Logf("Retry logic worked correctly, failed after %v: %v", elapsed, err)
}

func TestMySQLClient_RetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", cfg.MaxAttempts)
	}

	if cfg.InitialWait != 100*time.Millisecond {
		t.Errorf("Expected InitialWait=100ms, got %v", cfg.InitialWait)
	}

	if cfg.MaxWait != 5*time.Second {
		t.Errorf("Expected MaxWait=5s, got %v", cfg.MaxWait)
	}

	if cfg.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier=2.0, got %f", cfg.Multiplier)
	}
}
