# Phase 1.1: Project Setup - Детальный гайд

> **Статус**: 🔵 Ready to Execute  
> **Время**: ~1 день (8 часов)  
> **Приоритет**: 🔴 Critical  
> **Prerequisite**: Phase 0 completed

---

## Обзор

**Цель:** Создать базовую структуру проекта, go.mod, конфигурацию, logger.

**Deliverables:**
1. Project structure (cmd/, internal/, conf/, etc.)
2. go.mod with dependencies
3. config.yaml with full configuration
4. Config loader with validation
5. Logger with zerolog (hybrid text/json)
6. Basic main.go (compiles and runs)
7. Makefile with useful targets
8. .gitignore

---

## Содержание

- [1.1.1 Структура директорий](#111-структура-директорий-30-минут)
- [1.1.2 Go модуль](#112-go-модуль-15-минут)
- [1.1.3 Конфигурация](#113-конфигурация-45-минут)
- [1.1.4 Config Tests](#114-config-tests-30-минут)
- [1.1.5 Logger](#115-logger-1-час)
- [1.1.6 Makefile](#116-makefile-30-минут)
- [1.1.7 gitignore](#117-gitignore-15-минут)
- [Summary](#phase-11-summary)

---

## 1.1.1 Структура директорий (30 минут)

### Создать директории

```bash
cd /home/dev/docker/cts-core

# Core structure
mkdir -p cmd/cts-core
mkdir -p internal/{config,logger,db,hsm,api,session,scheduler,metrics,state}
mkdir -p internal/api/{rest,ws}
mkdir -p internal/db/models
mkdir -p conf
mkdir -p pki/{ca,server,client}
mkdir -p logs
mkdir -p scripts
mkdir -p state
mkdir -p guides  # For this guide

# Create placeholder files
touch cmd/cts-core/main.go
touch internal/config/{config.go,types.go,config_test.go}
touch internal/logger/logger.go
touch conf/{config.yaml,config.example.yaml}
touch scripts/init.sh
touch Makefile
touch .gitignore

# Set permissions
chmod +x scripts/init.sh
chmod 755 pki/{ca,server,client}
chmod 700 state  # State directory should be private (only owner)
```

### Verify structure

```bash
tree -L 3 -I 'other-sub-system|migrations|*.md'
```

**✅ Expected output:**
```
.
├── cmd
│   └── cts-core
│       └── main.go
├── conf
│   ├── config.example.yaml
│   └── config.yaml
├── internal
│   ├── api
│   │   ├── rest
│   │   └── ws
│   ├── config
│   │   ├── config.go
│   │   ├── config_test.go
│   │   └── types.go
│   ├── db
│   │   └── models
│   ├── hsm
│   ├── logger
│   │   └── logger.go
│   ├── metrics
│   ├── scheduler
│   ├── session
│   └── state
├── logs
├── pki
│   ├── ca
│   ├── client
│   └── server
├── scripts
│   └── init.sh
├── state
├── Makefile
└── .gitignore
```

### Check permissions

```bash
ls -la state/
# Expected: drwx------ (700) - only owner can read/write

ls -la pki/*/
# Expected: drwxr-xr-x (755) - readable by all
```

**✅ Definition of Done:**
- [x] Все директории созданы
- [x] Placeholder файлы созданы
- [x] state/ имеет permissions 700
- [x] pki/ имеет permissions 755

---

## 1.1.2 Go модуль (15 минут)

### Инициализация

```bash
cd /home/dev/docker/cts-core

go mod init github.com/your-org/cts-core
```

### Добавить dependencies

```bash
# Web framework
go get github.com/gin-gonic/gin@v1.9.1

# WebSocket
go get github.com/gorilla/websocket@v1.5.1

# Logging
go get github.com/rs/zerolog@v1.31.0

# Database
go get github.com/go-sql-driver/mysql@v1.7.1

# Metrics
go get github.com/prometheus/client_golang@v1.17.0

# Config
go get gopkg.in/yaml.v3@v3.0.1

# Rate limiting
go get github.com/ulule/limiter/v3@v3.11.2

# Log rotation
go get gopkg.in/natefinch/lumberjack.v2@v2.2.1

# Clean up
go mod tidy
```

### Verify go.mod

```bash
cat go.mod
```

**✅ Expected content:**
```go
module github.com/your-org/cts-core

go 1.21

require (
    github.com/gin-gonic/gin v1.9.1
    github.com/go-sql-driver/mysql v1.7.1
    github.com/gorilla/websocket v1.5.1
    github.com/prometheus/client_golang v1.17.0
    github.com/rs/zerolog v1.31.0
    github.com/ulule/limiter/v3 v3.11.2
    gopkg.in/natefinch/lumberjack.v2 v2.2.1
    gopkg.in/yaml.v3 v3.0.1
)

// indirect dependencies...
```

### Verify

```bash
go mod verify
# Expected: all modules verified

go list -m all | head -15
# Expected: All main dependencies listed
```

**✅ Definition of Done:**
- [x] go.mod создан
- [x] 8 dependencies добавлены
- [x] go.sum сгенерирован
- [x] `go mod verify` успешно

---

## 1.1.3 Конфигурация (45 минут)

### conf/config.yaml

Создать полную конфигурацию (см. полный файл в attachments или ниже):

```yaml
# CTS-Core Configuration
# Environment: development | production

environment: development

server:
  host: "0.0.0.0"
  port: 8443
  
  tls:
    enabled: true
    cert_file: "pki/server/cts-core.crt"
    key_file: "pki/server/cts-core.key"
    ca_file: "pki/ca/ca.crt"
    
  timeouts:
    read: 30s
    write: 30s
    idle: 120s

mysql:
  host: "127.0.0.1"
  port: 3306
  user: "root"
  password: "root"
  database: "ct_system"
  
  pool:
    max_open_conns: 25
    max_idle_conns: 10
    conn_max_lifetime: 300s
    
  tls:
    enabled: false  # Set true in production
    ca_file: "pki/ca/ca.crt"
    cert_file: "pki/client/cts-core-mysql.crt"
    key_file: "pki/client/cts-core-mysql.key"
    
  retry:
    max_attempts: 3
    initial_delay: 100ms
    max_delay: 5s
    multiplier: 2.0

hsm:
  url: "https://hsm-service:8443"
  
  tls:
    enabled: true
    ca_file: "pki/ca/ca.crt"
    cert_file: "pki/client/cts-core-hsm.crt"
    key_file: "pki/client/cts-core-hsm.key"
    
  timeout: 10s
  
  retry:
    max_attempts: 5
    initial_delay: 200ms
    max_delay: 10s
    multiplier: 2.0

state:
  file_path: "state/daemon.state"
  sync_interval: 30s
  backup_count: 3

logging:
  level: debug
  format: text  # text (DEV) | json (PROD)
  
  output:
    console: true
    file: true
    file_path: "logs/cts-core.log"
    
  rotation:
    max_size: 100     # MB
    max_age: 7        # days
    max_backups: 10
    compress: true

session:
  heartbeat_interval: 5s
  heartbeat_timeout: 15s
  grace_period: 60s
  cleanup_interval: 300s

scheduler:
  task_assignment_interval: 1s
  latency_check_interval: 60s
  resource_check_interval: 30s

rate_limiting:
  rest:
    requests_per_minute: 1000
    burst: 100
    
  websocket:
    messages_per_minute: 10000
    burst: 1000

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

audit:
  enabled: true
  file_path: "logs/audit.log"
  mysql_enabled: false
  retention_days: 30
```

### internal/config/types.go

Создать все type definitions (200+ строк). См. полный код в DEVELOPMENT_PLAN.md секция 1.1.3.

**Основные типы:**
- `Config` - root
- `ServerConfig`, `TLSConfig`, `TimeoutConfig`
- `MySQLConfig`, `PoolConfig`, `RetryConfig`
- `HSMConfig`
- `StateConfig`
- `LoggingConfig`, `OutputConfig`, `RotationConfig`
- `SessionConfig`, `SchedulerConfig`
- `RateLimitConfig`, `LimitConfig`
- `MetricsConfig`, `AuditConfig`

### internal/config/config.go

```go
package config

import (
    "fmt"
    "os"
    
    "gopkg.in/yaml.v3"
)

// Load reads configuration from file
func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }
    
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config: %w", err)
    }
    
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("invalid configuration: %w", err)
    }
    
    cfg.applyEnvOverrides()
    
    return &cfg, nil
}

// Validate checks configuration values
func (c *Config) Validate() error {
    if c.Environment != "development" && c.Environment != "production" {
        return fmt.Errorf("invalid environment: %s", c.Environment)
    }
    
    if c.Server.Port < 1 || c.Server.Port > 65535 {
        return fmt.Errorf("invalid server port: %d", c.Server.Port)
    }
    
    if c.MySQL.Database == "" {
        return fmt.Errorf("mysql database cannot be empty")
    }
    
    if c.State.FilePath == "" {
        return fmt.Errorf("state file path cannot be empty")
    }
    
    validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
    if !validLevels[c.Logging.Level] {
        return fmt.Errorf("invalid log level: %s", c.Logging.Level)
    }
    
    validFormats := map[string]bool{"text": true, "json": true}
    if !validFormats[c.Logging.Format] {
        return fmt.Errorf("invalid log format: %s", c.Logging.Format)
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
```

### Copy to example

```bash
cp conf/config.yaml conf/config.example.yaml
```

**✅ Definition of Done:**
- [x] config.yaml создан (100+ строк)
- [x] types.go создан со всеми structs
- [x] config.go с Load() и Validate()
- [x] config.example.yaml создан

---

## 1.1.4 Config Tests (30 минут)

### internal/config/config_test.go

```go
package config

import (
    "os"
    "testing"
)

func TestLoad(t *testing.T) {
    tmpFile := createTempConfig(t)
    defer os.Remove(tmpFile)
    
    cfg, err := Load(tmpFile)
    if err != nil {
        t.Fatalf("Failed to load config: %v", err)
    }
    
    if cfg.Environment != "development" {
        t.Errorf("Expected environment=development, got %s", cfg.Environment)
    }
    
    if cfg.Server.Port != 8443 {
        t.Errorf("Expected port=8443, got %d", cfg.Server.Port)
    }
    
    if cfg.MySQL.Database != "ct_system" {
        t.Errorf("Expected database=ct_system, got %s", cfg.MySQL.Database)
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
                MySQL:       MySQLConfig{Database: "ct_system"},
                State:       StateConfig{FilePath: "state/daemon.state"},
                Logging:     LoggingConfig{Level: "info", Format: "text"},
            },
            wantErr: false,
        },
        {
            name: "invalid environment",
            cfg: Config{
                Environment: "staging",
                Server:      ServerConfig{Port: 8443},
                MySQL:       MySQLConfig{Database: "ct_system"},
                State:       StateConfig{FilePath: "state/daemon.state"},
                Logging:     LoggingConfig{Level: "info", Format: "text"},
            },
            wantErr: true,
        },
        {
            name: "invalid port",
            cfg: Config{
                Environment: "development",
                Server:      ServerConfig{Port: 99999},
                MySQL:       MySQLConfig{Database: "ct_system"},
                State:       StateConfig{FilePath: "state/daemon.state"},
                Logging:     LoggingConfig{Level: "info", Format: "text"},
            },
            wantErr: true,
        },
        {
            name: "invalid log level",
            cfg: Config{
                Environment: "development",
                Server:      ServerConfig{Port: 8443},
                MySQL:       MySQLConfig{Database: "ct_system"},
                State:       StateConfig{FilePath: "state/daemon.state"},
                Logging:     LoggingConfig{Level: "verbose", Format: "text"},
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

func createTempConfig(t *testing.T) string {
    content := `
environment: development
server:
  port: 8443
mysql:
  database: "ct_system"
state:
  file_path: "state/daemon.state"
logging:
  level: info
  format: text
`
    
    tmpFile, err := os.CreateTemp("", "config-*.yaml")
    if err != nil {
        t.Fatalf("Failed to create temp file: %v", err)
    }
    
    if _, err := tmpFile.WriteString(content); err != nil {
        t.Fatalf("Failed to write temp file: %v", err)
    }
    
    tmpFile.Close()
    return tmpFile.Name()
}
```

### Run tests

```bash
go test ./internal/config/... -v

# Expected output:
# === RUN   TestLoad
# --- PASS: TestLoad
# === RUN   TestValidate
# === RUN   TestValidate/valid_config
# === RUN   TestValidate/invalid_environment
# === RUN   TestValidate/invalid_port
# === RUN   TestValidate/invalid_log_level
# --- PASS: TestValidate
# === RUN   TestEnvOverrides
# --- PASS: TestEnvOverrides
# PASS
```

### Check coverage

```bash
go test ./internal/config/... -cover

# Expected: coverage: 85.0% of statements
```

**✅ Definition of Done:**
- [x] config_test.go создан
- [x] Все тесты пройдены (3/3)
- [x] Coverage > 80%

---

## 1.1.5 Logger (1 час)

### internal/logger/logger.go

```go
package logger

import (
    "io"
    "os"
    "path/filepath"
    "time"
    
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
    "gopkg.in/natefinch/lumberjack.v2"
)

type LoggerConfig struct {
    Level      string
    Format     string // "text" or "json"
    Console    bool
    File       bool
    FilePath   string
    MaxSize    int  // MB
    MaxAge     int  // days
    MaxBackups int
    Compress   bool
}

// Init initializes the global logger
func Init(cfg LoggerConfig) error {
    level, err := zerolog.ParseLevel(cfg.Level)
    if err != nil {
        return err
    }
    zerolog.SetGlobalLevel(level)
    
    var writers []io.Writer
    
    // Console output
    if cfg.Console {
        var consoleWriter io.Writer
        if cfg.Format == "text" {
            consoleWriter = zerolog.ConsoleWriter{
                Out:        os.Stdout,
                TimeFormat: time.RFC3339,
            }
        } else {
            consoleWriter = os.Stdout
        }
        writers = append(writers, consoleWriter)
    }
    
    // File output with rotation
    if cfg.File {
        logDir := filepath.Dir(cfg.FilePath)
        if err := os.MkdirAll(logDir, 0755); err != nil {
            return err
        }
        
        fileWriter := &lumberjack.Logger{
            Filename:   cfg.FilePath,
            MaxSize:    cfg.MaxSize,
            MaxAge:     cfg.MaxAge,
            MaxBackups: cfg.MaxBackups,
            Compress:   cfg.Compress,
        }
        writers = append(writers, fileWriter)
    }
    
    multiWriter := io.MultiWriter(writers...)
    
    log.Logger = zerolog.New(multiWriter).
        With().
        Timestamp().
        Caller().
        Logger()
    
    log.Info().
        Str("level", cfg.Level).
        Str("format", cfg.Format).
        Bool("console", cfg.Console).
        Bool("file", cfg.File).
        Msg("Logger initialized")
    
    return nil
}

// GetLogger returns the global logger
func GetLogger() *zerolog.Logger {
    return &log.Logger
}
```

### cmd/cts-core/main.go

```go
package main

import (
    "flag"
    "os"
    
    "github.com/your-org/cts-core/internal/config"
    "github.com/your-org/cts-core/internal/logger"
    "github.com/rs/zerolog/log"
)

func main() {
    configPath := flag.String("config", "conf/config.yaml", "Path to configuration file")
    flag.Parse()
    
    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load configuration")
    }
    
    loggerCfg := logger.LoggerConfig{
        Level:      cfg.Logging.Level,
        Format:     cfg.Logging.Format,
        Console:    cfg.Logging.Output.Console,
        File:       cfg.Logging.Output.File,
        FilePath:   cfg.Logging.Output.FilePath,
        MaxSize:    cfg.Logging.Rotation.MaxSize,
        MaxAge:     cfg.Logging.Rotation.MaxAge,
        MaxBackups: cfg.Logging.Rotation.MaxBackups,
        Compress:   cfg.Logging.Rotation.Compress,
    }
    
    if err := logger.Init(loggerCfg); err != nil {
        log.Fatal().Err(err).Msg("Failed to initialize logger")
    }
    
    log.Info().
        Str("environment", cfg.Environment).
        Str("version", "0.0.1").
        Msg("CTS-Core starting")
    
    // TODO: Phase 1.2 - Initialize MySQL pool
    // TODO: Phase 1.3 - Initialize HSM client
    // TODO: Phase 1.4 - Load state
    // TODO: Phase 1.5 - Start REST server
    
    log.Info().Msg("CTS-Core initialized successfully")
    
    // Keep running
    select {}
}
```

### Test run

```bash
# Build
go build -o bin/cts-core cmd/cts-core/main.go

# Run
./bin/cts-core -config conf/config.yaml
```

**✅ Expected console output (text format):**
```
2026-01-28T10:00:00Z INF Logger initialized level=debug format=text console=true file=true
2026-01-28T10:00:00Z INF CTS-Core starting environment=development version=0.0.1
2026-01-28T10:00:00Z INF CTS-Core initialized successfully
```

### Verify log file

```bash
tail -f logs/cts-core.log
# Should see same messages

# Test rotation (create large log)
for i in {1..1000}; do 
  echo "2026-01-28T10:00:00Z INF Test message $i" >> logs/cts-core.log
done

# Check rotation occurred
ls -lh logs/
# Expected: cts-core.log + cts-core-2026-01-28T10-00-00.log.gz (if >100MB)
```

**✅ Definition of Done:**
- [x] logger.go создан с zerolog
- [x] main.go компилируется без ошибок
- [x] Binary запускается успешно
- [x] Логи пишутся в console + file
- [x] Log rotation работает

---

## 1.1.6 Makefile (30 минут)

### Makefile

См. полный Makefile в DEVELOPMENT_PLAN.md или создайте с targets:

**Основные targets:**
- `help` - показать помощь
- `install` - установить зависимости
- `build` - собрать binary
- `run` - запустить приложение
- `dev` - build + run
- `test` - запустить тесты
- `test-coverage` - coverage report
- `lint` - запустить linters
- `fmt` - форматировать код
- `clean` - очистить artifacts
- `db-migrate` - применить миграции
- `docker-build` - собрать Docker image

### Test Makefile

```bash
# Test each target
make help
# Expected: List of targets with descriptions

make install
# Expected: Dependencies installed

make build
# Expected: bin/cts-core created

ls -lh bin/cts-core
# Expected: ~20-30MB binary

make test
# Expected: All tests pass

make clean
# Expected: bin/ removed, logs cleared
```

**✅ Definition of Done:**
- [x] Makefile создан
- [x] Все targets работают
- [x] `make build` создает binary
- [x] `make test` проходит

---

## 1.1.7 .gitignore (15 минут)

### .gitignore

```gitignore
# Binaries
bin/
*.exe
*.dll
*.so
*.dylib

# Test
*.test
*.out
coverage.html
coverage.out

# Go
go.work

# Logs
logs/*.log
logs/*.log.*

# State files
state/*.state
state/*.backup

# Certificates (generated)
pki/server/*.crt
pki/server/*.key
pki/client/*.crt
pki/client/*.key

# Config
conf/config.yaml
!conf/config.example.yaml

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Backups
*.backup
backup_*.sql

# Temporary
tmp/
temp/

# Guides (delete after completion)
guides/
```

### Test .gitignore

```bash
# Check git status
git status

# Should NOT show:
# - logs/*.log
# - state/*.state
# - conf/config.yaml (unless added explicitly)
# - bin/

# Should show:
# - conf/config.example.yaml
# - internal/**/*.go
# - cmd/**/*.go
```

**✅ Definition of Done:**
- [x] .gitignore создан
- [x] Sensitive files игнорируются
- [x] Generated files игнорируются
- [x] Git status clean

---

## Phase 1.1 Summary

### ✅ Completed Checklist

- [x] **1.1.1 Directory structure** (30 min)
  - [x] cmd/, internal/, conf/, logs/, state/
  - [x] Permissions configured

- [x] **1.1.2 Go module** (15 min)
  - [x] go.mod initialized
  - [x] 8 dependencies added
  - [x] Verified

- [x] **1.1.3 Configuration** (45 min)
  - [x] config.yaml created (100+ lines)
  - [x] types.go with all structs
  - [x] config.go with Load() + Validate()
  - [x] config.example.yaml

- [x] **1.1.4 Config tests** (30 min)
  - [x] config_test.go
  - [x] All tests pass
  - [x] Coverage > 80%

- [x] **1.1.5 Logger** (1 hour)
  - [x] logger.go with zerolog
  - [x] Hybrid format (text/json)
  - [x] Log rotation
  - [x] main.go compiles and runs

- [x] **1.1.6 Makefile** (30 min)
  - [x] 15+ targets
  - [x] All targets work

- [x] **1.1.7 .gitignore** (15 min)
  - [x] Sensitive files ignored
  - [x] Git clean

### 📊 Metrics

**Total Time:** ~1 day (8 hours)  
**Files Created:** 15+  
**Lines of Code:** ~800  
**Go Packages:** 2 (config, logger)  
**Tests:** 3 test functions  
**Dependencies:** 8  

### 🎯 Next Phase

**Phase 1.2: MySQL Connection Pool**
- Location: `guides/phase_1_2_mysql_pool.md`
- Time: 2 days
- Deliverables: MySQL pool with mTLS, retry logic, repository pattern

### 📦 Commit Changes

```bash
git add .
git commit -m "feat(setup): phase 1.1 complete - project setup

- Project structure created (cmd/, internal/, conf/)
- go.mod with 8 dependencies
- Config system with validation and env overrides
- Logger with zerolog (hybrid text/json, rotation)
- Basic main.go (compiles and runs)
- Makefile with 15+ targets
- .gitignore configured

Tests: 3/3 passing, coverage: 85%"

git push
```

---

## ❓ FAQ

**Q: Почему Go 1.21, а не 1.24.9?**  
A: В go.mod используется 1.21 для совместимости. Можете обновить до 1.24 если нужно.

**Q: Можно ли использовать другой logger?**  
A: Да, но zerolog быстрый и поддерживает hybrid format (text DEV, json PROD).

**Q: Нужно ли настраивать IDE?**  
A: Рекомендуется VS Code с Go extension. `.vscode/` в .gitignore.

**Q: Что если config.yaml не найден?**  
A: Приложение завершится с ошибкой. Используйте `-config` flag для указания пути.

---

**🗑️ DELETE THIS FILE** after successfully completing Phase 1.1
