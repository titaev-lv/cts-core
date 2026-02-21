package config

import (
	"fmt"
	"os"
	"time"

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
	// Validate server port
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	if c.Server.Timeouts.Read == 0 {
		c.Server.Timeouts.Read = 30 * time.Second
	}
	if c.Server.Timeouts.Write == 0 {
		c.Server.Timeouts.Write = 30 * time.Second
	}
	if c.Server.Timeouts.Idle == 0 {
		c.Server.Timeouts.Idle = 120 * time.Second
	}
	if c.Server.Timeouts.ReadHeader == 0 {
		c.Server.Timeouts.ReadHeader = 5 * time.Second
	}
	if c.Server.Timeouts.ShutdownGrace == 0 {
		c.Server.Timeouts.ShutdownGrace = 10 * time.Second
	}
	if c.Server.Limits.MaxHeaderBytes == 0 {
		c.Server.Limits.MaxHeaderBytes = 1 << 20 // 1 MiB
	}

	if c.Server.Timeouts.Read < 0 || c.Server.Timeouts.Read > 24*time.Hour {
		return fmt.Errorf("invalid server.timeouts.read: %s", c.Server.Timeouts.Read)
	}
	if c.Server.Timeouts.Write < 0 || c.Server.Timeouts.Write > 24*time.Hour {
		return fmt.Errorf("invalid server.timeouts.write: %s", c.Server.Timeouts.Write)
	}
	if c.Server.Timeouts.Idle < 0 || c.Server.Timeouts.Idle > 24*time.Hour {
		return fmt.Errorf("invalid server.timeouts.idle: %s", c.Server.Timeouts.Idle)
	}
	if c.Server.Timeouts.ReadHeader < 0 || c.Server.Timeouts.ReadHeader > 24*time.Hour {
		return fmt.Errorf("invalid server.timeouts.read_header: %s", c.Server.Timeouts.ReadHeader)
	}
	if c.Server.Timeouts.ShutdownGrace < 0 || c.Server.Timeouts.ShutdownGrace > 24*time.Hour {
		return fmt.Errorf("invalid server.timeouts.shutdown_grace: %s", c.Server.Timeouts.ShutdownGrace)
	}
	if c.Server.Limits.MaxHeaderBytes < 4096 || c.Server.Limits.MaxHeaderBytes > (16<<20) {
		return fmt.Errorf("invalid server.limits.max_header_bytes: %d", c.Server.Limits.MaxHeaderBytes)
	}
	if c.Server.HTTP2 != nil {
		if _, err := c.Server.HTTP2.Parse(); err != nil {
			return fmt.Errorf("invalid server.http2: %w", err)
		}
	}

	if c.Server.TLS.Enabled {
		if c.Server.TLS.CertPath == "" {
			return fmt.Errorf("server.tls.cert_path is required when server.tls.enabled=true")
		}
		if c.Server.TLS.KeyPath == "" {
			return fmt.Errorf("server.tls.key_path is required when server.tls.enabled=true")
		}
	}

	// Validate MySQL settings
	if c.MySQL.Database == "" {
		return fmt.Errorf("mysql database cannot be empty")
	}

	if c.MySQL.Port < 1 || c.MySQL.Port > 65535 {
		return fmt.Errorf("invalid mysql port: %d (must be 1-65535)", c.MySQL.Port)
	}

	if c.RateLimit.REST.RequestsPerMinute < 0 || c.RateLimit.REST.MessagesPerMinute < 0 {
		return fmt.Errorf("invalid rate_limit.rest.requests_per_minute: %d", c.RateLimit.REST.PerMinute())
	}
	if c.RateLimit.WebSocket.RequestsPerMinute < 0 || c.RateLimit.WebSocket.MessagesPerMinute < 0 {
		return fmt.Errorf("invalid rate_limit.websocket.requests_per_minute: %d", c.RateLimit.WebSocket.PerMinute())
	}

	if c.RateLimit.REST.PerMinute() == 0 {
		c.RateLimit.REST.RequestsPerMinute = 1000
	}
	if c.RateLimit.REST.Burst == 0 {
		c.RateLimit.REST.Burst = 100
	}

	if c.RateLimit.WebSocket.PerMinute() == 0 {
		c.RateLimit.WebSocket.RequestsPerMinute = 10000
	}
	if c.RateLimit.WebSocket.Burst == 0 {
		c.RateLimit.WebSocket.Burst = 1000
	}

	if c.RateLimit.REST.PerMinute() <= 0 {
		return fmt.Errorf("invalid rate_limit.rest.requests_per_minute: %d", c.RateLimit.REST.PerMinute())
	}
	if c.RateLimit.REST.Burst < 0 {
		return fmt.Errorf("invalid rate_limit.rest.burst: %d", c.RateLimit.REST.Burst)
	}
	if c.RateLimit.WebSocket.PerMinute() <= 0 {
		return fmt.Errorf("invalid rate_limit.websocket.requests_per_minute: %d", c.RateLimit.WebSocket.PerMinute())
	}
	if c.RateLimit.WebSocket.Burst < 0 {
		return fmt.Errorf("invalid rate_limit.websocket.burst: %d", c.RateLimit.WebSocket.Burst)
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
	if mysqlPass := os.Getenv("CTS_MYSQL_PASSWORD"); mysqlPass != "" {
		c.MySQL.Password = mysqlPass
	}

	if logLevel := os.Getenv("CTS_LOG_LEVEL"); logLevel != "" {
		c.Logging.Level = logLevel
	}

}
