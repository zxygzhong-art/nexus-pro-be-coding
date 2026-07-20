package secret_test

import (
	"encoding/base64"
	"testing"

	platformsecret "nexus-pro-api/internal/platform/secret"
)

// TestAESGCMCipherRoundTripAndContextBinding verifies encryption randomness and associated-data isolation.
func TestAESGCMCipherRoundTripAndContextBinding(t *testing.T) {
	encodedKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := platformsecret.NewAESGCMCipher(encodedKey)
	if err != nil {
		t.Fatal(err)
	}
	first, err := cipher.Encrypt([]byte("token-value"), []byte("tenant-1\x00tool-1"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := cipher.Encrypt([]byte("token-value"), []byte("tenant-1\x00tool-1"))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected random nonces to produce distinct ciphertexts")
	}
	plaintext, err := cipher.Decrypt(first, []byte("tenant-1\x00tool-1"))
	if err != nil || string(plaintext) != "token-value" {
		t.Fatalf("unexpected round trip: plaintext=%q err=%v", plaintext, err)
	}
	if _, err := cipher.Decrypt(first, []byte("tenant-2\x00tool-1")); err == nil {
		t.Fatal("expected associated-data mismatch to fail")
	}
}

// TestAESGCMCipherRejectsInvalidKey verifies startup key validation is fail closed.
func TestAESGCMCipherRejectsInvalidKey(t *testing.T) {
	if _, err := platformsecret.NewAESGCMCipher("not-base64"); err == nil {
		t.Fatal("expected malformed base64 key to fail")
	}
	shortKey := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := platformsecret.NewAESGCMCipher(shortKey); err == nil {
		t.Fatal("expected non-32-byte key to fail")
	}
}
