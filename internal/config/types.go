package config

import "time"

// Config is the root configuration structure
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Databases DatabasesConfig `yaml:"databases"`
	HSM       HSMConfig       `yaml:"hsm"`
	State     StateConfig     `yaml:"state"`
	Logging   LoggingConfig   `yaml:"logging"`
	Session   SessionConfig   `yaml:"session"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Metrics   MetricsConfig   `yaml:"metrics"`
}

// DatabasesConfig contains unified database targets by function.
type DatabasesConfig struct {
	System DatabaseTargetConfig `yaml:"system"`
	Audit  DatabaseTargetConfig `yaml:"audit"`
	Quotes DatabaseTargetConfig `yaml:"quotes"`
}

// DatabaseTargetConfig contains selected engine and engine-specific sections.
type DatabaseTargetConfig struct {
	Engine     string           `yaml:"engine"`
	MySQL      MySQLConfig      `yaml:"mysql"`
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	PostgreSQL PostgreSQLConfig `yaml:"postgresql"`
}

// ServerConfig contains REST API server settings
type ServerConfig struct {
	Port     int             `yaml:"port"`
	TLS      ServerTLSConfig `yaml:"tls"`
	Timeouts TimeoutConfig   `yaml:"timeouts"`
	Limits   LimitsConfig    `yaml:"limits"`
	HTTP2    *HTTP2Config    `yaml:"http2,omitempty"`
}

// ServerTLSConfig contains REST/WS TLS settings.
// TLS is always enabled and WS client certificate validation is always enforced by code.
type ServerTLSConfig struct {
	CertPath                       string   `yaml:"cert_path"`
	KeyPath                        string   `yaml:"key_path"`
	CAPath                         string   `yaml:"ca_path"`
	AllowedClientCommonNames       []string `yaml:"allowed_client_common_names"`
	AllowedClientOrganizationalOUs []string `yaml:"allowed_client_organizational_units"`
	AllowedClientDNSNames          []string `yaml:"allowed_client_dns_names"`
}

// TLSConfig contains generic TLS settings for outbound services/databases.
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertPath string `yaml:"cert_path"`
	KeyPath  string `yaml:"key_path"`
	CAPath   string `yaml:"ca_path"`
}

// TimeoutConfig contains server timeout settings
type TimeoutConfig struct {
	Read          time.Duration `yaml:"read"`
	Write         time.Duration `yaml:"write"`
	Idle          time.Duration `yaml:"idle"`
	ReadHeader    time.Duration `yaml:"read_header"`
	ShutdownGrace time.Duration `yaml:"shutdown_grace"`
}

// LimitsConfig contains HTTP server limits settings
type LimitsConfig struct {
	MaxHeaderBytes int `yaml:"max_header_bytes"`
}

// MySQLConfig contains database connection settings
type MySQLConfig struct {
	Host     string      `yaml:"host"`
	Port     int         `yaml:"port"`
	User     string      `yaml:"user"`
	Password string      `yaml:"password"`
	Database string      `yaml:"database"`
	Pool     PoolConfig  `yaml:"pool"`
	TLS      TLSConfig   `yaml:"tls"`
	Retry    RetryConfig `yaml:"retry"`
}

// ClickHouseConfig is reserved for unified databases schema.
type ClickHouseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// PostgreSQLConfig is reserved for unified databases schema.
type PostgreSQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// PoolConfig contains connection pool settings
type PoolConfig struct {
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// RetryConfig contains retry logic settings
type RetryConfig struct {
	MaxAttempts  int           `yaml:"max_attempts"`
	InitialDelay time.Duration `yaml:"initial_delay"`
	MaxDelay     time.Duration `yaml:"max_delay"`
	Multiplier   float64       `yaml:"multiplier"`
}

// HSMConfig contains HSM service connection settings
// Supports two contexts: Trading (exchange keys) + 2FA (user secrets)
type HSMConfig struct {
	URL     string           `yaml:"url"`
	Timeout time.Duration    `yaml:"timeout"`
	Retry   RetryConfig      `yaml:"retry"`
	Trading HSMContextConfig `yaml:"trading"`
	TwoFA   HSMContextConfig `yaml:"two_fa"`
}

// HSMContextConfig contains settings for a specific HSM context
type HSMContextConfig struct {
	Context string    `yaml:"context"`
	TLS     TLSConfig `yaml:"tls"`
}

// StateConfig contains state file settings
type StateConfig struct {
	FilePath     string        `yaml:"file_path"`
	SyncInterval time.Duration `yaml:"sync_interval"`
	BackupCount  int           `yaml:"backup_count"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level              string `yaml:"level"`
	Format             string `yaml:"format"`
	Dir                string `yaml:"dir"`
	MaxSizeMB          int    `yaml:"max_size_mb"`
	MaxBackups         int    `yaml:"max_backups"`
	MaxAgeDays         int    `yaml:"max_age_days"`
	Compress           bool   `yaml:"compress"`
	ErrorPath          string `yaml:"error_path"`
	AccessPath         string `yaml:"access_path"`
	OutRequestPath     string `yaml:"out_request_path"`
	WSInPath           string `yaml:"ws_in_path"`
	WSOutPath          string `yaml:"ws_out_path"`
	AuditPath          string `yaml:"audit_path"`
	AccessToStdout     bool   `yaml:"access_to_stdout"`
	OutRequestToStdout bool   `yaml:"out_request_to_stdout"`
	WSInToStdout       bool   `yaml:"ws_in_to_stdout"`
	WSOutToStdout      bool   `yaml:"ws_out_to_stdout"`
	AuditToStdout      bool   `yaml:"audit_to_stdout"`
}

// SessionConfig contains session management settings
type SessionConfig struct {
	HeartbeatInterval   time.Duration `yaml:"heartbeat_interval"`
	HeartbeatTimeout    time.Duration `yaml:"heartbeat_timeout"`
	WriteTimeout        time.Duration `yaml:"write_timeout"`
	GracePeriod         time.Duration `yaml:"grace_period"`
	CleanupInterval     time.Duration `yaml:"cleanup_interval"`
	ProtocolVersion     string        `yaml:"protocol_version"`
	MaxPayloadBytes     int           `yaml:"max_payload_bytes"`
	MaxUnknownActions   int           `yaml:"max_unknown_actions"`
	UnknownActionWindow time.Duration `yaml:"unknown_action_window"`
	RequestDedupWindow  time.Duration `yaml:"request_dedup_window"`
}

// SchedulerConfig contains task scheduler settings
type SchedulerConfig struct {
	TaskAssignmentInterval time.Duration `yaml:"task_assignment_interval"`
	LatencyCheckInterval   time.Duration `yaml:"latency_check_interval"`
	ResourceCheckInterval  time.Duration `yaml:"resource_check_interval"`
	ResourceHardLimit      float64       `yaml:"resource_hard_limit"`
	ResourceSoftLimit      float64       `yaml:"resource_soft_limit"`
	ResourceSoftPenaltyMs  float64       `yaml:"resource_soft_penalty_ms"`
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	REST      LimitConfig `yaml:"rest"`
	WebSocket LimitConfig `yaml:"websocket"`
}

// LimitConfig contains rate limit values
type LimitConfig struct {
	RequestsPerSecond int `yaml:"requests_per_second"`
	MessagesPerSecond int `yaml:"messages_per_second"`
	Burst             int `yaml:"burst"`
}

func (l LimitConfig) PerSecond() int {
	if l.RequestsPerSecond > 0 {
		return l.RequestsPerSecond
	}
	return l.MessagesPerSecond
}

// MetricsConfig contains Prometheus metrics settings
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}
