package hsm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/titaev-lv/cts-core/internal/requestid"
)

// Client represents HSM service HTTP client with mTLS
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	outLogger  *slog.Logger
	retryCfg   RetryConfig
}

const hsmRequestIDHeader = "X-Request-ID"

// RetryConfig contains retry logic configuration
type RetryConfig struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// ClientConfig contains HSM client configuration
type ClientConfig struct {
	BaseURL          string
	CertPath         string
	KeyPath          string
	CAPath           string
	RequestTimeout   time.Duration
	RetryConfig      RetryConfig
	OutRequestLogger *slog.Logger
}

// NewClient creates new HSM client with mTLS
func NewClient(cfg ClientConfig, logger *slog.Logger) (*Client, error) {
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

	logger.Info("HSM client initialized", "url", cfg.BaseURL, "mtls", true)

	return &Client{
		baseURL:    cfg.BaseURL,
		httpClient: httpClient,
		logger:     logger,
		outLogger:  cfg.OutRequestLogger,
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
	ctx, requestID := ensureRequestID(ctx)
	var lastErr error

	for attempt := 1; attempt <= c.retryCfg.MaxAttempts; attempt++ {
		attemptStart := time.Now()
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
		req.Header.Set(hsmRequestIDHeader, requestID)

		// Execute request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			c.logOutRequest(ctx, method, path, url, 0, time.Since(attemptStart), -1, lastErr)
			c.logRetry(attempt, lastErr)

			if attempt < c.retryCfg.MaxAttempts {
				c.waitBeforeRetry(ctx, attempt)
				continue
			}
			break
		}

		// Read response body
		defer resp.Body.Close()
		serverLatencyUS := extractServerLatencyUS(resp)
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			c.logOutRequest(ctx, method, path, url, resp.StatusCode, time.Since(attemptStart), serverLatencyUS, lastErr)
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
			c.logOutRequest(ctx, method, path, url, resp.StatusCode, time.Since(attemptStart), serverLatencyUS, lastErr)
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
		c.logOutRequest(ctx, method, path, url, resp.StatusCode, time.Since(attemptStart), serverLatencyUS, nil)
		return nil
	}

	return fmt.Errorf("request failed after %d attempts: %w", c.retryCfg.MaxAttempts, lastErr)
}

func ensureRequestID(ctx context.Context) (context.Context, string) {
	requestID := requestid.FromContext(ctx)
	if requestID == "" {
		requestID = generateRequestID()
	}
	return requestid.WithContext(ctx, requestID), requestID
}

func generateRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("hsm-%d", time.Now().UTC().UnixNano())
}

func extractServerLatencyUS(resp *http.Response) int64 {
	if resp == nil {
		return -1
	}

	val := resp.Header.Get("X-HSM-Processing-Us")
	if val == "" {
		return -1
	}

	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil || parsed < 0 {
		return -1
	}

	return parsed
}

func (c *Client) logOutRequest(ctx context.Context, method, path, url string, status int, latency time.Duration, serverLatencyUS int64, err error) {
	if c.outLogger == nil {
		return
	}

	requestID := requestid.FromContext(ctx)
	includeDetailedLatency := c.outLogger.Enabled(ctx, slog.LevelDebug)
	totalLatencyMS := float64(latency.Microseconds()) / 1000.0

	serverLatencyMS := -1.0
	networkLatencyMS := -1.0
	if serverLatencyUS >= 0 {
		serverLatencyMS = float64(serverLatencyUS) / 1000.0
		networkLatencyMS = totalLatencyMS - serverLatencyMS
		if networkLatencyMS < 0 {
			networkLatencyMS = 0
		}
	}

	latencyField := any(totalLatencyMS)
	if includeDetailedLatency {
		latencyBreakdown := map[string]float64{
			"total": totalLatencyMS,
		}
		if serverLatencyMS >= 0 {
			latencyBreakdown["server"] = serverLatencyMS
			latencyBreakdown["network"] = networkLatencyMS
		}
		latencyField = latencyBreakdown
	}

	fields := []any{
		"method", method,
		"path", path,
		"url", url,
		"status", status,
		"latency_ms", latencyField,
		"request_id", requestID,
	}

	if err != nil {
		fields = append(fields, "error", err)
		c.outLogger.Warn("HSM request", fields...)
		return
	}

	c.outLogger.Info("HSM request", fields...)
}

// logRetry logs retry attempt
func (c *Client) logRetry(attempt int, err error) {
	if attempt < c.retryCfg.MaxAttempts {
		wait := c.calculateBackoff(attempt)
		c.logger.Warn("HSM request failed, retrying",
			"error", err,
			"attempt", attempt,
			"retry_in", wait,
		)
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

// Encrypt encrypts plaintext using HSM service
// context: "exchange-key" for trading credentials
// plaintext: raw bytes to encrypt
// Returns: keyID and ciphertext (both base64-encoded)
func (c *Client) Encrypt(ctx context.Context, context string, plaintext []byte) (keyID string, ciphertext string, err error) {
	req := EncryptRequest{
		Context:   context,
		Plaintext: EncodeBase64(plaintext),
	}

	var resp EncryptResponse
	if err := c.doRequest(ctx, "POST", "/encrypt", req, &resp); err != nil {
		return "", "", fmt.Errorf("encrypt request failed: %w", err)
	}

	if resp.Error != "" {
		return "", "", fmt.Errorf("HSM encrypt error: %s", resp.Error)
	}

	return resp.KeyID, resp.Ciphertext, nil
}

// Decrypt decrypts ciphertext using HSM service
// context: "exchange-key" for trading credentials
// keyID: key version identifier (e.g., "kek-exchange-key-v1")
// ciphertext: base64-encoded encrypted data
// Returns: decrypted plaintext bytes
func (c *Client) Decrypt(ctx context.Context, context string, keyID string, ciphertext string) ([]byte, error) {
	req := DecryptRequest{
		Context:    context,
		KeyID:      keyID,
		Ciphertext: ciphertext,
	}

	var resp DecryptResponse
	if err := c.doRequest(ctx, "POST", "/decrypt", req, &resp); err != nil {
		return nil, fmt.Errorf("decrypt request failed: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("HSM decrypt error: %s", resp.Error)
	}

	// Decode base64 plaintext
	plaintext, err := DecodeBase64(resp.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode plaintext: %w", err)
	}

	return plaintext, nil
}

// Health fetches HSM service health status.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.doRequest(ctx, "GET", "/health", nil, &resp); err != nil {
		return nil, fmt.Errorf("health request failed: %w", err)
	}

	return &resp, nil
}
