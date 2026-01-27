# Phase 1.3: HSM Client

**Цель:** Создать mTLS HTTP client для взаимодействия с hsm-service (encrypt/decrypt operations).

**Время:** ~2 дня (12-16 часов)

**Зависимости:**
- ✅ Phase 1.1 завершена (config, logger готовы)
- hsm-service запущен и доступен (https://localhost:8200)
- HSM client certificates готовы (conf/ssl/hsm-client-cert.pem, hsm-client-key.pem, ca-cert.pem)

---

## 1.3.1: HSM Client Types и Configuration (2 часа)

### Шаг 1: Создать internal/hsm/types.go

```go
package hsm

import (
    "encoding/base64"
)

// EncryptRequest represents /encrypt API request
type EncryptRequest struct {
    Context   string `json:"context"`   // e.g., "2fa", "api_key"
    Plaintext string `json:"plaintext"` // base64-encoded data
}

// EncryptResponse represents /encrypt API response
type EncryptResponse struct {
    KeyID      string `json:"key_id"`      // e.g., "kek-2fa-v1"
    Ciphertext string `json:"ciphertext"`  // base64-encoded encrypted data
    Error      string `json:"error,omitempty"`
}

// DecryptRequest represents /decrypt API request
type DecryptRequest struct {
    Context    string `json:"context"`    // e.g., "2fa"
    KeyID      string `json:"key_id"`     // e.g., "kek-2fa-v1"
    Ciphertext string `json:"ciphertext"` // base64-encoded encrypted data
}

// DecryptResponse represents /decrypt API response
type DecryptResponse struct {
    Plaintext string `json:"plaintext"` // base64-encoded decrypted data
    Error     string `json:"error,omitempty"`
}

// HelperFunctions for base64 encoding/decoding

// EncodeBase64 encodes byte slice to base64 string
func EncodeBase64(data []byte) string {
    return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 decodes base64 string to byte slice
func DecodeBase64(encoded string) ([]byte, error) {
    return base64.StdEncoding.DecodeString(encoded)
}
```

**Время:** 30 минут

### Шаг 2: Добавить HSM config в types.go

Добавить в `internal/config/types.go`:

```go
type HSMConfig struct {
    BaseURL string `yaml:"base_url"` // https://localhost:8200
    
    TLS struct {
        CertPath string `yaml:"cert"`
        KeyPath  string `yaml:"key"`
        CAPath   string `yaml:"ca"`
    } `yaml:"tls"`
    
    Timeout struct {
        Connect int `yaml:"connect_seconds"`
        Request int `yaml:"request_seconds"`
    } `yaml:"timeout"`
    
    Retry struct {
        MaxAttempts int `yaml:"max_attempts"`
        InitialWait int `yaml:"initial_wait_ms"`
        MaxWait     int `yaml:"max_wait_seconds"`
        Multiplier  float64 `yaml:"multiplier"`
    } `yaml:"retry"`
}
```

### Шаг 3: Обновить conf/config.yaml

Добавить секцию:

```yaml
hsm:
  base_url: https://localhost:8200
  tls:
    cert: conf/ssl/hsm-client-cert.pem
    key: conf/ssl/hsm-client-key.pem
    ca: conf/ssl/ca-cert.pem
  timeout:
    connect_seconds: 5
    request_seconds: 10
  retry:
    max_attempts: 5
    initial_wait_ms: 200
    max_wait_seconds: 10
    multiplier: 2.0
```

**Время:** 30 минут

### Шаг 4: Обновить internal/config/config.go

Добавить поле HSM:

```go
type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Database DatabaseConfig `yaml:"database"`
    HSM      HSMConfig      `yaml:"hsm"`      // <-- ADD THIS
    Logging  LoggingConfig  `yaml:"logging"`
}
```

**Время:** 10 минут

### Верификация 1.3.1

```bash
# Скомпилировать
make build

# Проверить что config загружается
./bin/cts-core -config conf/config.yaml -validate

# Ожидаемый вывод:
# Configuration valid
```

**Definition of Done:**
- [ ] `internal/hsm/types.go` создан (~50 строк)
- [ ] HSMConfig добавлен в config
- [ ] config.yaml обновлен (hsm секция)
- [ ] `make build` проходит без ошибок

---

## 1.3.2: mTLS HTTP Client (3 часа)

### Шаг 1: Создать internal/hsm/client.go

```go
package hsm

import (
    "bytes"
    "context"
    "crypto/tls"
    "crypto/x509"
    "encoding/json"
    "fmt"
    "io"
    "math"
    "net/http"
    "os"
    "time"

    "github.com/rs/zerolog"
)

type Client struct {
    baseURL    string
    httpClient *http.Client
    logger     *zerolog.Logger
    retryCfg   RetryConfig
}

type RetryConfig struct {
    MaxAttempts int
    InitialWait time.Duration
    MaxWait     time.Duration
    Multiplier  float64
}

type ClientConfig struct {
    BaseURL  string
    CertPath string
    KeyPath  string
    CAPath   string
    
    ConnectTimeout time.Duration
    RequestTimeout time.Duration
    
    RetryConfig RetryConfig
}

// NewClient creates new HSM client with mTLS
func NewClient(cfg ClientConfig, logger *zerolog.Logger) (*Client, error) {
    // Load client certificate
    cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load client cert: %w", err)
    }

    // Load CA certificate
    caCert, err := os.ReadFile(cfg.CAPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read CA cert: %w", err)
    }

    caCertPool := x509.NewCertPool()
    if !caCertPool.AppendCertsFromPEM(caCert) {
        return nil, fmt.Errorf("failed to append CA cert")
    }

    // Configure TLS
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      caCertPool,
        MinVersion:   tls.VersionTLS12,
    }

    // Create HTTP client with custom transport
    transport := &http.Transport{
        TLSClientConfig:     tlsConfig,
        MaxIdleConns:        10,
        MaxIdleConnsPerHost: 5,
        IdleConnTimeout:     90 * time.Second,
        DisableKeepAlives:   false,
    }

    httpClient := &http.Client{
        Transport: transport,
        Timeout:   cfg.RequestTimeout,
    }

    logger.Info().Msgf("HSM client initialized: %s (mTLS enabled)", cfg.BaseURL)

    return &Client{
        baseURL:    cfg.BaseURL,
        httpClient: httpClient,
        logger:     logger,
        retryCfg:   cfg.RetryConfig,
    }, nil
}

// Close closes HTTP client connections
func (c *Client) Close() error {
    c.httpClient.CloseIdleConnections()
    return nil
}

// doRequest performs HTTP request with retry logic
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, response interface{}) error {
    var lastErr error

    for attempt := 1; attempt <= c.retryCfg.MaxAttempts; attempt++ {
        // Marshal request body
        var reqBody []byte
        var err error
        if body != nil {
            reqBody, err = json.Marshal(body)
            if err != nil {
                return fmt.Errorf("failed to marshal request: %w", err)
            }
        }

        // Create HTTP request
        url := c.baseURL + path
        req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBody))
        if err != nil {
            return fmt.Errorf("failed to create request: %w", err)
        }

        req.Header.Set("Content-Type", "application/json")

        // Execute request
        resp, err := c.httpClient.Do(req)
        if err != nil {
            lastErr = fmt.Errorf("HTTP request failed: %w", err)
            c.logRetry(attempt, lastErr)
            
            if attempt < c.retryCfg.MaxAttempts {
                c.waitBeforeRetry(ctx, attempt)
                continue
            }
            break
        }

        // Read response body
        defer resp.Body.Close()
        respBody, err := io.ReadAll(resp.Body)
        if err != nil {
            lastErr = fmt.Errorf("failed to read response: %w", err)
            c.logRetry(attempt, lastErr)
            
            if attempt < c.retryCfg.MaxAttempts {
                c.waitBeforeRetry(ctx, attempt)
                continue
            }
            break
        }

        // Check status code
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
            c.logRetry(attempt, lastErr)
            
            // Don't retry on 4xx errors (client errors)
            if resp.StatusCode >= 400 && resp.StatusCode < 500 {
                return lastErr
            }
            
            if attempt < c.retryCfg.MaxAttempts {
                c.waitBeforeRetry(ctx, attempt)
                continue
            }
            break
        }

        // Unmarshal response
        if response != nil {
            if err := json.Unmarshal(respBody, response); err != nil {
                return fmt.Errorf("failed to unmarshal response: %w", err)
            }
        }

        // Success
        return nil
    }

    return fmt.Errorf("request failed after %d attempts: %w", c.retryCfg.MaxAttempts, lastErr)
}

// logRetry logs retry attempt
func (c *Client) logRetry(attempt int, err error) {
    if attempt < c.retryCfg.MaxAttempts {
        wait := c.calculateBackoff(attempt)
        c.logger.Warn().
            Err(err).
            Int("attempt", attempt).
            Dur("retry_in", wait).
            Msg("HSM request failed, retrying...")
    }
}

// waitBeforeRetry waits before next retry attempt
func (c *Client) waitBeforeRetry(ctx context.Context, attempt int) {
    wait := c.calculateBackoff(attempt)
    
    select {
    case <-time.After(wait):
        // Continue to next attempt
    case <-ctx.Done():
        // Context cancelled
    }
}

// calculateBackoff calculates exponential backoff delay
func (c *Client) calculateBackoff(attempt int) time.Duration {
    wait := time.Duration(float64(c.retryCfg.InitialWait) * math.Pow(c.retryCfg.Multiplier, float64(attempt-1)))
    if wait > c.retryCfg.MaxWait {
        wait = c.retryCfg.MaxWait
    }
    return wait
}
```

**Время:** 2 часа

### Шаг 2: Добавить unit tests для retry logic

Создать `internal/hsm/client_test.go`:

```go
package hsm

import (
    "context"
    "testing"
    "time"

    "github.com/rs/zerolog"
)

func TestCalculateBackoff(t *testing.T) {
    logger := zerolog.Nop()
    
    client := &Client{
        logger: &logger,
        retryCfg: RetryConfig{
            MaxAttempts: 5,
            InitialWait: 100 * time.Millisecond,
            MaxWait:     5 * time.Second,
            Multiplier:  2.0,
        },
    }

    tests := []struct {
        attempt  int
        expected time.Duration
    }{
        {1, 100 * time.Millisecond},  // 100ms * 2^0
        {2, 200 * time.Millisecond},  // 100ms * 2^1
        {3, 400 * time.Millisecond},  // 100ms * 2^2
        {4, 800 * time.Millisecond},  // 100ms * 2^3
        {5, 1600 * time.Millisecond}, // 100ms * 2^4
        {10, 5 * time.Second},        // Capped at MaxWait
    }

    for _, tt := range tests {
        result := client.calculateBackoff(tt.attempt)
        if result != tt.expected {
            t.Errorf("Attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
        }
    }
}
```

**Время:** 30 минут

### Верификация 1.3.2

```bash
# Run unit tests
go test -v ./internal/hsm/...

# Ожидаемый вывод:
# === RUN   TestCalculateBackoff
# --- PASS: TestCalculateBackoff (0.00s)
# PASS
```

**Definition of Done:**
- [ ] `internal/hsm/client.go` создан (~200 строк)
- [ ] mTLS HTTP client реализован
- [ ] Retry logic с exponential backoff
- [ ] Unit tests для backoff logic
- [ ] Tests проходят

---

## 1.3.3: Encrypt/Decrypt Methods (2 часа)

### Шаг 1: Добавить Encrypt method в client.go

Добавить в `internal/hsm/client.go`:

```go
// Encrypt encrypts plaintext using HSM
// plaintext - raw bytes to encrypt
// context - encryption context (e.g., "2fa", "api_key")
// Returns: keyID, ciphertext (both strings), error
func (c *Client) Encrypt(ctx context.Context, plaintext []byte, context string) (keyID string, ciphertext string, err error) {
    // Encode plaintext to base64
    plaintextB64 := EncodeBase64(plaintext)

    req := EncryptRequest{
        Context:   context,
        Plaintext: plaintextB64,
    }

    var resp EncryptResponse
    err = c.doRequest(ctx, "POST", "/encrypt", req, &resp)
    if err != nil {
        return "", "", fmt.Errorf("encrypt request failed: %w", err)
    }

    // Check for HSM error
    if resp.Error != "" {
        return "", "", fmt.Errorf("HSM encrypt error: %s", resp.Error)
    }

    c.logger.Debug().
        Str("context", context).
        Str("key_id", resp.KeyID).
        Msg("HSM encrypt successful")

    return resp.KeyID, resp.Ciphertext, nil
}
```

### Шаг 2: Добавить Decrypt method

```go
// Decrypt decrypts ciphertext using HSM
// ciphertext - base64-encoded encrypted data
// keyID - key identifier (e.g., "kek-2fa-v1")
// context - encryption context (must match encrypt context)
// Returns: plaintext bytes, error
func (c *Client) Decrypt(ctx context.Context, ciphertext, keyID, context string) ([]byte, error) {
    req := DecryptRequest{
        Context:    context,
        KeyID:      keyID,
        Ciphertext: ciphertext,
    }

    var resp DecryptResponse
    err := c.doRequest(ctx, "POST", "/decrypt", req, &resp)
    if err != nil {
        return nil, fmt.Errorf("decrypt request failed: %w", err)
    }

    // Check for HSM error
    if resp.Error != "" {
        return nil, fmt.Errorf("HSM decrypt error: %s", resp.Error)
    }

    // Decode base64 plaintext
    plaintext, err := DecodeBase64(resp.Plaintext)
    if err != nil {
        return nil, fmt.Errorf("failed to decode plaintext: %w", err)
    }

    c.logger.Debug().
        Str("context", context).
        Str("key_id", keyID).
        Msg("HSM decrypt successful")

    return plaintext, nil
}
```

**Время:** 1 час

### Шаг 3: Добавить helper methods

```go
// Encrypt2FA encrypts 2FA secret (convenience wrapper)
func (c *Client) Encrypt2FA(ctx context.Context, secret string) (keyID, ciphertext string, err error) {
    return c.Encrypt(ctx, []byte(secret), "2fa")
}

// Decrypt2FA decrypts 2FA secret (convenience wrapper)
func (c *Client) Decrypt2FA(ctx context.Context, ciphertext, keyID string) (string, error) {
    plaintext, err := c.Decrypt(ctx, ciphertext, keyID, "2fa")
    if err != nil {
        return "", err
    }
    return string(plaintext), nil
}

// EncryptAPIKey encrypts API key (convenience wrapper)
func (c *Client) EncryptAPIKey(ctx context.Context, apiKey string) (keyID, ciphertext string, err error) {
    return c.Encrypt(ctx, []byte(apiKey), "api_key")
}

// DecryptAPIKey decrypts API key (convenience wrapper)
func (c *Client) DecryptAPIKey(ctx context.Context, ciphertext, keyID string) (string, error) {
    plaintext, err := c.Decrypt(ctx, ciphertext, keyID, "api_key")
    if err != nil {
        return "", err
    }
    return string(plaintext), nil
}
```

**Время:** 30 минут

### Шаг 4: Добавить mock HSM server для tests

Создать `internal/hsm/mock_server_test.go`:

```go
package hsm

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
)

// MockHSMServer creates mock HSM server for testing
func MockHSMServer() *httptest.Server {
    mux := http.NewServeMux()

    // Mock /encrypt endpoint
    mux.HandleFunc("/encrypt", func(w http.ResponseWriter, r *http.Request) {
        var req EncryptRequest
        json.NewDecoder(r.Body).Decode(&req)

        resp := EncryptResponse{
            KeyID:      "kek-" + req.Context + "-v1",
            Ciphertext: "mock_encrypted_" + req.Plaintext,
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(resp)
    })

    // Mock /decrypt endpoint
    mux.HandleFunc("/decrypt", func(w http.ResponseWriter, r *http.Request) {
        var req DecryptRequest
        json.NewDecoder(r.Body).Decode(&req)

        // Remove "mock_encrypted_" prefix
        plaintext := req.Ciphertext[15:]

        resp := DecryptResponse{
            Plaintext: plaintext,
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(resp)
    })

    return httptest.NewTLSServer(mux)
}
```

### Шаг 5: Добавить integration tests

Добавить в `internal/hsm/client_test.go`:

```go
func TestEncryptDecrypt(t *testing.T) {
    // Create mock HSM server
    server := MockHSMServer()
    defer server.Close()

    logger := zerolog.Nop()

    // Create client (using mock server certificates)
    client := &Client{
        baseURL:    server.URL,
        httpClient: server.Client(),
        logger:     &logger,
        retryCfg: RetryConfig{
            MaxAttempts: 3,
            InitialWait: 100 * time.Millisecond,
            MaxWait:     5 * time.Second,
            Multiplier:  2.0,
        },
    }

    ctx := context.Background()

    // Test Encrypt
    plaintext := []byte("my-secret-data")
    keyID, ciphertext, err := client.Encrypt(ctx, plaintext, "2fa")
    if err != nil {
        t.Fatalf("Encrypt failed: %v", err)
    }

    if keyID == "" {
        t.Fatal("Expected non-empty keyID")
    }

    if ciphertext == "" {
        t.Fatal("Expected non-empty ciphertext")
    }

    t.Logf("KeyID: %s", keyID)
    t.Logf("Ciphertext: %s", ciphertext)

    // Test Decrypt
    decrypted, err := client.Decrypt(ctx, ciphertext, keyID, "2fa")
    if err != nil {
        t.Fatalf("Decrypt failed: %v", err)
    }

    if string(decrypted) != string(plaintext) {
        t.Errorf("Expected %s, got %s", plaintext, decrypted)
    }
}

func TestEncrypt2FA(t *testing.T) {
    server := MockHSMServer()
    defer server.Close()

    logger := zerolog.Nop()

    client := &Client{
        baseURL:    server.URL,
        httpClient: server.Client(),
        logger:     &logger,
        retryCfg: RetryConfig{
            MaxAttempts: 3,
            InitialWait: 100 * time.Millisecond,
            MaxWait:     5 * time.Second,
            Multiplier:  2.0,
        },
    }

    ctx := context.Background()

    // Test 2FA helpers
    secret := "JBSWY3DPEHPK3PXP"
    keyID, ciphertext, err := client.Encrypt2FA(ctx, secret)
    if err != nil {
        t.Fatalf("Encrypt2FA failed: %v", err)
    }

    decrypted, err := client.Decrypt2FA(ctx, ciphertext, keyID)
    if err != nil {
        t.Fatalf("Decrypt2FA failed: %v", err)
    }

    if decrypted != secret {
        t.Errorf("Expected %s, got %s", secret, decrypted)
    }
}
```

**Время:** 30 минут

### Верификация 1.3.3

```bash
# Run tests with mock server
go test -v ./internal/hsm/...

# Ожидаемый вывод:
# === RUN   TestEncryptDecrypt
# --- PASS: TestEncryptDecrypt (0.01s)
# === RUN   TestEncrypt2FA
# --- PASS: TestEncrypt2FA (0.01s)
# PASS
```

**Definition of Done:**
- [ ] Encrypt() и Decrypt() методы реализованы
- [ ] Helper methods (Encrypt2FA, DecryptAPIKey) созданы
- [ ] Mock HSM server для tests
- [ ] Integration tests проходят

---

## 1.3.4: Интеграция в main.go (1 час)

### Шаг 1: Обновить cmd/daemon/main.go

```go
import (
    "github.com/your-org/cts-core/internal/hsm"
    "time"
)

func main() {
    // ... (после repo)

    // Initialize HSM client
    hsmCfg := hsm.ClientConfig{
        BaseURL:  cfg.HSM.BaseURL,
        CertPath: cfg.HSM.TLS.CertPath,
        KeyPath:  cfg.HSM.TLS.KeyPath,
        CAPath:   cfg.HSM.TLS.CAPath,
        ConnectTimeout: time.Duration(cfg.HSM.Timeout.Connect) * time.Second,
        RequestTimeout: time.Duration(cfg.HSM.Timeout.Request) * time.Second,
        RetryConfig: hsm.RetryConfig{
            MaxAttempts: cfg.HSM.Retry.MaxAttempts,
            InitialWait: time.Duration(cfg.HSM.Retry.InitialWait) * time.Millisecond,
            MaxWait:     time.Duration(cfg.HSM.Retry.MaxWait) * time.Second,
            Multiplier:  cfg.HSM.Retry.Multiplier,
        },
    }

    hsmClient, err := hsm.NewClient(hsmCfg, logger)
    if err != nil {
        logger.Fatal().Err(err).Msg("Failed to create HSM client")
    }
    defer hsmClient.Close()

    // Test HSM connection
    ctx := context.Background()
    testPlaintext := []byte("test")
    keyID, ciphertext, err := hsmClient.Encrypt(ctx, testPlaintext, "test")
    if err != nil {
        logger.Fatal().Err(err).Msg("HSM connection test failed")
    }

    decrypted, err := hsmClient.Decrypt(ctx, ciphertext, keyID, "test")
    if err != nil {
        logger.Fatal().Err(err).Msg("HSM decryption test failed")
    }

    if string(decrypted) != string(testPlaintext) {
        logger.Fatal().Msg("HSM test failed: decrypted data mismatch")
    }

    logger.Info().Msg("HSM client connected and tested successfully")

    // ... rest of application
}
```

**Время:** 30 минут

### Шаг 2: Обновить health check

Обновить `internal/api/rest/health.go`:

```go
import (
    "github.com/your-org/cts-core/internal/hsm"
)

type HealthHandler struct {
    dbClient  *db.MySQLClient
    hsmClient *hsm.Client
}

func NewHealthHandler(dbClient *db.MySQLClient, hsmClient *hsm.Client) *HealthHandler {
    return &HealthHandler{
        dbClient:  dbClient,
        hsmClient: hsmClient,
    }
}

func (h *HealthHandler) Health(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
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
        // Try decrypt to ensure full round-trip works
        _, err = h.hsmClient.Decrypt(ctx, ciphertext, keyID, "health")
        if err != nil {
            hsmStatus = "error: decrypt failed: " + err.Error()
        }
    }

    response := gin.H{
        "status": "ok",
        "components": gin.H{
            "database": dbStatus,
            "hsm":      hsmStatus,
        },
        "timestamp": time.Now().Unix(),
    }

    // If any component is down, return 503
    if dbStatus != "ok" || hsmStatus != "ok" {
        c.JSON(http.StatusServiceUnavailable, response)
        return
    }

    c.JSON(http.StatusOK, response)
}
```

**Время:** 20 минут

### Верификация 1.3.4

```bash
# Убедиться что hsm-service запущен
curl -k https://localhost:8200/health
# Expected: {"status":"ok"}

# Build and run CTS-Core
make build
./bin/cts-core -config conf/config.yaml

# Ожидаемый вывод:
# {"level":"info","message":"HSM client initialized: https://localhost:8200 (mTLS enabled)"}
# {"level":"info","message":"HSM client connected and tested successfully"}

# Test health endpoint
curl -k https://localhost:8443/health
# Expected: {"status":"ok","components":{"database":"ok","hsm":"ok"},"timestamp":...}
```

**Definition of Done:**
- [ ] HSM client интегрирован в main.go
- [ ] Health check обновлен (database + HSM)
- [ ] `make build` проходит
- [ ] HSM connection test успешен при запуске
- [ ] Health endpoint возвращает hsm status

---

## 1.3.5: Real HSM Integration Test (2 часа)

### Шаг 1: Убедиться что hsm-service запущен

```bash
# Navigate to hsm-service
cd ../other-sub-system/hsm-service

# Check if running
curl -k https://localhost:8200/health

# If not running, start it:
make run
# Or:
./hsm-service -config config.yaml
```

**Время:** 10 минут

### Шаг 2: Создать integration test файл

Создать `tests/integration/hsm_integration_test.go`:

```go
// +build integration

package integration

import (
    "context"
    "testing"
    "time"

    "github.com/your-org/cts-core/internal/hsm"
    "github.com/rs/zerolog"
)

func TestRealHSMEncryptDecrypt(t *testing.T) {
    logger := zerolog.Nop()

    cfg := hsm.ClientConfig{
        BaseURL:  "https://localhost:8200",
        CertPath: "../../conf/ssl/hsm-client-cert.pem",
        KeyPath:  "../../conf/ssl/hsm-client-key.pem",
        CAPath:   "../../conf/ssl/ca-cert.pem",
        ConnectTimeout: 5 * time.Second,
        RequestTimeout: 10 * time.Second,
        RetryConfig: hsm.RetryConfig{
            MaxAttempts: 3,
            InitialWait: 200 * time.Millisecond,
            MaxWait:     5 * time.Second,
            Multiplier:  2.0,
        },
    }

    client, err := hsm.NewClient(cfg, &logger)
    if err != nil {
        t.Fatalf("Failed to create HSM client: %v", err)
    }
    defer client.Close()

    ctx := context.Background()

    // Test 1: Basic encrypt/decrypt
    t.Run("BasicEncryptDecrypt", func(t *testing.T) {
        plaintext := []byte("Hello, HSM!")
        
        keyID, ciphertext, err := client.Encrypt(ctx, plaintext, "test")
        if err != nil {
            t.Fatalf("Encrypt failed: %v", err)
        }

        t.Logf("KeyID: %s", keyID)
        t.Logf("Ciphertext length: %d", len(ciphertext))

        decrypted, err := client.Decrypt(ctx, ciphertext, keyID, "test")
        if err != nil {
            t.Fatalf("Decrypt failed: %v", err)
        }

        if string(decrypted) != string(plaintext) {
            t.Errorf("Mismatch: expected %s, got %s", plaintext, decrypted)
        }
    })

    // Test 2: 2FA secret encryption
    t.Run("Encrypt2FA", func(t *testing.T) {
        secret := "JBSWY3DPEHPK3PXP" // TOTP secret
        
        keyID, ciphertext, err := client.Encrypt2FA(ctx, secret)
        if err != nil {
            t.Fatalf("Encrypt2FA failed: %v", err)
        }

        // KeyID should contain "2fa"
        if keyID != "kek-2fa-v1" {
            t.Logf("Warning: Unexpected keyID format: %s", keyID)
        }

        decrypted, err := client.Decrypt2FA(ctx, ciphertext, keyID)
        if err != nil {
            t.Fatalf("Decrypt2FA failed: %v", err)
        }

        if decrypted != secret {
            t.Errorf("Mismatch: expected %s, got %s", secret, decrypted)
        }
    })

    // Test 3: API Key encryption
    t.Run("EncryptAPIKey", func(t *testing.T) {
        apiKey := "sk_live_1234567890abcdef"
        
        keyID, ciphertext, err := client.EncryptAPIKey(ctx, apiKey)
        if err != nil {
            t.Fatalf("EncryptAPIKey failed: %v", err)
        }

        decrypted, err := client.DecryptAPIKey(ctx, ciphertext, keyID)
        if err != nil {
            t.Fatalf("DecryptAPIKey failed: %v", err)
        }

        if decrypted != apiKey {
            t.Errorf("Mismatch: expected %s, got %s", apiKey, decrypted)
        }
    })

    // Test 4: Large data encryption
    t.Run("LargeData", func(t *testing.T) {
        // 1KB of data
        plaintext := make([]byte, 1024)
        for i := range plaintext {
            plaintext[i] = byte(i % 256)
        }

        keyID, ciphertext, err := client.Encrypt(ctx, plaintext, "test")
        if err != nil {
            t.Fatalf("Encrypt failed: %v", err)
        }

        decrypted, err := client.Decrypt(ctx, ciphertext, keyID, "test")
        if err != nil {
            t.Fatalf("Decrypt failed: %v", err)
        }

        if len(decrypted) != len(plaintext) {
            t.Errorf("Length mismatch: expected %d, got %d", len(plaintext), len(decrypted))
        }
    })

    // Test 5: Wrong context should fail
    t.Run("WrongContext", func(t *testing.T) {
        plaintext := []byte("test")
        
        keyID, ciphertext, err := client.Encrypt(ctx, plaintext, "2fa")
        if err != nil {
            t.Fatalf("Encrypt failed: %v", err)
        }

        // Try to decrypt with wrong context
        _, err = client.Decrypt(ctx, ciphertext, keyID, "api_key")
        if err == nil {
            t.Error("Expected error when using wrong context, got nil")
        }
    })
}

func TestHSMRetry(t *testing.T) {
    logger := zerolog.Nop()

    // Use invalid URL to trigger retries
    cfg := hsm.ClientConfig{
        BaseURL:  "https://invalid-host:8200",
        CertPath: "../../conf/ssl/hsm-client-cert.pem",
        KeyPath:  "../../conf/ssl/hsm-client-key.pem",
        CAPath:   "../../conf/ssl/ca-cert.pem",
        ConnectTimeout: 1 * time.Second,
        RequestTimeout: 2 * time.Second,
        RetryConfig: hsm.RetryConfig{
            MaxAttempts: 3,
            InitialWait: 100 * time.Millisecond,
            MaxWait:     1 * time.Second,
            Multiplier:  2.0,
        },
    }

    client, err := hsm.NewClient(cfg, &logger)
    if err != nil {
        t.Fatalf("Failed to create HSM client: %v", err)
    }
    defer client.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Should fail after 3 retries
    _, _, err = client.Encrypt(ctx, []byte("test"), "test")
    if err == nil {
        t.Error("Expected error with invalid host, got nil")
    }

    t.Logf("Expected error: %v", err)
}
```

**Время:** 1 час

### Шаг 3: Запустить integration tests

```bash
# Убедиться что hsm-service запущен
curl -k https://localhost:8200/health

# Run integration tests
go test -v -tags=integration ./tests/integration/...

# Ожидаемый вывод:
# === RUN   TestRealHSMEncryptDecrypt
# === RUN   TestRealHSMEncryptDecrypt/BasicEncryptDecrypt
# --- PASS: TestRealHSMEncryptDecrypt/BasicEncryptDecrypt (0.05s)
# === RUN   TestRealHSMEncryptDecrypt/Encrypt2FA
# --- PASS: TestRealHSMEncryptDecrypt/Encrypt2FA (0.03s)
# === RUN   TestRealHSMEncryptDecrypt/EncryptAPIKey
# --- PASS: TestRealHSMEncryptDecrypt/EncryptAPIKey (0.03s)
# === RUN   TestRealHSMEncryptDecrypt/LargeData
# --- PASS: TestRealHSMEncryptDecrypt/LargeData (0.04s)
# === RUN   TestRealHSMEncryptDecrypt/WrongContext
# --- PASS: TestRealHSMEncryptDecrypt/WrongContext (0.03s)
# === RUN   TestHSMRetry
# --- PASS: TestHSMRetry (3.21s)
# PASS
```

**Время:** 30 минут

### Шаг 4: Обновить Makefile

Добавить в `Makefile`:

```makefile
# HSM tests
.PHONY: test-hsm-unit
test-hsm-unit:
	@go test -v ./internal/hsm/...

.PHONY: test-hsm-integration
test-hsm-integration:
	@go test -v -tags=integration ./tests/integration/...

.PHONY: test-hsm
test-hsm: test-hsm-unit test-hsm-integration
```

**Время:** 10 минут

### Верификация 1.3.5

```bash
# Run all HSM tests
make test-hsm

# Both unit and integration tests should pass
```

**Definition of Done:**
- [ ] Real HSM integration test создан
- [ ] 5+ test cases (basic, 2FA, API key, large data, wrong context)
- [ ] Retry test добавлен
- [ ] Makefile обновлен (test-hsm targets)
- [ ] All tests проходят

---

## Troubleshooting

### Проблема: "connection refused" к hsm-service

**Причина:** hsm-service не запущен или неверный URL.

**Решение:**
```bash
# Check if running
curl -k https://localhost:8200/health

# Start hsm-service
cd ../other-sub-system/hsm-service
./hsm-service -config config.yaml
```

### Проблема: "x509: certificate signed by unknown authority"

**Причина:** Client certificate не подписан той же CA, что и server certificate.

**Решение:**
1. Verify CA cert используется одна и та же:
   ```bash
   openssl x509 -in conf/ssl/ca-cert.pem -text -noout | grep Subject
   ```
2. Re-issue client certificate если нужно

### Проблема: "HSM decrypt error: invalid key_id"

**Причина:** Пытаетесь расшифровать с неверным key_id.

**Решение:**
- Используйте key_id, который вернулся из Encrypt()
- Убедитесь что key_id не изменился (не был обрезан, etc.)

### Проблема: "context mismatch"

**Причина:** Context в Decrypt() отличается от Encrypt().

**Решение:**
- Храните context вместе с ciphertext и key_id
- Пример: зашифровали с context="2fa", расшифровывайте тоже с "2fa"

### Проблема: Tests timeout

**Причина:** hsm-service медленно отвечает или недоступен.

**Решение:**
1. Увеличить timeout в test:
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
   ```
2. Check hsm-service logs для ошибок

---

## FAQ

**Q: Почему plaintext кодируется в base64?**
A: HTTP JSON не может передавать binary data напрямую. Base64 - стандартный способ для encoding binary в text format.

**Q: Безопасно ли передавать plaintext по HTTPS?**
A: Да, mTLS обеспечивает encryption + authentication. Plaintext шифруется на transport layer (TLS), затем снова на HSM layer (AES-256-GCM).

**Q: Как работает retry logic?**
A: Exponential backoff: 1-я попытка сразу, 2-я через 200ms, 3-я через 400ms, 4-я через 800ms, 5-я через 1.6s. Максимум 5 попыток, максимальная задержка 10s.

**Q: Что если HSM недоступен?**
A: После 5 failed retries возвращается ошибка. Application должен обработать это (fallback, alert, etc.). Health check покажет "hsm: error".

**Q: Можно ли использовать разные KEK для разных contexts?**
A: Да, hsm-service автоматически использует разные KEK на основе context. "2fa" использует kek-2fa-v1, "api_key" использует kek-api_key-v1.

**Q: Как тестировать без real HSM?**
A: Используйте MockHSMServer (httptest.Server). Для unit tests достаточно mock, для integration нужен real hsm-service.

**Q: Производительность - сколько операций в секунду?**
A: hsm-service обрабатывает ~1000-2000 encrypt/decrypt ops/sec (зависит от hardware). Используйте connection pooling и параллельные requests для high throughput.

---

## Summary Phase 1.3

**Созданные файлы:**
- `internal/hsm/types.go` (~50 строк)
- `internal/hsm/client.go` (~300 строк)
- `internal/hsm/client_test.go` (~100 строк)
- `internal/hsm/mock_server_test.go` (~50 строк)
- `tests/integration/hsm_integration_test.go` (~200 строк)

**Total LOC:** ~700 строк

**Обновленные файлы:**
- `internal/config/types.go` (добавлен HSMConfig)
- `conf/config.yaml` (добавлена hsm секция)
- `cmd/daemon/main.go` (добавлен HSM client + test)
- `internal/api/rest/health.go` (добавлен HSM health check)
- `Makefile` (test-hsm targets)

**Deliverables:**
✅ mTLS HTTP client для hsm-service  
✅ Encrypt/Decrypt methods  
✅ Retry logic (5 attempts, exponential backoff)  
✅ Helper methods (Encrypt2FA, EncryptAPIKey)  
✅ Mock HSM server для unit tests  
✅ Integration tests с real hsm-service  
✅ Health check с HSM status  

**Next Phase:** Phase 1.4 - State Management

---

## Definition of Done - Phase 1.3

- [ ] Все файлы созданы и скомпилированы
- [ ] `make build` проходит без ошибок
- [ ] `make test-hsm-unit` проходит (unit tests)
- [ ] `make test-hsm-integration` проходит (integration tests)
- [ ] HSM client подключается к hsm-service с mTLS
- [ ] Encrypt/Decrypt работают с real HSM
- [ ] Health endpoint возвращает hsm status
- [ ] Закоммичено в git:
  ```bash
  git add internal/hsm/ internal/config/ conf/config.yaml cmd/daemon/main.go internal/api/rest/health.go tests/integration/ Makefile
  git commit -m "Phase 1.3: HSM client with mTLS and retry logic"
  ```
- [ ] `guides/phase_1_3_hsm_client.md` удален (после завершения фазы)
