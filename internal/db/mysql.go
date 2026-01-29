package db

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

type MySQLConfig struct {
	Host       string
	Port       int
	User       string
	Password   string
	Database   string
	TLSEnabled bool
	CertPath   string
	KeyPath    string
	CAPath     string

	// Connection Pool
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

type MySQLClient struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewMySQLClient creates new MySQL client with optional mTLS
func NewMySQLClient(cfg MySQLConfig, logger *slog.Logger) (*MySQLClient, error) {
	var dsn string

	if cfg.TLSEnabled {
		// Load client certificate
		cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}

		// Load CA certificate
		caCert, err := os.ReadFile(cfg.CAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA cert")
		}

		// Configure TLS
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
			ServerName:   cfg.Host, // Important for certificate validation
		}

		// Register TLS config
		err = mysql.RegisterTLSConfig("custom", tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to register TLS config: %w", err)
		}

		// Build DSN with mTLS
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?tls=custom&parseTime=true&loc=UTC",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Database,
		)
	} else {
		// Build DSN without TLS
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=UTC",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Database,
		)
	}

	// Open connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)       // Max 25 connections
	db.SetMaxIdleConns(cfg.MaxIdleConns)       // Keep 10 idle
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime) // 5 minutes
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime) // 2 minutes

	// Create client for retry logic
	client := &MySQLClient{
		db:     db,
		logger: logger,
	}

	// Test connection with retry
	ctx := context.Background()
	err = client.WithRetry(ctx, func() error {
		return db.Ping()
	})

	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("MySQL connected", "host", cfg.Host, "port", cfg.Port, "mtls", cfg.TLSEnabled)

	return client, nil
}

// Close closes database connection
func (c *MySQLClient) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// DB returns underlying *sql.DB for advanced usage
func (c *MySQLClient) DB() *sql.DB {
	return c.db
}

// Ping checks if connection is alive
func (c *MySQLClient) Ping() error {
	return c.db.Ping()
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 100 * time.Millisecond,
		MaxWait:     5 * time.Second,
		Multiplier:  2.0,
	}
}

// WithRetry executes function with exponential backoff retry
func (c *MySQLClient) WithRetry(ctx context.Context, operation func() error) error {
	cfg := DefaultRetryConfig()
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Execute operation
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if context is cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// Don't retry on last attempt
		if attempt == cfg.MaxAttempts {
			break
		}

		// Calculate backoff delay
		wait := time.Duration(float64(cfg.InitialWait) * math.Pow(cfg.Multiplier, float64(attempt-1)))
		if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}

		c.logger.Warn("Operation failed, retrying",
			"error", err,
			"attempt", attempt,
			"retry_in", wait)

		// Wait before retry
		select {
		case <-time.After(wait):
			// Continue to next attempt
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}
