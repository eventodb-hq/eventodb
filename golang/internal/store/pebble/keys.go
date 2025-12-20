// Package pebble provides a Pebble-based key-value store backend for EventoDB.
//
// Key Schema (per namespace DB):
//   - M:{gp_20}                    → {message_json}      Message data
//   - SI:{stream}:{pos_20}         → {gp_20}             Stream index
//   - CI:{category}:{gp_20}        → {stream}            Category index
//   - VI:{stream}                  → {pos_20}            Version index
//   - GP                           → {next_gp_20}        Global position counter
//
// Metadata DB Schema:
//   - NS:{namespace_id}            → {namespace_json}    Namespace registry
package pebble

import (
	"fmt"
	"strconv"
	"strings"
)

// Key prefixes for different index types
const (
	prefixMessage        = "M:"  // Message data
	prefixStreamIndex    = "SI:" // Stream index
	prefixCategoryIndex  = "CI:" // Category index
	prefixVersionIndex   = "VI:" // Version index
	prefixGlobalPosition = "GP"  // Global position counter
	prefixNamespace      = "NS:" // Namespace metadata (in metadata DB)
)

// Key separator
const keySeparator = ":"

// Integer formatting width (20 digits for lexicographic ordering)
const intWidth = 20

// formatKey joins parts with separator
func formatKey(parts ...string) []byte {
	return []byte(strings.Join(parts, keySeparator))
}

// formatMessageKey creates a message key: M:{gp_20}
func formatMessageKey(gp int64) []byte {
	return []byte(fmt.Sprintf("%s%s", prefixMessage, encodeInt64(gp)))
}

// formatStreamIndexKey creates a stream index key: SI:{stream}:{pos_20}
func formatStreamIndexKey(stream string, pos int64) []byte {
	return []byte(fmt.Sprintf("%s%s%s%s", prefixStreamIndex, stream, keySeparator, encodeInt64(pos)))
}

// formatCategoryIndexKey creates a category index key: CI:{category}:{gp_20}
func formatCategoryIndexKey(category string, gp int64) []byte {
	return []byte(fmt.Sprintf("%s%s%s%s", prefixCategoryIndex, category, keySeparator, encodeInt64(gp)))
}

// formatVersionIndexKey creates a version index key: VI:{stream}
func formatVersionIndexKey(stream string) []byte {
	return []byte(fmt.Sprintf("%s%s", prefixVersionIndex, stream))
}

// formatGlobalPositionKey creates the global position counter key: GP
func formatGlobalPositionKey() []byte {
	return []byte(prefixGlobalPosition)
}

// formatNamespaceKey creates a namespace metadata key: NS:{nsID}
func formatNamespaceKey(nsID string) []byte {
	return []byte(fmt.Sprintf("%s%s", prefixNamespace, nsID))
}

// encodeInt64 zero-pads an integer to 20 digits for lexicographic ordering
func encodeInt64(n int64) string {
	return fmt.Sprintf("%020d", n)
}

// decodeInt64 parses a zero-padded integer
func decodeInt64(b []byte) (int64, error) {
	return strconv.ParseInt(string(b), 10, 64)
}

// extractCategory extracts the category from a stream name
// Examples:
//   - "account-123" → "account"
//   - "account" → "account"
//   - "user-profile-456" → "user"
func extractCategory(stream string) string {
	if idx := strings.IndexByte(stream, '-'); idx >= 0 {
		return stream[:idx]
	}
	return stream
}

// extractCardinalID extracts the cardinal ID from a stream name
// Examples:
//   - "account-123" → "123"
//   - "account" → ""
//   - "user-profile-456" → "profile-456"
func extractCardinalID(stream string) string {
	if idx := strings.IndexByte(stream, '-'); idx >= 0 {
		return stream[idx+1:]
	}
	return ""
}

// hashCardinalID hashes a cardinal ID for consumer group distribution
// Uses FNV-1a 64-bit hash algorithm
func hashCardinalID(cardinalID string) uint64 {
	if cardinalID == "" {
		return 0
	}

	// FNV-1a 64-bit
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)

	hash := uint64(offset64)
	for i := 0; i < len(cardinalID); i++ {
		hash ^= uint64(cardinalID[i])
		hash *= prime64
	}

	return hash
}
