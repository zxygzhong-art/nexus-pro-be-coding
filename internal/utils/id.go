package utils

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var idCounter uint64

// NewID returns a stable, sortable-enough local identifier with the given prefix.
func NewID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "id"
	}
	seq := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d-%06d", prefix, time.Now().UTC().UnixNano(), seq)
}
