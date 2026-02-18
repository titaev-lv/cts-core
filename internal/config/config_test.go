package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Use example config for testing
	cfg, err := Load("../../conf/config.example.yaml")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate loaded values
	if cfg.Environment != "development" {
		t.Errorf("Expected environment=development, got %s", cfg.Environment)
	}

	if cfg.Server.Port != 8443 {
		t.Errorf("Expected port=8443, got %d", cfg.Server.Port)
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
	if cfg.Logging.MaxFileSizeMB != 100 {
		t.Errorf("Expected max_file_size_mb=100, got %d", cfg.Logging.MaxFileSizeMB)
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

	// Test HSM dual-context configuration
	if cfg.HSM.URL != "https://192.168.50.4:8443" {
		t.Errorf("Expected HSM URL=https://192.168.50.4:8443, got %s", cfg.HSM.URL)
	}

	if cfg.HSM.Trading.Context != "exchange-key" {
		t.Errorf("Expected HSM Trading context=exchange-key, got %s", cfg.HSM.Trading.Context)
	}

	if cfg.HSM.Trading.TLS.CertFile != "pki/client/hsm-trading-client-1.crt" {
		t.Errorf("Expected Trading cert=hsm-trading-client-1.crt, got %s", cfg.HSM.Trading.TLS.CertFile)
	}

	if cfg.HSM.TwoFA.Context != "2fa" {
		t.Errorf("Expected HSM 2FA context=2fa, got %s", cfg.HSM.TwoFA.Context)
	}

	if cfg.HSM.TwoFA.TLS.CertFile != "pki/client/hsm-2fa-client-1.crt" {
		t.Errorf("Expected 2FA cert=hsm-2fa-client-1.crt, got %s", cfg.HSM.TwoFA.TLS.CertFile)
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
				Environment: "development",
				Server:      ServerConfig{Port: 8443},
				MySQL:       MySQLConfig{Database: "ct_system", Port: 3306},
				State:       StateConfig{FilePath: "state/daemon.state"},
				Logging:     LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: false,
		},
		{
			name: "invalid environment",
			cfg: Config{
				Environment: "staging", // Invalid
				Server:      ServerConfig{Port: 8443},
				MySQL:       MySQLConfig{Database: "ct_system", Port: 3306},
				State:       StateConfig{FilePath: "state/daemon.state"},
				Logging:     LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			cfg: Config{
				Environment: "development",
				Server:      ServerConfig{Port: 99999}, // Invalid
				MySQL:       MySQLConfig{Database: "ct_system", Port: 3306},
				State:       StateConfig{FilePath: "state/daemon.state"},
				Logging:     LoggingConfig{Level: "info", Dir: "logs"},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			cfg: Config{
				Environment: "development",
				Server:      ServerConfig{Port: 8443},
				MySQL:       MySQLConfig{Database: "ct_system", Port: 3306},
				State:       StateConfig{FilePath: "state/daemon.state"},
				Logging:     LoggingConfig{Level: "verbose", Dir: "logs"}, // Invalid
			},
			wantErr: true,
		},
		{
			name: "empty database",
			cfg: Config{
				Environment: "development",
				Server:      ServerConfig{Port: 8443},
				MySQL:       MySQLConfig{Database: "", Port: 3306}, // Invalid
				State:       StateConfig{FilePath: "state/daemon.state"},
				Logging:     LoggingConfig{Level: "info", Dir: "logs"},
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
	os.Setenv("CTS_ENVIRONMENT", "production")
	os.Setenv("CTS_MYSQL_PASSWORD", "secret123")
	os.Setenv("CTS_LOG_LEVEL", "error")
	defer func() {
		os.Unsetenv("CTS_ENVIRONMENT")
		os.Unsetenv("CTS_MYSQL_PASSWORD")
		os.Unsetenv("CTS_LOG_LEVEL")
	}()

	cfg := &Config{
		Environment: "development",
		MySQL:       MySQLConfig{Password: "default"},
		Logging:     LoggingConfig{Level: "debug"},
	}

	cfg.applyEnvOverrides()

	if cfg.Environment != "production" {
		t.Errorf("Expected environment=production, got %s", cfg.Environment)
	}

	if cfg.MySQL.Password != "secret123" {
		t.Errorf("Expected password=secret123, got %s", cfg.MySQL.Password)
	}

	if cfg.Logging.Level != "error" {
		t.Errorf("Expected log level=error, got %s", cfg.Logging.Level)
	}
}

func TestIsDevelopment(t *testing.T) {
	cfg := &Config{Environment: "development"}
	if !cfg.IsDevelopment() {
		t.Error("Expected IsDevelopment() to return true")
	}

	cfg.Environment = "production"
	if cfg.IsDevelopment() {
		t.Error("Expected IsDevelopment() to return false")
	}
}

func TestIsProduction(t *testing.T) {
	cfg := &Config{Environment: "production"}
	if !cfg.IsProduction() {
		t.Error("Expected IsProduction() to return true")
	}

	cfg.Environment = "development"
	if cfg.IsProduction() {
		t.Error("Expected IsProduction() to return false")
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

		if cfg.HSM.Trading.TLS.CertFile == "" {
			t.Error("Trading cert file should not be empty")
		}

		if cfg.HSM.Trading.TLS.KeyFile == "" {
			t.Error("Trading key file should not be empty")
		}

		if cfg.HSM.Trading.TLS.CAFile == "" {
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

		if cfg.HSM.TwoFA.TLS.CertFile == "" {
			t.Error("2FA cert file should not be empty")
		}

		if cfg.HSM.TwoFA.TLS.KeyFile == "" {
			t.Error("2FA key file should not be empty")
		}

		if cfg.HSM.TwoFA.TLS.CAFile == "" {
			t.Error("2FA CA file should not be empty")
		}
	})

	// Test that contexts are different
	t.Run("Context Isolation", func(t *testing.T) {
		if cfg.HSM.Trading.Context == cfg.HSM.TwoFA.Context {
			t.Error("Trading and 2FA contexts should be different")
		}

		if cfg.HSM.Trading.TLS.CertFile == cfg.HSM.TwoFA.TLS.CertFile {
			t.Error("Trading and 2FA should use different certificates")
		}

		if cfg.HSM.Trading.TLS.KeyFile == cfg.HSM.TwoFA.TLS.KeyFile {
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
