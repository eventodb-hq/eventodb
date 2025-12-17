package store

import (
	"crypto/md5"
	"encoding/binary"
	"strings"
)

// Category extracts the category name from a stream name
// Examples:
//
//	Category("account-123") → "account"
//	Category("account-123+456") → "account"
//	Category("account") → "account"
func Category(streamName string) string {
	parts := strings.SplitN(streamName, "-", 2)
	return parts[0]
}

// ID extracts the ID portion from a stream name
// Examples:
//
//	ID("account-123") → "123"
//	ID("account-123+456") → "123+456"
//	ID("account") → ""
func ID(streamName string) string {
	parts := strings.SplitN(streamName, "-", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// CardinalID extracts the cardinal ID (before '+') from a stream name
// Used for consumer group partitioning with compound IDs
// Examples:
//
//	CardinalID("account-123") → "123"
//	CardinalID("account-123+456") → "123"
//	CardinalID("account") → ""
func CardinalID(streamName string) string {
	id := ID(streamName)
	if id == "" {
		return ""
	}
	// Extract part before '+' for compound IDs
	parts := strings.SplitN(id, "+", 2)
	return parts[0]
}

// IsCategory determines if a name represents a category (no ID part)
// Examples:
//
//	IsCategory("account") → true
//	IsCategory("account-123") → false
func IsCategory(name string) bool {
	return !strings.Contains(name, "-")
}

// Hash64 computes a 64-bit hash compatible with Message DB
// Uses MD5, takes first 8 bytes, converts to int64
// CRITICAL: Must produce identical results to Message DB for consumer group compatibility
func Hash64(value string) int64 {
	hash := md5.Sum([]byte(value))
	// Take first 8 bytes of MD5 hash and convert to int64
	// Use big-endian to match Postgres bit(64) conversion
	return int64(binary.BigEndian.Uint64(hash[:8]))
}

// IsAssignedToConsumerMember determines which consumer group member should handle a stream
// Returns true if the given stream should be handled by the specified consumer member
func IsAssignedToConsumerMember(streamName string, member, size int64) bool {
	if size <= 0 || member < 0 || member >= size {
		return false
	}

	cardinalID := CardinalID(streamName)
	if cardinalID == "" {
		return false
	}

	hash := Hash64(cardinalID)
	// Use absolute value to handle negative hashes
	if hash < 0 {
		hash = -hash
	}

	return (hash % size) == member
}
