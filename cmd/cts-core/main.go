package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/titaev-lv/cts-core/internal/config"
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

	// TODO: Phase 1.2 - Initialize MySQL pool
	// TODO: Phase 1.3 - Initialize HSM client
	// TODO: Phase 1.4 - Load state
	// TODO: Phase 1.5 - Start REST server

	log.Info("CTS-Core initialized successfully")
	log.Info("CTS-Core is running. Press Ctrl+C to stop.")

	// Keep running
	select {}
}
