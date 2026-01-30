package hsm

import "encoding/base64"

// EncryptRequest represents /encrypt API request to HSM service
type EncryptRequest struct {
Context   string `json:"context"`   // e.g., "exchange-key" for trading credentials
Plaintext string `json:"plaintext"` // base64-encoded data
}

// EncryptResponse represents /encrypt API response from HSM service
type EncryptResponse struct {
KeyID      string `json:"key_id"`      // e.g., "kek-exchange-key-v1"
Ciphertext string `json:"ciphertext"`  // base64-encoded encrypted data
Error      string `json:"error,omitempty"`
}

// DecryptRequest represents /decrypt API request to HSM service
type DecryptRequest struct {
Context    string `json:"context"`    // e.g., "exchange-key"
KeyID      string `json:"key_id"`     // e.g., "kek-exchange-key-v1"
Ciphertext string `json:"ciphertext"` // base64-encoded encrypted data
}

// DecryptResponse represents /decrypt API response from HSM service
type DecryptResponse struct {
Plaintext string `json:"plaintext"` // base64-encoded decrypted data
Error     string `json:"error,omitempty"`
}

// EncodeBase64 encodes byte slice to base64 string
func EncodeBase64(data []byte) string {
return base64.StdEncoding.EncodeToString(data)
}

// DecodeBase64 decodes base64 string to byte slice
func DecodeBase64(encoded string) ([]byte, error) {
return base64.StdEncoding.DecodeString(encoded)
}
