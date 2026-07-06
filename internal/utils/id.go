package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var idCounter uint64

// NewID 建立 ID。
func NewID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "id"
	}
	seq := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d-%06d", prefix, time.Now().UTC().UnixNano(), seq)
}

// NewSecretID 建立 secret ID。
func NewSecretID(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "id"
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw), nil
}
