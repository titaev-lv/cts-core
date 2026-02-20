# Phase 1.5: Basic REST API Server

**Цель:** Запустить Gin HTTP server с TLS, middleware, /health и /metrics endpoints.

**Время:** ~2 дня (12-16 часов)

**Зависимости:**
- ✅ Phase 1.1 завершена (config, logger готовы)
- ✅ Phase 1.2 завершена (MySQL готов для health check)
- ✅ Phase 1.3 завершена (HSM готов для health check)
- ✅ Phase 1.4 завершена (State готов для health check)
- TLS certificates готовы (conf/ssl/server-cert.pem, server-key.pem, ca-cert.pem)

---

## 1.5.1: Gin Server Setup (3 часа)

### Шаг 1: Создать internal/api/server.go

```go
package api

import (
    "context"
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "net/http"
    "os"
    "time"

    "github.com/gin-gonic/gin"
    "log/slog"
)

type Server struct {
    router     *gin.Engine
    httpServer *http.Server
    logger     *slog.Logger
}

type ServerConfig struct {
    Host string
    Port int
    
    TLS struct {
        Enabled  bool
        CertPath string
        KeyPath  string
        CAPath   string
        ClientAuth bool // Require client certificates (mTLS)
    }
    
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    IdleTimeout  time.Duration
}

// NewServer creates new API server
func NewServer(cfg ServerConfig, logger *slog.Logger) (*Server, error) {
    // Set Gin mode based on environment
    gin.SetMode(gin.ReleaseMode)
    
    router := gin.New()
    
    server := &Server{
        router: router,
        logger: logger,
    }
    
    // Create HTTP server
    addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
    httpServer := &http.Server{
        Addr:         addr,
        Handler:      router,
        ReadTimeout:  cfg.ReadTimeout,
        WriteTimeout: cfg.WriteTimeout,
        IdleTimeout:  cfg.IdleTimeout,
    }
    
    // Configure TLS if enabled
    if cfg.TLS.Enabled {
        tlsConfig, err := server.configureTLS(cfg)
        if err != nil {
            return nil, fmt.Errorf("failed to configure TLS: %w", err)
        }
        httpServer.TLSConfig = tlsConfig
    }
    
    server.httpServer = httpServer
    
    logger.Info("API server initialized",
        "addr", addr,
        "tls", cfg.TLS.Enabled,
        "mtls", cfg.TLS.ClientAuth,
    )
    
    return server, nil
}

// configureTLS configures TLS settings
func (s *Server) configureTLS(cfg ServerConfig) (*tls.Config, error) {
    // Load server certificate
    cert, err := tls.LoadX509KeyPair(cfg.TLS.CertPath, cfg.TLS.KeyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load server cert: %w", err)
    }
    
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS12,
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
            tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
        },
        PreferServerCipherSuites: true,
    }
    
    // Configure mTLS (client certificate authentication)
    if cfg.TLS.ClientAuth {
        // Load CA certificate
        caCert, err := os.ReadFile(cfg.TLS.CAPath)
        if err != nil {
            return nil, fmt.Errorf("failed to read CA cert: %w", err)
        }
        
        caCertPool := x509.NewCertPool()
        if !caCertPool.AppendCertsFromPEM(caCert) {
            return nil, fmt.Errorf("failed to append CA cert")
        }
        
        tlsConfig.ClientCAs = caCertPool
        tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
        
        s.logger.Info().Msg("mTLS client authentication enabled")
    }
    
    return tlsConfig, nil
}

// Router returns Gin router for registering routes
func (s *Server) Router() *gin.Engine {
    return s.router
}

// Start starts HTTP server
func (s *Server) Start() error {
    s.logger.Info().Str("addr", s.httpServer.Addr).Msg("Starting API server...")
    
    if s.httpServer.TLSConfig != nil {
        // Start HTTPS server
        return s.httpServer.ListenAndServeTLS("", "")
    }
    
    // Start HTTP server
    return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down server
func (s *Server) Shutdown(ctx context.Context) error {
    s.logger.Info().Msg("Shutting down API server...")
    
    if err := s.httpServer.Shutdown(ctx); err != nil {
        return fmt.Errorf("server shutdown failed: %w", err)
    }
    
    s.logger.Info().Msg("API server stopped")
    return nil
}
```

**Время:** 2 часа

### Шаг 2: Обновить server config

Добавить в `internal/config/types.go`:

```go
type ServerConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
    
    TLS struct {
        Enabled    bool   `yaml:"enabled"`
        CertPath   string `yaml:"cert"`
        KeyPath    string `yaml:"key"`
        CAPath     string `yaml:"ca"`
        ClientAuth bool   `yaml:"client_auth"` // mTLS
    } `yaml:"tls"`
    
    Timeouts struct {
        Read  int `yaml:"read_seconds"`
        Write int `yaml:"write_seconds"`
        Idle  int `yaml:"idle_seconds"`
    } `yaml:"timeouts"`
}
```

Обновить `conf/config.yaml`:

```yaml
server:
  host: 0.0.0.0
  port: 8443
  tls:
    enabled: true
    cert: conf/ssl/server-cert.pem
    key: conf/ssl/server-key.pem
    ca: conf/ssl/ca-cert.pem
    client_auth: false  # Set true for mTLS (Phase 2)
  timeouts:
    read_seconds: 15
    write_seconds: 15
    idle_seconds: 60
```

**Время:** 30 минут

### Шаг 3: Интегрировать в main.go

Обновить `cmd/daemon/main.go`:

```go
import (
    "github.com/your-org/cts-core/internal/api"
    "os"
    "os/signal"
    "syscall"
    "time"
)

func main() {
    // ... (после stateManager)

    // Initialize API server
    serverCfg := api.ServerConfig{
        Host: cfg.Server.Host,
        Port: cfg.Server.Port,
        ReadTimeout:  time.Duration(cfg.Server.Timeouts.Read) * time.Second,
        WriteTimeout: time.Duration(cfg.Server.Timeouts.Write) * time.Second,
        IdleTimeout:  time.Duration(cfg.Server.Timeouts.Idle) * time.Second,
    }
    serverCfg.TLS.Enabled = cfg.Server.TLS.Enabled
    serverCfg.TLS.CertPath = cfg.Server.TLS.CertPath
    serverCfg.TLS.KeyPath = cfg.Server.TLS.KeyPath
    serverCfg.TLS.CAPath = cfg.Server.TLS.CAPath
    serverCfg.TLS.ClientAuth = cfg.Server.TLS.ClientAuth

    apiServer, err := api.NewServer(serverCfg, logger)
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to create API server")
    }

    // TODO: Register routes (will do in next step)

    // Start server in goroutine
    go func() {
        if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
            logger.Fatal().Err(err).Msg("API server failed")
        }
    }()

    logger.Info().Msg("CTS-Core started successfully")

    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    logger.Info().Msg("Shutting down...")

    // Graceful shutdown
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := apiServer.Shutdown(shutdownCtx); err != nil {
        logger.Error().Err(err).Msg("Server shutdown error")
    }

    // stateManager.Close() is handled by defer
    // dbClient.Close() is handled by defer
    // hsmClient.Close() is handled by defer

    logger.Info().Msg("CTS-Core stopped")
}
```

**Время:** 30 минут

### Верификация 1.5.1

```bash
# Build
make build

# Run
./bin/cts-core -config conf/config.yaml

# Ожидаемый вывод:
# {"level":"info","message":"API server initialized","addr":"0.0.0.0:8443","tls":true,"mtls":false}
# {"level":"info","message":"Starting API server...","addr":"0.0.0.0:8443"}
# {"level":"info","message":"CTS-Core started successfully"}

# Test connection (should fail because no routes yet)
curl -k https://localhost:8443/
# Expected: 404 page not found

# Stop server (Ctrl+C)
# Expected: Graceful shutdown logs
```

**Definition of Done:**
- [ ] `internal/api/server.go` создан (~180 строк)
- [ ] ServerConfig добавлен в config
- [ ] config.yaml обновлен (server секция)
- [ ] main.go интегрирован с graceful shutdown
- [ ] Server запускается с TLS
- [ ] `make build` проходит без ошибок

---

## 1.5.2: Middleware (2 часа)

### Шаг 1: Создать internal/api/middleware/logger.go

```go
package middleware

import (
    "time"

    "github.com/gin-gonic/gin"
    "log/slog"
)

// Logger middleware logs HTTP requests
func Logger(logger *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        query := c.Request.URL.RawQuery

        // Process request
        c.Next()

        // Log after request
        latency := time.Since(start)
        statusCode := c.Writer.Status()
        clientIP := c.ClientIP()
        method := c.Request.Method
        errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

        attrs := []any{
            "method", method,
            "path", path,
            "query", query,
            "status", statusCode,
            "latency", latency,
            "client_ip", clientIP,
            "user_agent", c.Request.UserAgent(),
            "error", errorMessage,
        }

        if statusCode >= 500 {
            logger.Error("HTTP request", attrs...)
        } else if statusCode >= 400 {
            logger.Warn("HTTP request", attrs...)
        } else {
            logger.Info("HTTP request", attrs...)
        }
    }
}
```

**Время:** 30 минут

### Шаг 2: Создать internal/api/middleware/recovery.go

```go
package middleware

import (
    "fmt"
    "net/http"
    "runtime/debug"

    "github.com/gin-gonic/gin"
    "log/slog"
)

// Recovery middleware recovers from panics
func Recovery(logger *slog.Logger) gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if err := recover(); err != nil {
                // Log panic with stack trace
                logger.Error("Panic recovered",
                    "panic", fmt.Sprintf("%v", err),
                    "stack", string(debug.Stack()),
                )

                // Return 500 error
                c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                    "error": "Internal server error",
                })
            }
        }()

        c.Next()
    }
}
```

**Время:** 20 минут

### Шаг 3: Создать internal/api/middleware/cors.go

```go
package middleware

import (
    "github.com/gin-gonic/gin"
)

// CORS middleware adds CORS headers
func CORS() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Access-Control-Allow-Origin", "*")
        c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Session-Token")
        c.Header("Access-Control-Max-Age", "86400")

        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(204)
            return
        }

        c.Next()
    }
}
```

**Время:** 15 минут

### Шаг 4: Создать internal/api/middleware/ratelimit.go

```go
package middleware

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/ulule/limiter/v3"
    "github.com/ulule/limiter/v3/drivers/store/memory"
)

// RateLimit middleware limits requests per IP
func RateLimit(rate string) gin.HandlerFunc {
    // Parse rate (e.g., "100-M" = 100 requests per minute)
    parsedRate, err := limiter.NewRateFromFormatted(rate)
    if err != nil {
        panic("invalid rate limit format: " + err.Error())
    }

    store := memory.NewStore()
    limitInstance := limiter.New(store, parsedRate)

    return func(c *gin.Context) {
        clientIP := c.ClientIP()

        context, err := limitInstance.Get(c.Request.Context(), clientIP)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                "error": "Rate limit error",
            })
            return
        }

        // Set rate limit headers
        c.Header("X-RateLimit-Limit", string(rune(context.Limit)))
        c.Header("X-RateLimit-Remaining", string(rune(context.Remaining)))
        c.Header("X-RateLimit-Reset", string(rune(context.Reset)))

        if context.Reached {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded",
            })
            return
        }

        c.Next()
    }
}
```

**Время:** 30 minuts

### Шаг 5: Применить middleware в server.go

Добавить в `internal/api/server.go`:

```go
import (
    "github.com/your-org/cts-core/internal/api/middleware"
)

// NewServer creates new API server
func NewServer(cfg ServerConfig, logger *slog.Logger) (*Server, error) {
    // ... (existing code)
    
    router := gin.New()
    
    // Apply global middleware
    router.Use(middleware.Recovery(logger))
    router.Use(middleware.Logger(logger))
    router.Use(middleware.CORS())
    
    // Optional: Rate limiting (100 req/min per IP)
    // router.Use(middleware.RateLimit("100-M"))
    
    // ... (rest of code)
}
```

**Время:** 15 минут

### Верификация 1.5.2

```bash
# Build and run
make build
./bin/cts-core -config conf/config.yaml

# Test request (404 but middleware should log)
curl -k https://localhost:8443/test

# Check logs - should see HTTP request log:
# {"level":"warn","method":"GET","path":"/test","status":404,"latency":"...","client_ip":"..."}

# Test OPTIONS (CORS preflight)
curl -k -X OPTIONS https://localhost:8443/test
# Expected: 204 No Content with CORS headers
```

**Definition of Done:**
- [x] 4 middleware созданы (logger, recovery, cors, ratelimit)
- [x] Middleware применены в server.go
- [x] HTTP requests логируются
- [ ] CORS headers добавляются
- [ ] Recovery отлавливает panics

---

## 1.5.3: Health Endpoint (1 час)

### Шаг 1: Обновить internal/api/rest/health.go (уже создан в Phase 1.2-1.4)

Убедиться что `health.go` содержит все checks:

```go
package rest

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/your-org/cts-core/internal/db"
    "github.com/your-org/cts-core/internal/hsm"
    "github.com/your-org/cts-core/internal/state"
)

type HealthHandler struct {
    dbClient     *db.MySQLClient
    hsmClient    *hsm.Client
    stateManager *state.Manager
}

func NewHealthHandler(dbClient *db.MySQLClient, hsmClient *hsm.Client, stateManager *state.Manager) *HealthHandler {
    return &HealthHandler{
        dbClient:     dbClient,
        hsmClient:    hsmClient,
        stateManager: stateManager,
    }
}

// Health returns system health status
func (h *HealthHandler) Health(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
    defer cancel()

    // Check database
    dbStatus := "ok"
    if err := h.dbClient.Ping(); err != nil {
        dbStatus = "error: " + err.Error()
    }

    // Check HSM
    hsmStatus := "ok"
    testData := []byte("health-check")
    keyID, ciphertext, err := h.hsmClient.Encrypt(ctx, testData, "health")
    if err != nil {
        hsmStatus = "error: " + err.Error()
    } else {
        _, err = h.hsmClient.Decrypt(ctx, ciphertext, keyID, "health")
        if err != nil {
            hsmStatus = "error: decrypt failed: " + err.Error()
        }
    }

    // Check state
    stateStatus := "ok"
    stateData := h.stateManager.GetState()
    issues := h.stateManager.ValidateState()
    if len(issues) > 0 {
        stateStatus = fmt.Sprintf("validation issues: %d", len(issues))
    }

    response := gin.H{
        "status": "ok",
        "components": gin.H{
            "database": dbStatus,
            "hsm":      hsmStatus,
            "state":    stateStatus,
        },
        "state_info": gin.H{
            "traders":  len(stateData.Traders),
            "sessions": len(stateData.Sessions),
            "orders":   len(stateData.Orders),
            "updated":  stateData.UpdatedAt.Unix(),
        },
        "timestamp": time.Now().Unix(),
    }

    // If any component is down, return 503
    if dbStatus != "ok" || hsmStatus != "ok" || stateStatus != "ok" {
        c.JSON(http.StatusServiceUnavailable, response)
        return
    }

    c.JSON(http.StatusOK, response)
}

// Readiness returns readiness status (lightweight check)
func (h *HealthHandler) Readiness(c *gin.Context) {
    // Quick check - just verify components exist
    response := gin.H{
        "ready":     true,
        "timestamp": time.Now().Unix(),
    }
    
    c.JSON(http.StatusOK, response)
}

// Liveness returns liveness status (very lightweight)
func (h *HealthHandler) Liveness(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "alive": true,
    })
}
```

**Время:** 30 минут

### Шаг 2: Register health routes

Обновить `cmd/daemon/main.go`:

```go
import (
    "github.com/your-org/cts-core/internal/api/rest"
)

func main() {
    // ... (после apiServer)

    // Initialize handlers
    healthHandler := rest.NewHealthHandler(dbClient, hsmClient, stateManager)

    // Register routes
    router := apiServer.Router()
    
    // Health endpoints
    router.GET("/health", healthHandler.Health)
    router.GET("/readiness", healthHandler.Readiness)
    router.GET("/liveness", healthHandler.Liveness)

    // ... (rest of code)
}
```

**Время:** 15 минут

### Верификация 1.5.3

```bash
# Build and run
make build
./bin/cts-core -config conf/config.yaml

# Test health endpoint
curl -k https://localhost:8443/health
# Expected: {"status":"ok","components":{"database":"ok","hsm":"ok","state":"ok"},...}

# Test readiness
curl -k https://localhost:8443/readiness
# Expected: {"ready":true,"timestamp":...}

# Test liveness
curl -k https://localhost:8443/liveness
# Expected: {"alive":true}

# Stop MySQL and test health (should return 503)
systemctl stop mysql
curl -k https://localhost:8443/health
# Expected: {"status":"ok","components":{"database":"error: ..."},...} with 503 status

# Restart MySQL
systemctl start mysql
```

**Definition of Done:**
- [ ] Health handler обновлен (все checks)
- [ ] 3 endpoints зарегистрированы (/health, /readiness, /liveness)
- [ ] /health возвращает 200 когда все OK
- [ ] /health возвращает 503 когда component down
- [ ] /readiness и /liveness работают

---

## 1.5.4: Metrics Endpoint (2 часа)

### Шаг 1: Создать internal/metrics/collector.go

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // HTTP metrics
    HTTPRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cts_http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )

    HTTPRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "cts_http_request_duration_seconds",
            Help:    "HTTP request latency in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "endpoint"},
    )

    // Database metrics
    DBConnectionsActive = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "cts_db_connections_active",
            Help: "Number of active database connections",
        },
    )

    DBQueriesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cts_db_queries_total",
            Help: "Total number of database queries",
        },
        []string{"operation"},
    )

    DBQueryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "cts_db_query_duration_seconds",
            Help:    "Database query latency in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"operation"},
    )

    // HSM metrics
    HSMOperationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cts_hsm_operations_total",
            Help: "Total number of HSM operations",
        },
        []string{"operation", "status"},
    )

    HSMOperationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "cts_hsm_operation_duration_seconds",
            Help:    "HSM operation latency in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"operation"},
    )

    // Business metrics
    ActiveTradersTotal = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "cts_active_traders_total",
            Help: "Number of active traders",
        },
    )

    ActiveSessionsTotal = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "cts_active_sessions_total",
            Help: "Number of active WebSocket sessions",
        },
    )

    OrdersInFlightTotal = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "cts_orders_inflight_total",
            Help: "Number of in-flight orders",
        },
    )

    OrdersProcessedTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cts_orders_processed_total",
            Help: "Total number of orders processed",
        },
        []string{"status"},
    )
)
```

**Время:** 30 минут

### Шаг 2: Создать metrics middleware

Создать `internal/api/middleware/metrics.go`:

```go
package middleware

import (
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/your-org/cts-core/internal/metrics"
)

// Metrics middleware records HTTP metrics
func Metrics() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path
        method := c.Request.Method

        // Process request
        c.Next()

        // Record metrics
        duration := time.Since(start).Seconds()
        status := strconv.Itoa(c.Writer.Status())

        metrics.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
        metrics.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
    }
}
```

**Время:** 20 минут

### Шаг 3: Register metrics endpoint

Обновить `cmd/daemon/main.go`:

```go
import (
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // ... (после register routes)

    // Metrics endpoint (on separate port for security)
    metricsRouter := gin.New()
    metricsRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))

    metricsServer := &http.Server{
        Addr:    ":9090",
        Handler: metricsRouter,
    }

    go func() {
        logger.Info("Starting metrics server on :9090")
        if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("Metrics server failed", "error", err)
        }
    }()

    // ... (rest of code)

    // In shutdown section, add:
    if err := metricsServer.Shutdown(shutdownCtx); err != nil {
        logger.Error("Metrics server shutdown error", "error", err)
    }
}
```

**Время:** 30 минут

### Шаг 4: Apply metrics middleware

Обновить `internal/api/server.go`:

```go
import (
    "github.com/your-org/cts-core/internal/api/middleware"
)

func NewServer(cfg ServerConfig, logger *slog.Logger) (*Server, error) {
    // ...
    
    router.Use(middleware.Recovery(logger))
    router.Use(middleware.Logger(logger))
    router.Use(middleware.Metrics())  // <-- ADD THIS
    router.Use(middleware.CORS())
    
    // ...
}
```

**Время:** 10 минут

### Шаг 5: Update state metrics

Убедиться что `internal/state/metrics.go` уже создан (Phase 1.4) и обновляется в background sync.

**Время:** 10 минут

### Верификация 1.5.4

```bash
# Build and run
make build
./bin/cts-core -config conf/config.yaml

# Test metrics endpoint
curl http://localhost:9090/metrics

# Expected output (many lines):
# ...
# cts_http_requests_total{endpoint="/health",method="GET",status="200"} 1
# cts_http_request_duration_seconds_bucket{endpoint="/health",method="GET",le="0.005"} 1
# cts_active_traders_total 0
# cts_active_sessions_total 0
# cts_orders_inflight_total 0
# ...

# Make some requests to generate metrics
for i in {1..10}; do curl -k https://localhost:8443/health; done

# Check metrics again
curl http://localhost:9090/metrics | grep cts_http_requests_total
# Expected: counter increased
```

**Definition of Done:**
- [ ] `internal/metrics/collector.go` создан (~80 строк)
- [ ] Metrics middleware создан
- [ ] Metrics endpoint зарегистрирован на :9090
- [ ] HTTP metrics собираются
- [ ] State metrics обновляются
- [ ] Prometheus может scrape /metrics

---

## 1.5.5: Integration Tests (2 часа)

### Шаг 1: Создать tests/integration/api_integration_test.go

```go
// +build integration

package integration

import (
    "crypto/tls"
    "encoding/json"
    "net/http"
    "testing"
    "time"
)

const (
    baseURL = "https://localhost:8443"
)

// httpClient creates HTTP client that accepts self-signed certs
func httpClient() *http.Client {
    return &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                InsecureSkipVerify: true,
            },
        },
        Timeout: 5 * time.Second,
    }
}

func TestHealthEndpoint(t *testing.T) {
    client := httpClient()

    resp, err := client.Get(baseURL + "/health")
    if err != nil {
        t.Fatalf("Health request failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }

    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        t.Fatalf("Failed to decode response: %v", err)
    }

    // Check response structure
    if result["status"] != "ok" {
        t.Errorf("Expected status 'ok', got %v", result["status"])
    }

    components, ok := result["components"].(map[string]interface{})
    if !ok {
        t.Fatal("Missing 'components' field")
    }

    // Check each component
    for name, status := range components {
        if status != "ok" {
            t.Logf("Warning: Component %s is not ok: %v", name, status)
        }
    }
}

func TestReadinessEndpoint(t *testing.T) {
    client := httpClient()

    resp, err := client.Get(baseURL + "/readiness")
    if err != nil {
        t.Fatalf("Readiness request failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }

    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        t.Fatalf("Failed to decode response: %v", err)
    }

    if result["ready"] != true {
        t.Errorf("Expected ready=true, got %v", result["ready"])
    }
}

func TestLivenessEndpoint(t *testing.T) {
    client := httpClient()

    resp, err := client.Get(baseURL + "/liveness")
    if err != nil {
        t.Fatalf("Liveness request failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }

    var result map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        t.Fatalf("Failed to decode response: %v", err)
    }

    if result["alive"] != true {
        t.Errorf("Expected alive=true, got %v", result["alive"])
    }
}

func TestMetricsEndpoint(t *testing.T) {
    client := &http.Client{Timeout: 5 * time.Second}

    resp, err := client.Get("http://localhost:9090/metrics")
    if err != nil {
        t.Fatalf("Metrics request failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }

    // Check Content-Type
    contentType := resp.Header.Get("Content-Type")
    if contentType != "text/plain; version=0.0.4; charset=utf-8" {
        t.Logf("Unexpected Content-Type: %s", contentType)
    }
}

func TestCORSHeaders(t *testing.T) {
    client := httpClient()

    req, _ := http.NewRequest("OPTIONS", baseURL+"/health", nil)
    req.Header.Set("Origin", "https://example.com")

    resp, err := client.Do(req)
    if err != nil {
        t.Fatalf("OPTIONS request failed: %v", err)
    }
    defer resp.Body.Close()

    // Check CORS headers
    allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
    if allowOrigin != "*" {
        t.Errorf("Expected Access-Control-Allow-Origin: *, got %s", allowOrigin)
    }

    allowMethods := resp.Header.Get("Access-Control-Allow-Methods")
    if allowMethods == "" {
        t.Error("Missing Access-Control-Allow-Methods header")
    }
}

func TestNotFoundRoute(t *testing.T) {
    client := httpClient()

    resp, err := client.Get(baseURL + "/nonexistent")
    if err != nil {
        t.Fatalf("Request failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusNotFound {
        t.Errorf("Expected status 404, got %d", resp.StatusCode)
    }
}
```

**Время:** 1 hour

### Шаг 2: Update Makefile

Добавить в `Makefile`:

```makefile
# API tests
.PHONY: test-api
test-api:
	@echo "Starting CTS-Core in background..."
	@./bin/cts-core -config conf/config.yaml > /tmp/cts-core.log 2>&1 & echo $$! > /tmp/cts-core.pid
	@sleep 3
	@echo "Running API integration tests..."
	@go test -v -tags=integration ./tests/integration/...
	@echo "Stopping CTS-Core..."
	@kill `cat /tmp/cts-core.pid` || true
	@rm /tmp/cts-core.pid

.PHONY: test-all
test-all: test test-db-test test-hsm test-api
	@echo "All tests passed!"
```

**Время:** 20 минут

### Шаг 3: Create CI test script

Создать `scripts/run-integration-tests.sh`:

```bash
#!/bin/bash
set -e

echo "=== Integration Test Suite ==="

# Build application
echo "Building application..."
make build

# Start services in background
echo "Starting CTS-Core..."
./bin/cts-core -config conf/config.yaml > /tmp/cts-core.log 2>&1 &
CTS_PID=$!
echo "CTS-Core PID: $CTS_PID"

# Wait for server to start
echo "Waiting for server to start..."
for i in {1..30}; do
    if curl -k -s https://localhost:8443/liveness > /dev/null 2>&1; then
        echo "Server is up!"
        break
    fi
    echo "Attempt $i/30..."
    sleep 1
done

# Run tests
echo "Running integration tests..."
go test -v -tags=integration ./tests/integration/... || TEST_FAILED=1

# Cleanup
echo "Stopping CTS-Core..."
kill $CTS_PID || true
wait $CTS_PID 2>/dev/null || true

# Show logs if tests failed
if [ "$TEST_FAILED" = "1" ]; then
    echo "=== CTS-Core Logs ==="
    cat /tmp/cts-core.log
    exit 1
fi

echo "=== All tests passed! ==="
```

```bash
chmod +x scripts/run-integration-tests.sh
```

**Время:** 20 минут

### Верификация 1.5.5

```bash
# Run integration tests (manual)
make build
./bin/cts-core -config conf/config.yaml &
sleep 3
go test -v -tags=integration ./tests/integration/...
killall cts-core

# Or use Makefile target
make test-api

# Or use script
./scripts/run-integration-tests.sh

# Expected output:
# === RUN   TestHealthEndpoint
# --- PASS: TestHealthEndpoint (0.05s)
# === RUN   TestReadinessEndpoint
# --- PASS: TestReadinessEndpoint (0.02s)
# === RUN   TestLivenessEndpoint
# --- PASS: TestLivenessEndpoint (0.02s)
# === RUN   TestMetricsEndpoint
# --- PASS: TestMetricsEndpoint (0.03s)
# === RUN   TestCORSHeaders
# --- PASS: TestCORSHeaders (0.02s)
# === RUN   TestNotFoundRoute
# --- PASS: TestNotFoundRoute (0.02s)
# PASS
```

**Definition of Done:**
- [ ] Integration tests созданы (6 test cases)
- [ ] Makefile обновлен (test-api target)
- [ ] Test script создан
- [ ] All tests проходят
- [ ] CI-ready

---

## Troubleshooting

### Проблема: "bind: address already in use"

**Причина:** Порт 8443 или 9090 уже занят.

**Решение:**
```bash
# Найти процесс
lsof -i :8443
lsof -i :9090

# Убить процесс
kill <PID>

# Or change port in config.yaml
```

### Проблема: "x509: certificate has expired"

**Причина:** TLS certificates истекли.

**Решение:**
```bash
# Re-generate certificates
cd pki/scripts
./issue-server-cert.sh

# Restart CTS-Core
```

### Проблема: curl returns "SSL certificate problem"

**Причина:** Self-signed certificate.

**Решение:**
```bash
# Use -k flag to ignore certificate validation
curl -k https://localhost:8443/health

# Or add CA to system trust store (production)
```

### Проблема: Middleware не работает

**Причина:** Middleware применен после регистрации routes.

**Решение:**
- Apply middleware BEFORE registering routes
- Check order in server.go NewServer()

### Проблема: Metrics не обновляются

**Причина:** Metrics middleware не применен или не registered.

**Решение:**
1. Check middleware order in server.go
2. Verify metrics endpoint доступен:
   ```bash
   curl http://localhost:9090/metrics
   ```

---

## FAQ

**Q: Почему TLS порт отличается от metrics порт?**
A: Security best practice - metrics endpoint не должен быть publicly exposed. Обычно metrics доступны только internal network.

**Q: Нужен ли mTLS для Phase 1?**
A: Нет, для Phase 1 достаточно TLS. mTLS включим в Phase 2 когда будет WebSocket с traders.

**Q: Как добавить новый endpoint?**
A:
1. Create handler in `internal/api/rest/`
2. Register route in `main.go`:
   ```go
   router.GET("/my-endpoint", handler.MyEndpoint)
   ```

**Q: Как организовать routes?**
A: Используйте Gin route groups:
```go
v1 := router.Group("/api/v1")
{
    v1.GET("/traders", traderHandler.List)
    v1.POST("/orders", orderHandler.Create)
}
```

**Q: Rate limiting на production?**
A: Включить в `server.go`:
```go
router.Use(middleware.RateLimit("1000-M")) // 1000 req/min
```

**Q: Graceful shutdown работает?**
A: Да, `server.Shutdown()` ждет до 10 секунд для завершения активных requests.

**Q: Как мониторить API в production?**
A:
1. Prometheus scrape `/metrics` каждые 15s
2. Grafana dashboards для визуализации
3. Alerting на high error rates / latency

---

## Summary Phase 1.5

**Созданные файлы:**
- `internal/api/server.go` (~180 строк)
- `internal/api/middleware/logger.go` (~40 строк)
- `internal/api/middleware/recovery.go` (~30 строк)
- `internal/api/middleware/cors.go` (~20 строк)
- `internal/api/middleware/ratelimit.go` (~40 строк)
- `internal/api/middleware/metrics.go` (~25 строк)
- `internal/metrics/collector.go` (~80 строк)
- `tests/integration/api_integration_test.go` (~200 строк)
- `scripts/run-integration-tests.sh` (~40 строк)

**Total LOC:** ~655 строк

**Обновленные файлы:**
- `internal/config/types.go` (добавлен ServerConfig)
- `conf/config.yaml` (добавлена server секция)
- `cmd/daemon/main.go` (добавлен API server + graceful shutdown)
- `internal/api/rest/health.go` (обновлен - все checks)
- `Makefile` (test-api, test-all targets)

**Deliverables:**
✅ Gin HTTP server с TLS  
✅ 5 middleware (logger, recovery, cors, ratelimit, metrics)  
✅ Health endpoints (/health, /readiness, /liveness)  
✅ Metrics endpoint (/metrics на :9090)  
✅ Prometheus metrics (HTTP, DB, HSM, business)  
✅ Graceful shutdown  
✅ Integration tests (6 test cases)  
✅ CI-ready test script  

**Next Phase:** Phase 2 - WebSocket & Session Management

---

## Definition of Done - Phase 1.5

- [ ] Все файлы созданы и скомпилированы
- [ ] `make build` проходит без ошибок
- [ ] Server запускается на :8443 с TLS
- [ ] Metrics server запускается на :9090
- [ ] /health endpoint возвращает 200 OK
- [ ] /metrics endpoint работает
- [ ] Integration tests проходят (`make test-api`)
- [ ] Graceful shutdown работает (Ctrl+C)
- [ ] CORS headers добавляются
- [ ] HTTP requests логируются
- [ ] Закоммичено в git:
  ```bash
  git add internal/api/ internal/metrics/ internal/config/ conf/config.yaml cmd/daemon/main.go tests/integration/ scripts/ Makefile
  git commit -m "Phase 1.5: REST API server with middleware and metrics"
  ```
- [ ] `guides/phase_1_5_rest_server.md` удален (после завершения фазы)

---

## Congratulations! 🎉

**Phase 1 Complete!**

Вы завершили Phase 1 CTS-Core:
- ✅ Phase 0: Database migrations (11 tables)
- ✅ Phase 1.1: Project setup (config, logger, main.go)
- ✅ Phase 1.2: MySQL connection pool (mTLS, retry, repository)
- ✅ Phase 1.3: HSM client (encrypt/decrypt, mTLS)
- ✅ Phase 1.4: State management (daemon.state + MySQL sync)
- ✅ Phase 1.5: REST API server (Gin, middleware, metrics)

**Что теперь работает:**
- CTS-Core запускается как HTTP/HTTPS server
- Подключается к MySQL с mTLS
- Подключается к HSM service с mTLS
- Сохраняет state в файл + MySQL
- Отдает /health, /readiness, /liveness
- Экспортирует Prometheus metrics
- Graceful shutdown

**Next Steps:**
- Phase 2: WebSocket server и session management
- Phase 3: Trader integration и heartbeat monitoring
- Phase 4: Order routing и arbitrage logic

**Запустите CTS-Core:**
```bash
make build
./bin/cts-core -config conf/config.yaml

# В другом терминале:
curl -k https://localhost:8443/health
curl http://localhost:9090/metrics
```

Готовы к Phase 2! 🚀
