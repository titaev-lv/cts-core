# HSM Integration Tests

Integration tests для проверки реального взаимодействия с HSM service.

## Предварительные требования

1. HSM service должен быть запущен и доступен (по умолчанию: `https://192.168.50.4:8443`)
2. Сертификаты должны быть настроены в `pki/client/`:
   - `hsm-trading-client-1.{crt,key}` - для контекста `exchange-key` (OU=Trading)
   - `hsm-2fa-client-1.{crt,key}` - для контекста `2fa` (OU=2FA)
3. HSM service должен иметь KEK ключи:
   - `kek-exchange-key-v1` - для Trading
   - `kek-2fa-v1` - для 2FA

## Запуск тестов

### Проверка доступности HSM

```bash
# Проверка health endpoint
curl -k --cert pki/client/hsm-trading-client-1.crt \
        --key pki/client/hsm-trading-client-1.key \
        https://192.168.50.4:8443/health
```

### Unit тесты (без HSM)

```bash
go test -v ./internal/hsm/...
```

### Integration тесты (требуют HSM)

```bash
# Все integration тесты
HSM_URL=https://192.168.50.4:8443 go test -v -tags=integration ./internal/hsm/...

# Только Trading контекст
HSM_URL=https://192.168.50.4:8443 go test -v -tags=integration -run TestHSMIntegration_TradingContext ./internal/hsm/...

# Только 2FA контекст
HSM_URL=https://192.168.50.4:8443 go test -v -tags=integration -run TestHSMIntegration_2FAContext ./internal/hsm/...

# Через Makefile
make hsm-test
```

## Что тестируется

### TestHSMIntegration_TradingContext
- ✅ Encrypt с контекстом `exchange-key` (должен работать)
- ✅ Decrypt round-trip
- ✅ Попытка доступа к контексту `2fa` (должна вернуть 403 Forbidden)

### TestHSMIntegration_2FAContext
- ✅ Encrypt с контекстом `2fa` (должен работать)
- ✅ Decrypt round-trip
- ✅ Попытка доступа к контексту `exchange-key` (должна вернуть 403 Forbidden)

### TestHSMIntegration_ACLIsolation
- ✅ Проверка изоляции между контекстами

## Архитектура HSM ACL

HSM service использует OU (Organizational Unit) из клиентского сертификата для контроля доступа:

```yaml
acl:
  mappings:
    Trading: [exchange-key]  # OU=Trading → контекст exchange-key
    2FA: [2fa]               # OU=2FA → контекст 2fa
```

**CTS-Core использует:**
- Сертификат: `hsm-trading-client-1.crt` (OU=Trading)
- Контекст: `exchange-key`
- Цель: Шифрование торговых креденшелов (exchange API keys, secrets)

## Режимы AAD (Additional Authenticated Data)

### exchange-key (mode=shared)
- AAD = context + OU
- Все Trading сервисы могут расшифровывать данные друг друга
- Подходит для общих секретов (exchange credentials)

### 2fa (mode=private)
- AAD = context + clientCN
- Каждый клиент видит только свои данные
- Подходит для пользовательских 2FA секретов

## Примеры использования

### В production коде

```go
// Encrypt exchange API key
plaintext := []byte("binance_api_key=abc123&secret=xyz789")
keyID, ciphertext, err := hsmClient.Encrypt(ctx, "exchange-key", plaintext)
if err != nil {
    log.Error("Failed to encrypt", "error", err)
    return err
}

// Store in database
trader.EncKeyID = keyID
trader.EncCredentials = ciphertext

// Later: decrypt
decrypted, err := hsmClient.Decrypt(ctx, "exchange-key", keyID, ciphertext)
if err != nil {
    log.Error("Failed to decrypt", "error", err)
    return err
}
```

## Troubleshooting

### HSM недоступен
```
Error: HTTP request failed: dial tcp 192.168.50.4:8443: connect: connection refused
```
**Решение:** Проверьте что HSM service запущен

### 403 Forbidden
```
Error: HTTP 403: {"error":"access denied: insufficient permissions"}
```
**Решение:** Проверьте что используется правильный сертификат для контекста

### Certificate not found
```
Error: failed to load client cert: open pki/client/hsm-trading-client-1.crt: no such file or directory
```
**Решение:** Запускайте тесты из корня проекта или используйте правильные пути

### Invalid certificate
```
Error: tls: bad certificate
```
**Решение:** Проверьте что сертификат подписан правильным CA (pki/ca/ca.crt)
