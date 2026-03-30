package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	"github.com/titaev-lv/cts-core/internal/api/rest"
	"github.com/titaev-lv/cts-core/internal/config"
	"github.com/titaev-lv/cts-core/internal/db"
	"github.com/titaev-lv/cts-core/internal/db/repository"
	"github.com/titaev-lv/cts-core/internal/hsm"
	"github.com/titaev-lv/cts-core/internal/logger"
	"github.com/titaev-lv/cts-core/internal/scheduler"
	"github.com/titaev-lv/cts-core/internal/state"
	"golang.org/x/net/http2"
)

const version = "0.0.1"

type schedulerRequirementsProvider struct {
	repo repository.ExchangeRequirementsRepository
}

type schedulerResourceProvider struct {
	repo repository.TraderResourceRepository
}

func (p schedulerRequirementsProvider) GetTradeRequiredExchanges(ctx context.Context) ([]scheduler.ExchangeRef, error) {
	items, err := p.repo.ListTradeExchanges(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.ExchangeRef, 0, len(items))
	for _, item := range items {
		out = append(out, scheduler.ExchangeRef{ExchangeID: item.ExchangeID, ExchangeName: item.ExchangeName})
	}
	return out, nil
}

func (p schedulerRequirementsProvider) GetMonitorRequiredExchanges(ctx context.Context) ([]scheduler.ExchangeRef, error) {
	items, err := p.repo.ListMonitorExchanges(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]scheduler.ExchangeRef, 0, len(items))
	for _, item := range items {
		out = append(out, scheduler.ExchangeRef{ExchangeID: item.ExchangeID, ExchangeName: item.ExchangeName})
	}
	return out, nil
}

func (p schedulerResourceProvider) GetTraderExchangeUtilization(ctx context.Context, traderDBID int, exchangeID int) (float64, bool, error) {
	items, err := p.repo.GetByTraderAndExchange(ctx, traderDBID, exchangeID, nil)
	if err != nil {
		return 0, false, err
	}
	if len(items) == 0 {
		return 0, false, nil
	}

	now := time.Now().UTC()
	maxUtil := 0.0
	found := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if !item.ResetAt.IsZero() && now.After(item.ResetAt) {
			// Window reset - resource is currently fully available.
			found = true
			continue
		}
		used, err := strconv.ParseFloat(item.UsedValue, 64)
		if err != nil {
			continue
		}
		max, err := strconv.ParseFloat(item.MaxValue, 64)
		if err != nil || max <= 0 {
			continue
		}
		util := used / max
		if util < 0 {
			util = 0
		}
		if util > 1 {
			util = 1
		}
		if util > maxUtil {
			maxUtil = util
		}
		found = true
	}

	if !found {
		return 0, false, nil
	}
	return maxUtil, true, nil
}

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
	if err := logger.InitWithOptions(logger.Options{
		Level:              cfg.Logging.Level,
		Format:             cfg.Logging.Format,
		Dir:                cfg.Logging.Dir,
		MaxFileSizeMB:      cfg.Logging.MaxSizeMB,
		MaxBackups:         cfg.Logging.MaxBackups,
		MaxAgeDays:         cfg.Logging.MaxAgeDays,
		Compress:           cfg.Logging.Compress,
		ErrorPath:          cfg.Logging.ErrorPath,
		AccessPath:         cfg.Logging.AccessPath,
		OutRequestPath:     cfg.Logging.OutRequestPath,
		WSInPath:           cfg.Logging.WSInPath,
		WSOutPath:          cfg.Logging.WSOutPath,
		AuditPath:          cfg.Logging.AuditPath,
		AccessToStdout:     cfg.Logging.AccessToStdout,
		OutRequestToStdout: cfg.Logging.OutRequestToStdout,
		WSInToStdout:       cfg.Logging.WSInToStdout,
		WSOutToStdout:      cfg.Logging.WSOutToStdout,
		AuditToStdout:      cfg.Logging.AuditToStdout,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Get main logger
	log := logger.Get("main")
	log.Debug("Logger configured", "level", cfg.Logging.Level, "dir", cfg.Logging.Dir)
	log.Debug("Loaded configuration", "path", *configPath)

	log.Info("CTS-Core starting",
		"version", version,
		"log_level", cfg.Logging.Level)

	startedAt := time.Now().UTC()

	// Phase 1.2 - Initialize MySQL pool
	mysqlCfg := db.MySQLConfig{
		Host:            cfg.Databases.System.MySQL.Host,
		Port:            cfg.Databases.System.MySQL.Port,
		User:            cfg.Databases.System.MySQL.User,
		Password:        cfg.Databases.System.MySQL.Password,
		Database:        cfg.Databases.System.MySQL.Database,
		TLSEnabled:      cfg.Databases.System.MySQL.TLS.Enabled,
		CertPath:        cfg.Databases.System.MySQL.TLS.CertPath,
		KeyPath:         cfg.Databases.System.MySQL.TLS.KeyPath,
		CAPath:          cfg.Databases.System.MySQL.TLS.CAPath,
		MaxOpenConns:    cfg.Databases.System.MySQL.Pool.MaxOpenConns,
		MaxIdleConns:    cfg.Databases.System.MySQL.Pool.MaxIdleConns,
		ConnMaxLifetime: cfg.Databases.System.MySQL.Pool.ConnMaxLifetime,
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
		BaseURL:          cfg.HSM.URL,
		CertPath:         cfg.HSM.Trading.TLS.CertPath,
		KeyPath:          cfg.HSM.Trading.TLS.KeyPath,
		CAPath:           cfg.HSM.Trading.TLS.CAPath,
		RequestTimeout:   cfg.HSM.Timeout,
		OutRequestLogger: logger.GetOutRequest("hsm"),
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
		BaseURL:          cfg.HSM.URL,
		CertPath:         cfg.HSM.TwoFA.TLS.CertPath,
		KeyPath:          cfg.HSM.TwoFA.TLS.KeyPath,
		CAPath:           cfg.HSM.TwoFA.TLS.CAPath,
		RequestTimeout:   cfg.HSM.Timeout,
		OutRequestLogger: logger.GetOutRequest("hsm"),
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

	stateManager, err := state.NewManager(state.ManagerConfig{
		StateFile:    cfg.State.FilePath,
		BackupDir:    filepath.Join(filepath.Dir(cfg.State.FilePath), "backups"),
		MaxBackups:   cfg.State.BackupCount,
		SyncInterval: cfg.State.SyncInterval,
	}, logger.Get("state"))
	if err != nil {
		log.Error("Failed to initialize state manager", "error", err)
		os.Exit(1)
	}

	if err := stateManager.Load(); err != nil {
		log.Error("Failed to load state", "error", err)
		os.Exit(1)
	}
	stateManager.SetServerStatus("running")
	stateManager.StartBackgroundSync()

	router, wsHandler := rest.NewRouter(dbClient, rest.Options{
		RESTRequestsPerSecond: cfg.RateLimit.REST.PerSecond(),
		RESTBurst:             cfg.RateLimit.REST.Burst,
		WSRequestsPerSecond:   cfg.RateLimit.WebSocket.PerSecond(),
		WSBurst:               cfg.RateLimit.WebSocket.Burst,
		WSHeartbeatInterval:   cfg.Session.HeartbeatInterval,
		WSHeartbeatTimeout:    cfg.Session.HeartbeatTimeout,
		WSWriteTimeout:        cfg.Session.WriteTimeout,
		WSMaxPayloadBytes:     cfg.Session.MaxPayloadBytes,
		WSMaxMessagesPerSec:   cfg.RateLimit.WebSocket.PerSecond(),
		WSMaxUnknownActions:   cfg.Session.MaxUnknownActions,
		WSUnknownActionWindow: cfg.Session.UnknownActionWindow,
		WSRequestDedupWindow:  cfg.Session.RequestDedupWindow,
		WSAllowedCommonNames:  cfg.Server.TLS.AllowedClientCommonNames,
		WSAllowedOUs:          cfg.Server.TLS.AllowedClientOrganizationalOUs,
		WSAllowedDNSNames:     cfg.Server.TLS.AllowedClientDNSNames,
		HSMTrading:            hsmTradingClient,
		HSMTwoFA:              hsm2FAClient,
		StateManager:          stateManager,
		StartedAt:             startedAt,
		ServiceVersion:        version,
		MetricsEnabled:        cfg.Metrics.Enabled,
		MetricsPath:           cfg.Metrics.Path,
	})

	schedulerEngine := scheduler.NewEngine(
		scheduler.Config{
			Interval:                  cfg.Scheduler.TaskAssignmentInterval,
			LatencyCheckInterval:      cfg.Scheduler.LatencyCheckInterval,
			HealthyWindow:             cfg.Session.HeartbeatTimeout,
			MetricsSink:               stateManager,
			RequiredExchangesProvider: schedulerRequirementsProvider{repo: repo.ExchangeRequirements()},
			LatencyDispatcher:         wsHandler,
			ResourceProvider:          schedulerResourceProvider{repo: repo.TraderResource()},
			ResourceHardLimit:         cfg.Scheduler.ResourceHardLimit,
			ResourceSoftLimit:         cfg.Scheduler.ResourceSoftLimit,
			ResourceSoftPenaltyMs:     cfg.Scheduler.ResourceSoftPenaltyMs,
		},
		wsHandler,
		nil,
		logger.Get("scheduler"),
	)
	schedulerEngine.Start()
	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadTimeout:       cfg.Server.Timeouts.Read,
		WriteTimeout:      cfg.Server.Timeouts.Write,
		IdleTimeout:       cfg.Server.Timeouts.Idle,
		ReadHeaderTimeout: cfg.Server.Timeouts.ReadHeader,
		MaxHeaderBytes:    cfg.Server.Limits.MaxHeaderBytes,
	}

	closeAllWS := func(reason string) {
		if wsHandler == nil {
			return
		}
		wsHandler.CloseAll(websocket.CloseNormalClosure, reason)
	}

	caBytes, readErr := os.ReadFile(cfg.Server.TLS.CAPath)
	if readErr != nil {
		log.Error("Failed to read WS client CA file", "path", cfg.Server.TLS.CAPath, "error", readErr)
		os.Exit(1)
	}

	clientCAs := x509.NewCertPool()
	if ok := clientCAs.AppendCertsFromPEM(caBytes); !ok {
		log.Error("Failed to parse WS client CA file", "path", cfg.Server.TLS.CAPath)
		os.Exit(1)
	}

	httpServer.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: tls.VerifyClientCertIfGiven,
		ClientCAs:  clientCAs,
	}
	log.Info("WS client certificate verification enabled", "ca_path", cfg.Server.TLS.CAPath)

	if cfg.Server.HTTP2 != nil {
		parsed, err := cfg.Server.HTTP2.Parse()
		if err != nil {
			log.Error("Invalid server.http2 config", "error", err)
			os.Exit(1)
		}

		h2Config := &http2.Server{
			MaxConcurrentStreams:         parsed.MaxConcurrentStreams,
			MaxReadFrameSize:             parsed.MaxFrameSize,
			IdleTimeout:                  time.Duration(parsed.IdleTimeoutSeconds) * time.Second,
			MaxUploadBufferPerConnection: parsed.MaxUploadBufferPerConn,
			MaxUploadBufferPerStream:     parsed.MaxUploadBufferPerStream,
		}
		if err := http2.ConfigureServer(httpServer, h2Config); err != nil {
			log.Error("Failed to configure HTTP/2 server", "error", err)
			os.Exit(1)
		}
	}

	log.Info("CTS-Core initialized successfully")
	log.Info("REST server starting", "addr", addr, "tls_enabled", true)

	serverErrCh := make(chan error, 1)
	go func() {
		serveErr := httpServer.ListenAndServeTLS(cfg.Server.TLS.CertPath, cfg.Server.TLS.KeyPath)
		serverErrCh <- serveErr
	}()

	shutdownCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-shutdownCtx.Done():
		log.Info("Shutdown signal received")
		schedulerEngine.Stop()
		stateManager.SetServerStatus("stopping")
		closeAllWS("server_shutdown")
	case serveErr := <-serverErrCh:
		schedulerEngine.Stop()
		closeAllWS("server_error")
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Error("REST server stopped with error", "error", serveErr)
			if closeErr := stateManager.Close(); closeErr != nil {
				log.Error("State manager close failed", "error", closeErr)
			}
			os.Exit(1)
		}
		if closeErr := stateManager.Close(); closeErr != nil {
			log.Error("State manager close failed", "error", closeErr)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.Timeouts.ShutdownGrace)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("REST server shutdown failed", "error", err)
		schedulerEngine.Stop()
		if closeErr := stateManager.Close(); closeErr != nil {
			log.Error("State manager close failed", "error", closeErr)
		}
		os.Exit(1)
	}

	stateManager.SetServerStatus("stopped")
	if closeErr := stateManager.Close(); closeErr != nil {
		log.Error("State manager close failed", "error", closeErr)
		os.Exit(1)
	}

	if serveErr := <-serverErrCh; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Error("REST server stopped with error", "error", serveErr)
		os.Exit(1)
	}

	log.Info("REST server stopped")
}
