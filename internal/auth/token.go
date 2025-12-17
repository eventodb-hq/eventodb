// Package auth provides authentication and token management for Message DB.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Token format: ns_<base64(namespace)>_<random_32_bytes_hex>
// Example: ns_dGVuYW50LWE_a7f3c8d9e2b1f4a6c8e9d2b3f5a7c9e1

// GenerateToken creates a new authentication token for a namespace.
//
// The token format is: ns_<base64url(namespace)>_<random_hex>
// where random_hex is 32 random bytes encoded as hexadecimal (64 chars).
//
// The token embeds the namespace ID for quick extraction without database lookup.
func GenerateToken(namespace string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("namespace cannot be empty")
	}

	// Generate 32 random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode namespace as base64url (URL-safe, no padding)
	nsEncoded := base64.RawURLEncoding.EncodeToString([]byte(namespace))

	// Encode random bytes as hexadecimal
	randomHex := hex.EncodeToString(randomBytes)

	// Combine into token format
	token := fmt.Sprintf("ns_%s_%s", nsEncoded, randomHex)

	return token, nil
}

// ParseToken extracts the namespace from a token.
//
// Returns an error if the token format is invalid.
func ParseToken(token string) (namespace string, err error) {
	// Check prefix
	if !strings.HasPrefix(token, "ns_") {
		return "", fmt.Errorf("invalid token format: missing 'ns_' prefix")
	}

	// Remove prefix
	token = strings.TrimPrefix(token, "ns_")

	// Split by underscore
	parts := strings.Split(token, "_")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid token format: expected 2 parts after prefix, got %d", len(parts))
	}

	nsEncoded := parts[0]
	randomHex := parts[1]

	// Validate random part length (32 bytes = 64 hex chars)
	if len(randomHex) != 64 {
		return "", fmt.Errorf("invalid token format: random part must be 64 hex characters, got %d", len(randomHex))
	}

	// Validate random part is valid hex
	if _, err := hex.DecodeString(randomHex); err != nil {
		return "", fmt.Errorf("invalid token format: random part is not valid hexadecimal: %w", err)
	}

	// Decode namespace from base64url
	nsBytes, err := base64.RawURLEncoding.DecodeString(nsEncoded)
	if err != nil {
		return "", fmt.Errorf("invalid token format: namespace part is not valid base64url: %w", err)
	}

	namespace = string(nsBytes)
	if namespace == "" {
		return "", fmt.Errorf("invalid token format: namespace is empty")
	}

	return namespace, nil
}

// HashToken creates a SHA-256 hash of a token for storage.
//
// This is used to store tokens securely in the database without revealing
// the actual token value.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
