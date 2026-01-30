package hsm

import (
"log/slog"
"testing"
"time"
)

func TestCalculateBackoff(t *testing.T) {
logger := slog.Default()

client := &Client{
logger: logger,
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
{1, 100 * time.Millisecond},
{2, 200 * time.Millisecond},
{3, 400 * time.Millisecond},
{4, 800 * time.Millisecond},
{5, 1600 * time.Millisecond},
{10, 5 * time.Second},
}

for _, tt := range tests {
result := client.calculateBackoff(tt.attempt)
if result != tt.expected {
t.Errorf("Attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
}
}
}

func TestBase64Encoding(t *testing.T) {
tests := []struct {
name  string
input []byte
want  string
}{
{"empty", []byte{}, ""},
{"simple", []byte("hello"), "aGVsbG8="},
{"binary", []byte{0x01, 0x02, 0x03}, "AQID"},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
encoded := EncodeBase64(tt.input)
if encoded != tt.want {
t.Errorf("EncodeBase64() = %v, want %v", encoded, tt.want)
}

decoded, err := DecodeBase64(encoded)
if err != nil {
t.Errorf("DecodeBase64() error = %v", err)
}
if string(decoded) != string(tt.input) {
t.Errorf("Round-trip failed: got %v, want %v", decoded, tt.input)
}
})
}
}

func TestRetryConfig(t *testing.T) {
cfg := RetryConfig{
MaxAttempts: 3,
InitialWait: 100 * time.Millisecond,
MaxWait:     1 * time.Second,
Multiplier:  2.0,
}

if cfg.MaxAttempts != 3 {
t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
}
if cfg.InitialWait != 100*time.Millisecond {
t.Errorf("InitialWait = %v, want 100ms", cfg.InitialWait)
}
}
