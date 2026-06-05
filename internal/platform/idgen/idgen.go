// Package idgen produces prefixed, sortable-ish unique ids (e.g. ps-..., ars_...).
package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

// random returns a 16-char lowercase base32 token.
func random() string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return strings.ToLower(enc.EncodeToString(b[:]))
}

// New returns "<prefix>-<token>".
func New(prefix string) string {
	return prefix + "-" + random()
}

// NewWith returns "<prefix><sep><token>" (e.g. NewWith("ars", "_") -> ars_...).
func NewWith(prefix, sep string) string {
	return prefix + sep + random()
}
