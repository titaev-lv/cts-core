package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads configuration from file and validates it
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Override with environment variables (for production)
	cfg.applyEnvOverrides()

	return &cfg, nil
}

// Validate checks configuration values
func (c *Config) Validate() error {
	// Validate environment
	if c.Environment != "development" && c.Environment != "production" {
		return fmt.Errorf("invalid environment: %s (must be 'development' or 'production')", c.Environment)
	}

	// Validate server port
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	// Validate MySQL settings
	if c.MySQL.Database == "" {
		return fmt.Errorf("mysql database cannot be empty")
	}

	if c.MySQL.Port < 1 || c.MySQL.Port > 65535 {
		return fmt.Errorf("invalid mysql port: %d (must be 1-65535)", c.MySQL.Port)
	}

	// Validate state file path
	if c.State.FilePath == "" {
		return fmt.Errorf("state file path cannot be empty")
	}

	// Validate logging level
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	// Validate logging directory
	if c.Logging.Dir == "" {
		return fmt.Errorf("logging directory cannot be empty")
	}

	return nil
}

// applyEnvOverrides overrides config with environment variables
func (c *Config) applyEnvOverrides() {
	if env := os.Getenv("CTS_ENVIRONMENT"); env != "" {
		c.Environment = env
	}

	if mysqlPass := os.Getenv("CTS_MYSQL_PASSWORD"); mysqlPass != "" {
		c.MySQL.Password = mysqlPass
	}

	if logLevel := os.Getenv("CTS_LOG_LEVEL"); logLevel != "" {
		c.Logging.Level = logLevel
	}
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
