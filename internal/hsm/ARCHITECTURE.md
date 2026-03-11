# HSM Client Architecture in CTS-Core

## Overview

CTS-Core uses **two HSM clients** with different certificates to access two separate HSM contexts:

1. **Trading Context** (`exchange-key`) - for exchange API keys encryption
2. **2FA Context** (`2fa`) - for user 2FA secrets encryption (re-encryption job only)

This dual-client architecture is required for **HSM key rotation and re-encryption** functionality.

---

## Why Two HSM Clients?

### HSM ACL (Access Control Lists)

HSM service uses **OU-based ACL** where each certificate can only access its designated context:

| Certificate | OU Field | Allowed Context | Purpose |
|-------------|----------|-----------------|---------|
| `hsm-trading-client-1.crt` | `OU=Trading` | `exchange-key` | Exchange API keys encryption |
| `hsm-2fa-client-1.crt` | `OU=2FA` | `2fa` | User 2FA secrets encryption |

**Cross-context access is denied with HTTP 403:**
- Trading cert **CANNOT** access `2fa` context
- 2FA cert **CANNOT** access `exchange-key` context

This isolation is verified in integration tests: [integration_test.go](integration_test.go)

---

## Architecture Decision: CTS-Core Does NOT Decrypt User Credentials

### Original Design (from ARCHITECTURE.md)

> **CTS-Core НЕ имеет доступа к HSM** — Только передаёт зашифрованные данные

This means:
- CTS-Core **does not decrypt** exchange API keys in normal operation
- CTS-Core passes encrypted DEK + credentials to traders
- **Traders decrypt** credentials using HSM directly (OU=Trading)
- Web UI (www-go) handles 2FA encryption/decryption (OU=2FA)

### Exception: Re-encryption Job (Key Rotation)

CTS-Core **needs HSM access for re-encryption only:**

When HSM KEK key is rotated (v1 → v2), CTS-Core must:
1. **Decrypt** data with old key (v1) via HSM
2. **Encrypt** data with new key (v2) via HSM
3. **Update** database records with new ciphertext + key version

This requires access to **both contexts:**

| Table | Context | Certificate Needed |
|-------|---------|-------------------|
| `EXCHANGE_ACCOUNTS` | `exchange-key` | hsm-trading-client-1.crt |
| `USER_2FA` | `2fa` | hsm-2fa-client-1.crt |

See: [ARCHITECTURE.md#6.7 HSM Key Rotation](../../ARCHITECTURE.md#67-hsm-key-rotation--re-encryption-phase-1)

---

## Configuration

### config.yaml Structure

```yaml
hsm:
  url: "https://192.168.50.4:8443"
  timeout: 10s
  retry:
    max_attempts: 5
    initial_delay: 200ms
    max_delay: 10s
    multiplier: 2.0
  
  # Trading context (exchange API keys encryption)
  trading:
    context: "exchange-key"
    tls:
      enabled: true
      ca_path: "pki/ca/ca.crt"
      cert_path: "pki/client/hsm-trading-client-1.crt"
      key_path: "pki/client/hsm-trading-client-1.key"
  
  # 2FA context (user 2FA secrets - for re-encryption job only)
  two_fa:
    context: "2fa"
    tls:
      enabled: true
      ca_path: "pki/ca/ca.crt"
      cert_path: "pki/client/hsm-2fa-client-1.crt"
      key_path: "pki/client/hsm-2fa-client-1.key"
```

### Code Initialization (main.go)

```go
// 1. Trading context client
hsmTradingClient, err := hsm.NewClient(hsm.ClientConfig{
    BaseURL:  cfg.HSM.URL,
  CertPath: cfg.HSM.Trading.TLS.CertPath,
  KeyPath:  cfg.HSM.Trading.TLS.KeyPath,
  CAPath:   cfg.HSM.Trading.TLS.CAPath,
    // ...
}, hsmLogger)

// 2. 2FA context client
hsm2FAClient, err := hsm.NewClient(hsm.ClientConfig{
    BaseURL:  cfg.HSM.URL,
  CertPath: cfg.HSM.TwoFA.TLS.CertPath,
  KeyPath:  cfg.HSM.TwoFA.TLS.KeyPath,
  CAPath:   cfg.HSM.TwoFA.TLS.CAPath,
    // ...
}, hsmLogger)
```

---

## Usage Scenarios

### 1. Normal Operation (Phase 1.5+)

**CTS-Core does NOT use HSM in normal operation:**

```
1. Web Admin creates EXCHANGE_ACCOUNTS record via www-go
2. www-go encrypts DEK + API keys via HSM (OU=Trading)
3. CTS-Core reads encrypted data from MySQL
4. CTS-Core sends encrypted DEK + credentials to trader via WebSocket
5. Trader decrypts via HSM directly (OU=Trading)
6. Trader uses API keys to trade on exchange
```

**CTS-Core never decrypts exchange credentials** - only passes encrypted data.

### 2. Re-encryption Job (Key Rotation)

**CTS-Core uses BOTH HSM clients for re-encryption:**

```
TRIGGER: Admin rotates KEK key in HSM service (v1 → v2)

CTS-Core Scheduler detects:
1. SELECT records WHERE enc_key_version = 1 FROM EXCHANGE_ACCOUNTS
2. FOR EACH record:
   a) Decrypt DEK with v1 via hsmTradingClient
   b) Encrypt DEK with v2 via hsmTradingClient
   c) UPDATE record SET dek_enc=new, enc_key_version=2
3. SAME PROCESS for USER_2FA table via hsm2FAClient

Result: All data re-encrypted with new KEK key
```

See implementation: [ARCHITECTURE.md lines 843-900](../../ARCHITECTURE.md#L843-L900)

---

## Testing

### Unit Tests

Run without HSM service:
```bash
make test
```

Tests:
- `TestCalculateBackoff` - exponential backoff logic
- `TestBase64Encoding` - base64 encoding/decoding
- `TestRetryConfig` - retry configuration structure

### Integration Tests

Requires HSM service at `https://192.168.50.4:8443`:
```bash
make hsm-test
```

Tests:
- **Trading context**: Encrypt/decrypt with `exchange-key` context
- **2FA context**: Encrypt/decrypt with `2fa` context
- **ACL isolation**: Verify 403 Forbidden for cross-context access

See: [README_TESTS.md](README_TESTS.md) for detailed testing guide.

---

## Security Model

### Key Hierarchy

```
KEK (Key Encryption Key) - stored in HSM
  ├─ kek-exchange-key-v1 (Trading context)
  │   └─ encrypts DEK (Data Encryption Key)
  │       └─ encrypts API keys + secrets
  │
  └─ kek-2fa-v1 (2FA context)
      └─ encrypts DEK
          └─ encrypts TOTP secrets + recovery codes
```

### Access Control

| Component | OU Certificate | Can Access | Cannot Access |
|-----------|---------------|------------|---------------|
| **CTS-Core** (re-encryption) | Trading + 2FA | Both contexts | - |
| **Traders** (runtime) | Trading | exchange-key | 2fa |
| **Web UI** (www-go) | 2FA | 2fa | exchange-key |

### HSM Service ACL Enforcement

- Each client certificate has `OU` field in subject
- HSM service checks `OU` field on every request
- Context access is mapped to `OU`:
  - `OU=Trading` → `exchange-key` context
  - `OU=2FA` → `2fa` context
- Cross-context requests return **HTTP 403 Forbidden**

---

## Future Enhancements (Phase 2+)

1. **Circuit Breaker**: Disable HSM calls if service unavailable
2. **Metrics**: Track encrypt/decrypt latency, success rate
3. **Caching**: Cache KEK key IDs to reduce HSM roundtrips
4. **Failover**: Support multiple HSM service instances
5. **Audit**: Log all HSM operations to audit log

---

## References

- [ARCHITECTURE.md](../../ARCHITECTURE.md) - Full architecture (section 6.7: Key Rotation)
- [integration_test.go](integration_test.go) - ACL isolation tests
- [README_TESTS.md](README_TESTS.md) - Testing guide
- `services/hsm-service` - HSM service implementation
