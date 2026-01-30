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
	if err := logger.Init(cfg.Logging.Level, cfg.Logging.Dir, cfg.Logging.MaxFileSizeMB); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Get main logger
	log := logger.Get("main")

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

	// TODO: Phase 1.3 - Initialize HSM client
	// TODO: Phase 1.4 - Load state
	// TODO: Phase 1.5 - Start REST server

	log.Info("CTS-Core initialized successfully")
	log.Info("CTS-Core is running. Press Ctrl+C to stop.")

	// Keep running
	select {}
}
