package config

import (
	"fmt"
	"os"
	"path/filepath"
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

	if c.Server.TLS.CertPath == "" {
		return fmt.Errorf("server.tls.cert_path is required")
	}
	if c.Server.TLS.KeyPath == "" {
		return fmt.Errorf("server.tls.key_path is required")
	}
	if c.Server.TLS.CAPath == "" {
		return fmt.Errorf("server.tls.ca_path is required")
	}

	if c.Databases.System.Engine == "" {
		return fmt.Errorf("databases.system.engine cannot be empty")
	}
	if c.Databases.System.Engine != "mysql" {
		return fmt.Errorf("unsupported databases.system.engine: %s", c.Databases.System.Engine)
	}

	// Validate MySQL settings for system DB
	if c.Databases.System.MySQL.Database == "" {
		return fmt.Errorf("databases.system.mysql database cannot be empty")
	}

	if c.Databases.System.MySQL.Port < 1 || c.Databases.System.MySQL.Port > 65535 {
		return fmt.Errorf("invalid databases.system.mysql port: %d (must be 1-65535)", c.Databases.System.MySQL.Port)
	}

	if c.RateLimit.REST.RequestsPerSecond < 0 || c.RateLimit.REST.MessagesPerSecond < 0 {
		return fmt.Errorf("invalid rate_limit.rest.requests_per_second: %d", c.RateLimit.REST.PerSecond())
	}
	if c.RateLimit.WebSocket.RequestsPerSecond < 0 || c.RateLimit.WebSocket.MessagesPerSecond < 0 {
		return fmt.Errorf("invalid rate_limit.websocket.requests_per_second: %d", c.RateLimit.WebSocket.PerSecond())
	}

	if c.RateLimit.REST.PerSecond() == 0 {
		c.RateLimit.REST.RequestsPerSecond = 17
	}
	if c.RateLimit.REST.Burst == 0 {
		c.RateLimit.REST.Burst = 100
	}

	if c.RateLimit.WebSocket.PerSecond() == 0 {
		c.RateLimit.WebSocket.RequestsPerSecond = 167
	}
	if c.RateLimit.WebSocket.Burst == 0 {
		c.RateLimit.WebSocket.Burst = 1000
	}

	if c.RateLimit.REST.PerSecond() <= 0 {
		return fmt.Errorf("invalid rate_limit.rest.requests_per_second: %d", c.RateLimit.REST.PerSecond())
	}
	if c.RateLimit.REST.Burst < 0 {
		return fmt.Errorf("invalid rate_limit.rest.burst: %d", c.RateLimit.REST.Burst)
	}
	if c.RateLimit.WebSocket.PerSecond() <= 0 {
		return fmt.Errorf("invalid rate_limit.websocket.requests_per_second: %d", c.RateLimit.WebSocket.PerSecond())
	}
	if c.RateLimit.WebSocket.Burst < 0 {
		return fmt.Errorf("invalid rate_limit.websocket.burst: %d", c.RateLimit.WebSocket.Burst)
	}

	if c.Session.HeartbeatInterval == 0 {
		c.Session.HeartbeatInterval = 60 * time.Second
	}
	if c.Session.HeartbeatTimeout == 0 {
		c.Session.HeartbeatTimeout = 180 * time.Second
	}
	if c.Session.WriteTimeout == 0 {
		c.Session.WriteTimeout = 5 * time.Second
	}
	if c.Session.GracePeriod == 0 {
		c.Session.GracePeriod = 60 * time.Second
	}
	if c.Session.CleanupInterval == 0 {
		c.Session.CleanupInterval = 5 * time.Minute
	}
	if c.Session.ProtocolVersion == "" {
		c.Session.ProtocolVersion = "1"
	}
	if c.Session.MaxPayloadBytes == 0 {
		c.Session.MaxPayloadBytes = 64 * 1024
	}
	if c.Session.MaxUnknownActions == 0 {
		c.Session.MaxUnknownActions = 5
	}
	if c.Session.UnknownActionWindow == 0 {
		c.Session.UnknownActionWindow = 10 * time.Second
	}
	if c.Session.RequestDedupWindow == 0 {
		c.Session.RequestDedupWindow = 1 * time.Minute
	}

	if c.Session.HeartbeatInterval <= 0 || c.Session.HeartbeatInterval > 24*time.Hour {
		return fmt.Errorf("invalid session.heartbeat_interval: %s", c.Session.HeartbeatInterval)
	}
	if c.Session.HeartbeatTimeout <= 0 || c.Session.HeartbeatTimeout > 24*time.Hour {
		return fmt.Errorf("invalid session.heartbeat_timeout: %s", c.Session.HeartbeatTimeout)
	}
	if c.Session.WriteTimeout <= 0 || c.Session.WriteTimeout > 24*time.Hour {
		return fmt.Errorf("invalid session.write_timeout: %s", c.Session.WriteTimeout)
	}
	if c.Session.HeartbeatTimeout < c.Session.HeartbeatInterval {
		return fmt.Errorf("invalid session settings: heartbeat_timeout (%s) must be >= heartbeat_interval (%s)", c.Session.HeartbeatTimeout, c.Session.HeartbeatInterval)
	}
	if c.Session.GracePeriod <= 0 || c.Session.GracePeriod > 24*time.Hour {
		return fmt.Errorf("invalid session.grace_period: %s", c.Session.GracePeriod)
	}
	if c.Session.CleanupInterval <= 0 || c.Session.CleanupInterval > 24*time.Hour {
		return fmt.Errorf("invalid session.cleanup_interval: %s", c.Session.CleanupInterval)
	}
	if c.Session.ProtocolVersion == "" {
		return fmt.Errorf("invalid session.protocol_version: cannot be empty")
	}
	if c.Session.MaxPayloadBytes < 1024 || c.Session.MaxPayloadBytes > 10*1024*1024 {
		return fmt.Errorf("invalid session.max_payload_bytes: %d", c.Session.MaxPayloadBytes)
	}
	if c.Session.MaxUnknownActions <= 0 || c.Session.MaxUnknownActions > 1000 {
		return fmt.Errorf("invalid session.max_unknown_actions: %d", c.Session.MaxUnknownActions)
	}
	if c.Session.UnknownActionWindow <= 0 || c.Session.UnknownActionWindow > 24*time.Hour {
		return fmt.Errorf("invalid session.unknown_action_window: %s", c.Session.UnknownActionWindow)
	}
	if c.Session.RequestDedupWindow <= 0 || c.Session.RequestDedupWindow > 24*time.Hour {
		return fmt.Errorf("invalid session.request_dedup_window: %s", c.Session.RequestDedupWindow)
	}

	if c.Scheduler.TaskAssignmentInterval == 0 {
		c.Scheduler.TaskAssignmentInterval = 1 * time.Second
	}
	if c.Scheduler.LatencyCheckInterval == 0 {
		c.Scheduler.LatencyCheckInterval = 20 * time.Minute
	}
	if c.Scheduler.ResourceCheckInterval == 0 {
		c.Scheduler.ResourceCheckInterval = 30 * time.Second
	}
	if c.Scheduler.ResourceHardLimit == 0 {
		c.Scheduler.ResourceHardLimit = 0.98
	}
	if c.Scheduler.ResourceSoftLimit == 0 {
		c.Scheduler.ResourceSoftLimit = 0.75
	}
	if c.Scheduler.ResourceSoftPenaltyMs == 0 {
		c.Scheduler.ResourceSoftPenaltyMs = 600
	}

	if c.Scheduler.TaskAssignmentInterval <= 0 || c.Scheduler.TaskAssignmentInterval > 24*time.Hour {
		return fmt.Errorf("invalid scheduler.task_assignment_interval: %s", c.Scheduler.TaskAssignmentInterval)
	}
	if c.Scheduler.LatencyCheckInterval <= 0 || c.Scheduler.LatencyCheckInterval > 24*time.Hour {
		return fmt.Errorf("invalid scheduler.latency_check_interval: %s", c.Scheduler.LatencyCheckInterval)
	}
	if c.Scheduler.ResourceCheckInterval <= 0 || c.Scheduler.ResourceCheckInterval > 24*time.Hour {
		return fmt.Errorf("invalid scheduler.resource_check_interval: %s", c.Scheduler.ResourceCheckInterval)
	}
	if c.Scheduler.ResourceHardLimit <= 0 || c.Scheduler.ResourceHardLimit >= 1 {
		return fmt.Errorf("invalid scheduler.resource_hard_limit: %f", c.Scheduler.ResourceHardLimit)
	}
	if c.Scheduler.ResourceSoftLimit <= 0 || c.Scheduler.ResourceSoftLimit >= 1 {
		return fmt.Errorf("invalid scheduler.resource_soft_limit: %f", c.Scheduler.ResourceSoftLimit)
	}
	if c.Scheduler.ResourceSoftLimit >= c.Scheduler.ResourceHardLimit {
		return fmt.Errorf("invalid scheduler resource limits: resource_soft_limit (%f) must be < resource_hard_limit (%f)", c.Scheduler.ResourceSoftLimit, c.Scheduler.ResourceHardLimit)
	}
	if c.Scheduler.ResourceSoftPenaltyMs <= 0 || c.Scheduler.ResourceSoftPenaltyMs > 1_000_000 {
		return fmt.Errorf("invalid scheduler.resource_soft_penalty_ms: %f", c.Scheduler.ResourceSoftPenaltyMs)
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
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Format != "json" && c.Logging.Format != "text" {
		return fmt.Errorf("invalid logging format: %s (must be json or text)", c.Logging.Format)
	}
	if c.Logging.Dir == "" {
		if c.Logging.ErrorPath != "" {
			c.Logging.Dir = filepath.Dir(c.Logging.ErrorPath)
		} else {
			c.Logging.Dir = "/var/log/cts-core"
		}
	}
	if c.Logging.ErrorPath == "" {
		c.Logging.ErrorPath = filepath.Join(c.Logging.Dir, "error.log")
	}
	if c.Logging.AccessPath == "" {
		c.Logging.AccessPath = filepath.Join(c.Logging.Dir, "access.log")
	}
	if c.Logging.OutRequestPath == "" {
		c.Logging.OutRequestPath = filepath.Join(c.Logging.Dir, "out_request.log")
	}
	if c.Logging.WSInPath == "" {
		c.Logging.WSInPath = filepath.Join(c.Logging.Dir, "ws_in.log")
	}
	if c.Logging.WSOutPath == "" {
		c.Logging.WSOutPath = filepath.Join(c.Logging.Dir, "ws_out.log")
	}
	if c.Logging.AuditPath == "" {
		c.Logging.AuditPath = filepath.Join(c.Logging.Dir, "audit.log")
	}

	if c.Databases.System.MySQL.TLS.Enabled {
		if c.Databases.System.MySQL.TLS.CAPath == "" {
			return fmt.Errorf("databases.system.mysql.tls.ca_path is required when tls.enabled=true")
		}
		if c.Databases.System.MySQL.TLS.CertPath == "" {
			return fmt.Errorf("databases.system.mysql.tls.cert_path is required when tls.enabled=true")
		}
		if c.Databases.System.MySQL.TLS.KeyPath == "" {
			return fmt.Errorf("databases.system.mysql.tls.key_path is required when tls.enabled=true")
		}
	}

	return nil
}

// applyEnvOverrides overrides config with environment variables
func (c *Config) applyEnvOverrides() {
	if mysqlPass := os.Getenv("CTS_DATABASES_SYSTEM_MYSQL_PASSWORD"); mysqlPass != "" {
		c.Databases.System.MySQL.Password = mysqlPass
	}

	if logLevel := os.Getenv("CTS_LOG_LEVEL"); logLevel != "" {
		c.Logging.Level = logLevel
	}
	if logFormat := os.Getenv("CTS_LOG_FORMAT"); logFormat != "" {
		c.Logging.Format = logFormat
	}

}
