package id

import (
	"crypto/rand"
	"encoding/hex"
)

func New(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
