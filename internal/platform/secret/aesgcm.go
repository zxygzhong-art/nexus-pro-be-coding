package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const ciphertextVersion = "v1"

// AESGCMCipher encrypts tenant-scoped credentials using a versioned AES-256-GCM envelope.
type AESGCMCipher struct {
	aead cipher.AEAD
}

// NewAESGCMCipher builds a cipher from a standard-base64 encoded 32-byte key.
func NewAESGCMCipher(encodedKey string) (*AESGCMCipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return nil, fmt.Errorf("AGENT_TOOL_CREDENTIAL_ENCRYPTION_KEY must be standard base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("AGENT_TOOL_CREDENTIAL_ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize agent tool credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize agent tool credential envelope: %w", err)
	}
	return &AESGCMCipher{aead: aead}, nil
}

// Encrypt seals plaintext with a random nonce and caller-provided associated data.
func (c *AESGCMCipher) Encrypt(plaintext, associatedData []byte) (string, error) {
	if c == nil || c.aead == nil {
		return "", errors.New("agent tool credential cipher is not initialized")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate agent tool credential nonce: %w", err)
	}
	payload := c.aead.Seal(nonce, nonce, plaintext, associatedData)
	return ciphertextVersion + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

// Decrypt opens one versioned credential envelope using the same associated data.
func (c *AESGCMCipher) Decrypt(ciphertext string, associatedData []byte) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("agent tool credential cipher is not initialized")
	}
	version, encoded, ok := strings.Cut(strings.TrimSpace(ciphertext), ":")
	if !ok || version != ciphertextVersion {
		return nil, errors.New("unsupported agent tool credential ciphertext version")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid agent tool credential ciphertext")
	}
	if len(payload) < c.aead.NonceSize() {
		return nil, errors.New("invalid agent tool credential ciphertext")
	}
	nonce, sealed := payload[:c.aead.NonceSize()], payload[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, sealed, associatedData)
	if err != nil {
		return nil, errors.New("invalid agent tool credential ciphertext")
	}
	return plaintext, nil
}
