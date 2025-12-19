package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"math/big"

	"github.com/cespare/xxhash/v2"
)

// OneWayKeyedHash computes an HMAC-SHA256 of the provided data using the given key.
// This is a one-way keyed hash (non-reversible) suitable for pseudonymizing emails
// before sending them to upstream systems or using them as cache keys.
// Result is truncated to 16 bytes (32 hex characters) for compactness.
func OneWayKeyedHash(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

// OneWayKeyedHashNumeric computes an HMAC-SHA256 and returns a numeric string.
// It converts the first 16 bytes of the hash into a base-10 integer string.
func OneWayKeyedHashNumeric(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	sum := h.Sum(nil)
	// Treat first 16 bytes as big integer
	i := new(big.Int).SetBytes(sum[:16])
	return i.String()
}

// CompressUserAgent creates a short, deterministic hash of the User-Agent string.
// It uses xxHash-64 and Base64 encoding to produce a compact identifier.
func CompressUserAgent(ua string) string {
	sum := xxhash.Sum64([]byte(ua))
	var buf [8]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(sum >> (56 - 8*i))
	}
	encoded := make([]byte, base64.URLEncoding.EncodedLen(len(buf)))
	base64.URLEncoding.Encode(encoded, buf[:])
	// Return the first 11 characters which is sufficient entropy for this use case
	return string(encoded[:11])
}
