package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/config"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/db/repository"
	"github.com/titaev-lv/cts-core/internal/hsm"
	"github.com/titaev-lv/cts-core/internal/logger"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "conf/config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(
		cfg.Logging.Level,
		cfg.Logging.Dir,
		cfg.Logging.MaxFileSizeMB,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAgeDays,
		cfg.Logging.Compress,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Get main logger
	log := logger.Get("main")
	log.Debug("Logger configured", "level", cfg.Logging.Level, "dir", cfg.Logging.Dir)
	log.Debug("Loaded configuration", "path", *configPath, "environment", cfg.Environment)

	log.Info("CTS-Core starting",
		"environment", cfg.Environment,
		"version", "0.0.1",
		"log_level", cfg.Logging.Level)

	// Phase 1.2 - Initialize MySQL pool
	mysqlCfg := db.MySQLConfig{
		Host:            cfg.MySQL.Host,
		Port:            cfg.MySQL.Port,
		User:            cfg.MySQL.User,
		Password:        cfg.MySQL.Password,
		Database:        cfg.MySQL.Database,
		TLSEnabled:      cfg.MySQL.TLS.Enabled,
		CertPath:        cfg.MySQL.TLS.CertFile,
		KeyPath:         cfg.MySQL.TLS.KeyFile,
		CAPath:          cfg.MySQL.TLS.CAFile,
		MaxOpenConns:    cfg.MySQL.Pool.MaxOpenConns,
		MaxIdleConns:    cfg.MySQL.Pool.MaxIdleConns,
		ConnMaxLifetime: cfg.MySQL.Pool.ConnMaxLifetime,
		ConnMaxIdleTime: 2 * time.Minute, // Fixed value for now
	}
	log.Debug("MySQL config", "host", mysqlCfg.Host, "port", mysqlCfg.Port, "database", mysqlCfg.Database, "tls", mysqlCfg.TLSEnabled)

	dbLogger := logger.Get("database")
	dbClient, err := db.NewMySQLClient(mysqlCfg, dbLogger)
	if err != nil {
		log.Error("Failed to connect to MySQL", "error", err)
		os.Exit(1)
	}
	defer dbClient.Close()

	log.Info("MySQL connection established")

	// Phase 1.2.4 - Initialize Repository
	sqlxDB := sqlx.NewDb(dbClient.DB(), "mysql")
	repo := repository.New(sqlxDB)

	// Test database connection with repository
	ctx := context.Background()
	traders, err := repo.Trader().List(ctx, nil)
	if err != nil {
		log.Warn("Failed to list traders (database may be empty)", "error", err)
	} else {
		log.Info("Found traders", "count", len(traders))
	}

	// Phase 1.3 - Initialize HSM clients (Trading + 2FA contexts)
	hsmLogger := logger.Get("hsm")

	// 1. Trading context (exchange API keys)
	hsmTradingCfg := hsm.ClientConfig{
		BaseURL:        cfg.HSM.URL,
		CertPath:       cfg.HSM.Trading.TLS.CertFile,
		KeyPath:        cfg.HSM.Trading.TLS.KeyFile,
		CAPath:         cfg.HSM.Trading.TLS.CAFile,
		RequestTimeout: cfg.HSM.Timeout,
		RetryConfig: hsm.RetryConfig{
			MaxAttempts: cfg.HSM.Retry.MaxAttempts,
			InitialWait: cfg.HSM.Retry.InitialDelay,
			MaxWait:     cfg.HSM.Retry.MaxDelay,
			Multiplier:  cfg.HSM.Retry.Multiplier,
		},
	}
	log.Debug("HSM trading config", "url", hsmTradingCfg.BaseURL, "context", cfg.HSM.Trading.Context, "timeout", hsmTradingCfg.RequestTimeout)

	hsmTradingClient, err := hsm.NewClient(hsmTradingCfg, hsmLogger)
	if err != nil {
		log.Error("Failed to create HSM Trading client", "error", err)
		os.Exit(1)
	}
	defer hsmTradingClient.Close()

	// Test Trading context
	testPlaintext := []byte("binance_api_key=test123")
	keyID, ciphertext, err := hsmTradingClient.Encrypt(ctx, cfg.HSM.Trading.Context, testPlaintext)
	if err != nil {
		log.Warn("HSM Trading encrypt test failed (HSM service may be unavailable)", "error", err, "context", cfg.HSM.Trading.Context)
	} else {
		log.Info("HSM Trading encrypt successful", "key_id", keyID, "context", cfg.HSM.Trading.Context)

		// Test decrypt
		decrypted, err := hsmTradingClient.Decrypt(ctx, cfg.HSM.Trading.Context, keyID, ciphertext)
		if err != nil {
			log.Warn("HSM Trading decrypt test failed", "error", err)
		} else if string(decrypted) == string(testPlaintext) {
			log.Info("HSM Trading decrypt successful - round-trip verified")
		} else {
			log.Warn("HSM Trading decrypt mismatch", "expected", string(testPlaintext), "got", string(decrypted))
		}
	}

	// 2. 2FA context (for re-encryption job only, not used in normal operation)
	hsm2FACfg := hsm.ClientConfig{
		BaseURL:        cfg.HSM.URL,
		CertPath:       cfg.HSM.TwoFA.TLS.CertFile,
		KeyPath:        cfg.HSM.TwoFA.TLS.KeyFile,
		CAPath:         cfg.HSM.TwoFA.TLS.CAFile,
		RequestTimeout: cfg.HSM.Timeout,
		RetryConfig: hsm.RetryConfig{
			MaxAttempts: cfg.HSM.Retry.MaxAttempts,
			InitialWait: cfg.HSM.Retry.InitialDelay,
			MaxWait:     cfg.HSM.Retry.MaxDelay,
			Multiplier:  cfg.HSM.Retry.Multiplier,
		},
	}
	log.Debug("HSM 2FA config", "url", hsm2FACfg.BaseURL, "context", cfg.HSM.TwoFA.Context, "timeout", hsm2FACfg.RequestTimeout)

	hsm2FAClient, err := hsm.NewClient(hsm2FACfg, hsmLogger)
	if err != nil {
		log.Error("Failed to create HSM 2FA client", "error", err)
		os.Exit(1)
	}
	defer hsm2FAClient.Close()

	// Test 2FA context
	test2FAPlaintext := []byte("totp_secret=JBSWY3DPEHPK3PXP")
	keyID2FA, ciphertext2FA, err := hsm2FAClient.Encrypt(ctx, cfg.HSM.TwoFA.Context, test2FAPlaintext)
	if err != nil {
		log.Warn("HSM 2FA encrypt test failed (HSM service may be unavailable)", "error", err, "context", cfg.HSM.TwoFA.Context)
	} else {
		log.Info("HSM 2FA encrypt successful", "key_id", keyID2FA, "context", cfg.HSM.TwoFA.Context)

		// Test decrypt
		decrypted2FA, err := hsm2FAClient.Decrypt(ctx, cfg.HSM.TwoFA.Context, keyID2FA, ciphertext2FA)
		if err != nil {
			log.Warn("HSM 2FA decrypt test failed", "error", err)
		} else if string(decrypted2FA) == string(test2FAPlaintext) {
			log.Info("HSM 2FA decrypt successful - round-trip verified")
		} else {
			log.Warn("HSM 2FA decrypt mismatch", "expected", string(test2FAPlaintext), "got", string(decrypted2FA))
		}
	}

	log.Info("HSM clients initialized", "trading_context", cfg.HSM.Trading.Context, "2fa_context", cfg.HSM.TwoFA.Context)

	// TODO: Phase 1.4 - Load state
	// TODO: Phase 1.5 - Start REST server

	log.Info("CTS-Core initialized successfully")
	log.Info("CTS-Core is running. Press Ctrl+C to stop.")

	// Keep running
	select {}
}
