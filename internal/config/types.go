package config

import "time"

// Config is the root configuration structure
type Config struct {
	Environment  string          `yaml:"environment"`
	Server       ServerConfig    `yaml:"server"`
	MySQL        MySQLConfig     `yaml:"mysql"`
	HSM          HSMConfig       `yaml:"hsm"`
	State        StateConfig     `yaml:"state"`
	Logging      LoggingConfig   `yaml:"logging"`
	Session      SessionConfig   `yaml:"session"`
	Scheduler    SchedulerConfig `yaml:"scheduler"`
	RateLimiting RateLimitConfig `yaml:"rate_limiting"`
	Metrics      MetricsConfig   `yaml:"metrics"`
	Audit        AuditConfig     `yaml:"audit"`
}

// ServerConfig contains REST API server settings
type ServerConfig struct {
	Host     string        `yaml:"host"`
	Port     int           `yaml:"port"`
	TLS      TLSConfig     `yaml:"tls"`
	Timeouts TimeoutConfig `yaml:"timeouts"`
}

// TLSConfig contains TLS/mTLS settings
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// TimeoutConfig contains server timeout settings
type TimeoutConfig struct {
	Read  time.Duration `yaml:"read"`
	Write time.Duration `yaml:"write"`
	Idle  time.Duration `yaml:"idle"`
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
	Level               string `yaml:"level"`
	Dir                 string `yaml:"dir"`
	MaxFileSizeMB       int    `yaml:"max_file_size_mb"`
	MaxBackups          int    `yaml:"max_backups"`
	MaxAgeDays          int    `yaml:"max_age_days"`
	Compress            bool   `yaml:"compress"`
	ErrorPath           string `yaml:"error_path"`
	AccessPath          string `yaml:"access_path"`
	OutRequestPath      string `yaml:"out_request_path"`
	WSAccessPath        string `yaml:"ws_access_path"`
	WSOutPath           string `yaml:"ws_out_path"`
	AuditPath           string `yaml:"audit_path"`
	AccessToStdout      bool   `yaml:"access_to_stdout"`
	OutRequestToStdout  bool   `yaml:"out_request_to_stdout"`
	WSAccessToStdout    bool   `yaml:"ws_access_to_stdout"`
	WSOutToStdout       bool   `yaml:"ws_out_to_stdout"`
	AuditToStdout       bool   `yaml:"audit_to_stdout"`
	PostPayload         bool   `yaml:"post_payload"`
	PostPayloadMaxBytes int    `yaml:"post_payload_max_bytes"`
	WSDebug             bool   `yaml:"ws_debug"`
}

// SessionConfig contains session management settings
type SessionConfig struct {
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	HeartbeatTimeout  time.Duration `yaml:"heartbeat_timeout"`
	GracePeriod       time.Duration `yaml:"grace_period"`
	CleanupInterval   time.Duration `yaml:"cleanup_interval"`
}

// SchedulerConfig contains task scheduler settings
type SchedulerConfig struct {
	TaskAssignmentInterval time.Duration `yaml:"task_assignment_interval"`
	LatencyCheckInterval   time.Duration `yaml:"latency_check_interval"`
	ResourceCheckInterval  time.Duration `yaml:"resource_check_interval"`
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	REST      LimitConfig `yaml:"rest"`
	WebSocket LimitConfig `yaml:"websocket"`
}

// LimitConfig contains rate limit values
type LimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	Burst             int `yaml:"burst"`
}

// MetricsConfig contains Prometheus metrics settings
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// AuditConfig contains audit logging settings
type AuditConfig struct {
	Enabled       bool   `yaml:"enabled"`
	FilePath      string `yaml:"file_path"`
	MySQLEnabled  bool   `yaml:"mysql_enabled"`
	RetentionDays int    `yaml:"retention_days"`
}
