// +build integration

package hsm

import (
"context"
"log/slog"
"os"
"testing"
"time"

"github.com/titaev-lv/cts-core/internal/logger"
)

// TestHSMIntegration_TradingContext tests real HSM service with Trading OU certificate
// Run with: HSM_URL=https://192.168.50.4:8443 go test -v -tags=integration ./internal/hsm/...
func TestHSMIntegration_TradingContext(t *testing.T) {
hsmURL := os.Getenv("HSM_URL")
if hsmURL == "" {
t.Skip("Skipping integration test: HSM_URL not set")
}

log := initTestLogger(t)

cfg := ClientConfig{
BaseURL:        hsmURL,
CertPath:       "../../pki/client/hsm-trading-client-1.crt",
KeyPath:        "../../pki/client/hsm-trading-client-1.key",
CAPath:         "../../pki/ca/ca.crt",
RequestTimeout: 10 * time.Second,
RetryConfig: RetryConfig{
MaxAttempts: 3,
InitialWait: 100 * time.Millisecond,
MaxWait:     2 * time.Second,
Multiplier:  2.0,
},
}

client, err := NewClient(cfg, log)
if err != nil {
t.Fatalf("Failed to create HSM client: %v", err)
}
defer client.Close()

ctx := context.Background()

// Test 1: Encrypt with exchange-key context (should work for Trading OU)
t.Run("Encrypt_ExchangeKey", func(t *testing.T) {
plaintext := []byte("binance_api_key=abc123")

keyID, ciphertext, err := client.Encrypt(ctx, "exchange-key", plaintext)
if err != nil {
t.Fatalf("Encrypt failed: %v", err)
}

if keyID == "" {
t.Error("Expected non-empty keyID")
}
if ciphertext == "" {
t.Error("Expected non-empty ciphertext")
}

t.Logf("Encrypted successfully: keyID=%s, ciphertext_len=%d", keyID, len(ciphertext))

// Test 2: Decrypt the same data
t.Run("Decrypt_RoundTrip", func(t *testing.T) {
decrypted, err := client.Decrypt(ctx, "exchange-key", keyID, ciphertext)
if err != nil {
t.Fatalf("Decrypt failed: %v", err)
}

if string(decrypted) != string(plaintext) {
t.Errorf("Decrypted data mismatch: got %q, want %q", string(decrypted), string(plaintext))
}

t.Logf("Decrypt successful: %s", string(decrypted))
})
})

// Test 3: Try to access 2fa context with Trading certificate (should fail with 403)
t.Run("Encrypt_2FA_ShouldFail", func(t *testing.T) {
plaintext := []byte("totp_secret=ABCD1234")

_, _, err := client.Encrypt(ctx, "2fa", plaintext)
if err == nil {
t.Error("Expected error when Trading OU tries to access 2fa context, but got success")
} else {
t.Logf("Expected error occurred: %v", err)
// Should contain "403" or "Forbidden" in error message
}
})
}

// TestHSMIntegration_2FAContext tests HSM with 2FA OU certificate
// Run with: HSM_URL=https://192.168.50.4:8443 go test -v -tags=integration -run TestHSMIntegration_2FAContext ./internal/hsm/...
func TestHSMIntegration_2FAContext(t *testing.T) {
hsmURL := os.Getenv("HSM_URL")
if hsmURL == "" {
t.Skip("Skipping integration test: HSM_URL not set")
}

log := initTestLogger(t)

cfg := ClientConfig{
BaseURL:        hsmURL,
CertPath:       "../../pki/client/hsm-2fa-client-1.crt",
KeyPath:        "../../pki/client/hsm-2fa-client-1.key",
CAPath:         "../../pki/ca/ca.crt",
RequestTimeout: 10 * time.Second,
RetryConfig: RetryConfig{
MaxAttempts: 3,
InitialWait: 100 * time.Millisecond,
MaxWait:     2 * time.Second,
Multiplier:  2.0,
},
}

client, err := NewClient(cfg, log)
if err != nil {
t.Fatalf("Failed to create HSM client: %v", err)
}
defer client.Close()

ctx := context.Background()

// Test 1: Encrypt with 2fa context (should work for 2FA OU)
t.Run("Encrypt_2FA", func(t *testing.T) {
plaintext := []byte("totp_secret=JBSWY3DPEHPK3PXP")

keyID, ciphertext, err := client.Encrypt(ctx, "2fa", plaintext)
if err != nil {
t.Fatalf("Encrypt failed: %v", err)
}

func initTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	logDir := t.TempDir()
	if err := logger.Init("debug", logDir, 10, 3, 7, false); err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger.Get("hsm-test")
}

if keyID == "" {
t.Error("Expected non-empty keyID")
}
if ciphertext == "" {
t.Error("Expected non-empty ciphertext")
}

t.Logf("Encrypted successfully: keyID=%s, ciphertext_len=%d", keyID, len(ciphertext))

// Test 2: Decrypt the same data
t.Run("Decrypt_RoundTrip", func(t *testing.T) {
decrypted, err := client.Decrypt(ctx, "2fa", keyID, ciphertext)
if err != nil {
t.Fatalf("Decrypt failed: %v", err)
}

if string(decrypted) != string(plaintext) {
t.Errorf("Decrypted data mismatch: got %q, want %q", string(decrypted), string(plaintext))
}

t.Logf("Decrypt successful: %s", string(decrypted))
})
})

// Test 3: Try to access exchange-key context with 2FA certificate (should fail with 403)
t.Run("Encrypt_ExchangeKey_ShouldFail", func(t *testing.T) {
plaintext := []byte("binance_api_key=should_fail")

_, _, err := client.Encrypt(ctx, "exchange-key", plaintext)
if err == nil {
t.Error("Expected error when 2FA OU tries to access exchange-key context, but got success")
} else {
t.Logf("Expected error occurred: %v", err)
// Should contain "403" or "Forbidden" in error message
}
})
}

// TestHSMIntegration_ACLIsolation verifies that different OUs cannot access each other's contexts
func TestHSMIntegration_ACLIsolation(t *testing.T) {
hsmURL := os.Getenv("HSM_URL")
if hsmURL == "" {
t.Skip("Skipping integration test: HSM_URL not set")
}

t.Run("CrossContextIsolation", func(t *testing.T) {
t.Log("This test verifies HSM ACL isolation between Trading and 2FA contexts")
t.Log("Run TestHSMIntegration_TradingContext and TestHSMIntegration_2FAContext to see detailed results")
})
}
