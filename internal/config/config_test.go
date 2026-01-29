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
