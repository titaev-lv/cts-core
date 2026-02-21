package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Use example config for testing
	cfg, err := Load("../../conf/config.example.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate loaded values
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Limits.MaxHeaderBytes != 1048576 {
		t.Errorf("Expected server.limits.max_header_bytes=1048576, got %d", cfg.Server.Limits.MaxHeaderBytes)
	}

	if cfg.MySQL.Database != "ct_system" {
		t.Errorf("Expected database=ct_system, got %s", cfg.MySQL.Database)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level=debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Dir != "logs" {
		t.Errorf("Expected log dir=logs, got %s", cfg.Logging.Dir)
	}
	if cfg.Logging.MaxSizeMB != 100 {
		t.Errorf("Expected max_size_mb=100, got %d", cfg.Logging.MaxSizeMB)
	}
	if cfg.Logging.MaxBackups != 10 {
		t.Errorf("Expected max_backups=10, got %d", cfg.Logging.MaxBackups)
	}
	if cfg.Logging.MaxAgeDays != 30 {
		t.Errorf("Expected max_age_days=30, got %d", cfg.Logging.MaxAgeDays)
	}
	if !cfg.Logging.Compress {
		t.Error("Expected compress=true")
	}
	if cfg.Logging.ErrorPath != "logs/error.log" {
		t.Errorf("Expected error_path=logs/error.log, got %s", cfg.Logging.ErrorPath)
	}
	if cfg.Logging.AccessPath != "logs/access.log" {
		t.Errorf("Expected access_path=logs/access.log, got %s", cfg.Logging.AccessPath)
	}
	if cfg.Logging.OutRequestPath != "logs/out_request.log" {
		t.Errorf("Expected out_request_path=logs/out_request.log, got %s", cfg.Logging.OutRequestPath)
	}
	if cfg.Logging.WSAccessPath != "logs/ws_access.log" {
		t.Errorf("Expected ws_access_path=logs/ws_access.log, got %s", cfg.Logging.WSAccessPath)
	}
	if cfg.Logging.WSOutPath != "logs/ws_out.log" {
		t.Errorf("Expected ws_out_path=logs/ws_out.log, got %s", cfg.Logging.WSOutPath)
	}
	if cfg.Logging.AuditPath != "logs/audit.log" {
		t.Errorf("Expected audit_path=logs/audit.log, got %s", cfg.Logging.AuditPath)
	}
	if !cfg.Logging.AccessToStdout {
		t.Error("Expected access_to_stdout=true")
	}
	if cfg.Logging.OutRequestToStdout {
		t.Error("Expected out_request_to_stdout=false")
	}
	if cfg.Logging.WSAccessToStdout {
		t.Error("Expected ws_access_to_stdout=false")
	}
	if cfg.Logging.WSOutToStdout {
		t.Error("Expected ws_out_to_stdout=false")
	}
	if cfg.Logging.AuditToStdout {
		t.Error("Expected audit_to_stdout=false")
	}
	if cfg.Logging.PostPayload {
		t.Error("Expected post_payload=false")
	}
	if cfg.Logging.PostPayloadMaxBytes != 2048 {
		t.Errorf("Expected post_payload_max_bytes=2048, got %d", cfg.Logging.PostPayloadMaxBytes)
	}
	if cfg.Logging.WSDebug {
		t.Error("Expected ws_debug=false")
	}

	// Test HSM dual-context configuration
	if cfg.HSM.URL != "https://hsm:8443" {
		t.Errorf("Expected HSM URL=https://hsm:8443, got %s", cfg.HSM.URL)
	}

	if cfg.HSM.Trading.Context != "exchange-key" {
		t.Errorf("Expected HSM Trading context=exchange-key, got %s", cfg.HSM.Trading.Context)
	}

	if cfg.HSM.Trading.TLS.CertPath != "pki/hsm-service/clients/cts-core-trader/cts-core-hsm-trader.crt" {
		t.Errorf("Expected Trading cert=pki/hsm-service/clients/cts-core-trader/cts-core-hsm-trader.crt, got %s", cfg.HSM.Trading.TLS.CertPath)
	}

	if cfg.HSM.TwoFA.Context != "2fa" {
		t.Errorf("Expected HSM 2FA context=2fa, got %s", cfg.HSM.TwoFA.Context)
	}

	if cfg.HSM.TwoFA.TLS.CertPath != "pki/hsm-service/clients/cts-core-2fa/cts-core-hsm-2fa.crt" {
		t.Errorf("Expected 2FA cert=pki/hsm-service/clients/cts-core-2fa/cts-core-hsm-2fa.crt, got %s", cfg.HSM.TwoFA.TLS.CertPath)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				Server: ServerConfig{
					Port: 8443,
					TLS: TLSConfig{
						Enabled:  true,
						CertPath: "pki/server/cts-core.crt",
						KeyPath:  "pki/server/cts-core.key",
					},
				},
				MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: false,
		},
		{
			name: "valid config with tls disabled and empty cert paths",
			cfg: Config{
				Server:  ServerConfig{Port: 8443, TLS: TLSConfig{Enabled: false}},
				MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: false,
		},
		{
			name: "invalid tls enabled without cert path",
			cfg: Config{
				Server: ServerConfig{Port: 8443, TLS: TLSConfig{
					Enabled: true,
					KeyPath: "pki/server/cts-core.key",
				}},
				MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: true,
		},
		{
			name: "invalid tls enabled without key path",
			cfg: Config{
				Server: ServerConfig{Port: 8443, TLS: TLSConfig{
					Enabled:  true,
					CertPath: "pki/server/cts-core.crt",
				}},
				MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			cfg: Config{
				Server:  ServerConfig{Port: 99999}, // Invalid
				MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			cfg: Config{
				Server:  ServerConfig{Port: 8443},
				MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "verbose", Dir: "logs"}, // Invalid
			},
			wantErr: true,
		},
		{
			name: "empty database",
			cfg: Config{
				Server:  ServerConfig{Port: 8443},
				MySQL:   MySQLConfig{Database: "", Port: 3306}, // Invalid
				State:   StateConfig{FilePath: "state/daemon.state"},
				Logging: LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("CTS_MYSQL_PASSWORD", "secret123")
	os.Setenv("CTS_LOG_LEVEL", "error")
	defer func() {
		os.Unsetenv("CTS_MYSQL_PASSWORD")
		os.Unsetenv("CTS_LOG_LEVEL")
	}()

	cfg := &Config{
		MySQL:   MySQLConfig{Password: "default"},
		Logging: LoggingConfig{Level: "debug"},
	}

	cfg.applyEnvOverrides()

	if cfg.MySQL.Password != "secret123" {
		t.Errorf("Expected password=secret123, got %s", cfg.MySQL.Password)
	}

	if cfg.Logging.Level != "error" {
		t.Errorf("Expected log level=error, got %s", cfg.Logging.Level)
	}
}

func TestValidateServerTimeoutDefaults(t *testing.T) {
	cfg := Config{
		Server:  ServerConfig{Port: 8443, TLS: TLSConfig{Enabled: false}},
		MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
		State:   StateConfig{FilePath: "state/daemon.state"},
		Logging: LoggingConfig{Level: "info", Dir: "logs"},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if cfg.Server.Timeouts.Read == 0 || cfg.Server.Timeouts.Write == 0 || cfg.Server.Timeouts.Idle == 0 || cfg.Server.Timeouts.ReadHeader == 0 || cfg.Server.Timeouts.ShutdownGrace == 0 {
		t.Fatalf("expected server timeout defaults to be populated, got %+v", cfg.Server.Timeouts)
	}
	if cfg.Server.Limits.MaxHeaderBytes == 0 {
		t.Fatalf("expected server.limits.max_header_bytes default to be populated")
	}
}

func TestValidateServerTimeoutBounds(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port: 8443,
			TLS:  TLSConfig{Enabled: false},
			Timeouts: TimeoutConfig{
				Read: -1 * time.Second,
			},
		},
		MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
		State:   StateConfig{FilePath: "state/daemon.state"},
		Logging: LoggingConfig{Level: "info", Dir: "logs"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to fail for negative server.timeouts.read")
	}
}

func TestValidateServerMaxHeaderBytesBounds(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port: 8443,
			TLS:  TLSConfig{Enabled: false},
			Limits: LimitsConfig{
				MaxHeaderBytes: 1024,
			},
		},
		MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
		State:   StateConfig{FilePath: "state/daemon.state"},
		Logging: LoggingConfig{Level: "info", Dir: "logs"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to fail for too small server.limits.max_header_bytes")
	}
}

func TestValidateServerHTTP2Invalid(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{
			Port: 8443,
			TLS:  TLSConfig{Enabled: false},
			HTTP2: &HTTP2Config{
				MaxFrameSize: "invalid",
			},
		},
		MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
		State:   StateConfig{FilePath: "state/daemon.state"},
		Logging: LoggingConfig{Level: "info", Dir: "logs"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to fail for invalid server.http2 settings")
	}
}

func TestValidateRateLimitDefaultsAndAlias(t *testing.T) {
	cfg := Config{
		Server:  ServerConfig{Port: 8443, TLS: TLSConfig{Enabled: false}},
		MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
		State:   StateConfig{FilePath: "state/daemon.state"},
		Logging: LoggingConfig{Level: "info", Dir: "logs"},
		RateLimit: RateLimitConfig{
			WebSocket: LimitConfig{MessagesPerMinute: 777, Burst: 10},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if cfg.RateLimit.REST.PerMinute() == 0 {
		t.Fatal("expected rate_limit.rest default to be set")
	}
	if cfg.RateLimit.WebSocket.PerMinute() != 777 {
		t.Fatalf("expected websocket alias messages_per_minute=777, got %d", cfg.RateLimit.WebSocket.PerMinute())
	}
}

func TestValidateRateLimitInvalid(t *testing.T) {
	cfg := Config{
		Server:  ServerConfig{Port: 8443, TLS: TLSConfig{Enabled: false}},
		MySQL:   MySQLConfig{Database: "ct_system", Port: 3306},
		State:   StateConfig{FilePath: "state/daemon.state"},
		Logging: LoggingConfig{Level: "info", Dir: "logs"},
		RateLimit: RateLimitConfig{
			REST: LimitConfig{RequestsPerMinute: -1, Burst: 0},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate() to fail for invalid rate_limit.rest.requests_per_minute")
	}
}

func TestHSMDualContext(t *testing.T) {
	// Load config with HSM dual-context configuration
	cfg, err := Load("../../conf/config.example.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test base HSM configuration
	if cfg.HSM.URL == "" {
		t.Error("HSM URL should not be empty")
	}

	if cfg.HSM.Timeout == 0 {
		t.Error("HSM timeout should not be zero")
	}

	// Test Trading context
	t.Run("Trading Context", func(t *testing.T) {
		if cfg.HSM.Trading.Context == "" {
			t.Error("Trading context should not be empty")
		}

		if cfg.HSM.Trading.Context != "exchange-key" {
			t.Errorf("Expected Trading context=exchange-key, got %s", cfg.HSM.Trading.Context)
		}

		if !cfg.HSM.Trading.TLS.Enabled {
			t.Error("Trading TLS should be enabled")
		}

		if cfg.HSM.Trading.TLS.CertPath == "" {
			t.Error("Trading cert file should not be empty")
		}

		if cfg.HSM.Trading.TLS.KeyPath == "" {
			t.Error("Trading key file should not be empty")
		}

		if cfg.HSM.Trading.TLS.CAPath == "" {
			t.Error("Trading CA file should not be empty")
		}
	})

	// Test 2FA context
	t.Run("2FA Context", func(t *testing.T) {
		if cfg.HSM.TwoFA.Context == "" {
			t.Error("2FA context should not be empty")
		}

		if cfg.HSM.TwoFA.Context != "2fa" {
			t.Errorf("Expected 2FA context=2fa, got %s", cfg.HSM.TwoFA.Context)
		}

		if !cfg.HSM.TwoFA.TLS.Enabled {
			t.Error("2FA TLS should be enabled")
		}

		if cfg.HSM.TwoFA.TLS.CertPath == "" {
			t.Error("2FA cert file should not be empty")
		}

		if cfg.HSM.TwoFA.TLS.KeyPath == "" {
			t.Error("2FA key file should not be empty")
		}

		if cfg.HSM.TwoFA.TLS.CAPath == "" {
			t.Error("2FA CA file should not be empty")
		}
	})

	// Test that contexts are different
	t.Run("Context Isolation", func(t *testing.T) {
		if cfg.HSM.Trading.Context == cfg.HSM.TwoFA.Context {
			t.Error("Trading and 2FA contexts should be different")
		}

		if cfg.HSM.Trading.TLS.CertPath == cfg.HSM.TwoFA.TLS.CertPath {
			t.Error("Trading and 2FA should use different certificates")
		}

		if cfg.HSM.Trading.TLS.KeyPath == cfg.HSM.TwoFA.TLS.KeyPath {
			t.Error("Trading and 2FA should use different keys")
		}
	})

	// Test retry configuration
	t.Run("Retry Config", func(t *testing.T) {
		if cfg.HSM.Retry.MaxAttempts == 0 {
			t.Error("HSM retry max attempts should not be zero")
		}

		if cfg.HSM.Retry.InitialDelay == 0 {
			t.Error("HSM retry initial delay should not be zero")
		}

		if cfg.HSM.Retry.MaxDelay == 0 {
			t.Error("HSM retry max delay should not be zero")
		}

		if cfg.HSM.Retry.Multiplier == 0 {
			t.Error("HSM retry multiplier should not be zero")
		}
	})
}
