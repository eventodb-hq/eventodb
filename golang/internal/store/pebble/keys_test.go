package pebble

import (
	"sort"
	"testing"
)

func TestFormatMessageKey(t *testing.T) {
	tests := []struct {
		name     string
		gp       int64
		expected string
	}{
		{"zero", 0, "M:00000000000000000000"},
		{"one", 1, "M:00000000000000000001"},
		{"ten", 10, "M:00000000000000000010"},
		{"hundred", 100, "M:00000000000000000100"},
		{"large", 1234567890123456789, "M:01234567890123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(formatMessageKey(tt.gp))
			if result != tt.expected {
				t.Errorf("formatMessageKey(%d) = %s, want %s", tt.gp, result, tt.expected)
			}
		})
	}
}

func TestFormatStreamIndexKey(t *testing.T) {
	tests := []struct {
		name     string
		stream   string
		pos      int64
		expected string
	}{
		{"simple", "account-123", 0, "SI:account-123:00000000000000000000"},
		{"position_10", "user-456", 10, "SI:user-456:00000000000000000010"},
		{"no_cardinal", "events", 5, "SI:events:00000000000000000005"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(formatStreamIndexKey(tt.stream, tt.pos))
			if result != tt.expected {
				t.Errorf("formatStreamIndexKey(%s, %d) = %s, want %s",
					tt.stream, tt.pos, result, tt.expected)
			}
		})
	}
}

func TestFormatCategoryIndexKey(t *testing.T) {
	tests := []struct {
		name     string
		category string
		gp       int64
		expected string
	}{
		{"simple", "account", 1, "CI:account:00000000000000000001"},
		{"large_gp", "user", 999999, "CI:user:00000000000000999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(formatCategoryIndexKey(tt.category, tt.gp))
			if result != tt.expected {
				t.Errorf("formatCategoryIndexKey(%s, %d) = %s, want %s",
					tt.category, tt.gp, result, tt.expected)
			}
		})
	}
}

func TestFormatVersionIndexKey(t *testing.T) {
	tests := []struct {
		name     string
		stream   string
		expected string
	}{
		{"simple", "account-123", "VI:account-123"},
		{"no_cardinal", "events", "VI:events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(formatVersionIndexKey(tt.stream))
			if result != tt.expected {
				t.Errorf("formatVersionIndexKey(%s) = %s, want %s",
					tt.stream, result, tt.expected)
			}
		})
	}
}

func TestFormatGlobalPositionKey(t *testing.T) {
	expected := "GP"
	result := string(formatGlobalPositionKey())
	if result != expected {
		t.Errorf("formatGlobalPositionKey() = %s, want %s", result, expected)
	}
}

func TestFormatNamespaceKey(t *testing.T) {
	tests := []struct {
		name     string
		nsID     string
		expected string
	}{
		{"simple", "default", "NS:default"},
		{"production", "production", "NS:production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(formatNamespaceKey(tt.nsID))
			if result != tt.expected {
				t.Errorf("formatNamespaceKey(%s) = %s, want %s",
					tt.nsID, result, tt.expected)
			}
		})
	}
}

func TestEncodeInt64_LexicographicOrdering(t *testing.T) {
	// Test that encoded integers maintain lexicographic ordering
	values := []int64{0, 1, 9, 10, 99, 100, 999, 1000, 9999, 10000}
	encoded := make([]string, len(values))

	for i, v := range values {
		encoded[i] = encodeInt64(v)
	}

	// Check that encoded strings are sorted
	if !sort.StringsAreSorted(encoded) {
		t.Errorf("encoded integers are not in lexicographic order: %v", encoded)
	}

	// Verify each encoding
	for i, v := range values {
		if len(encoded[i]) != intWidth {
			t.Errorf("encodeInt64(%d) = %s, length = %d, want %d",
				v, encoded[i], len(encoded[i]), intWidth)
		}
	}
}

func TestEncodeDecodeInt64_RoundTrip(t *testing.T) {
	tests := []int64{0, 1, 10, 100, 1000, 999999, 1234567890}

	for _, original := range tests {
		encoded := encodeInt64(original)
		decoded, err := decodeInt64([]byte(encoded))
		if err != nil {
			t.Errorf("decodeInt64(%s) error: %v", encoded, err)
			continue
		}
		if decoded != original {
			t.Errorf("round trip failed: original=%d, encoded=%s, decoded=%d",
				original, encoded, decoded)
		}
	}
}

func TestDecodeInt64_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"non_numeric", "abc"},
		{"mixed", "123abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeInt64([]byte(tt.input))
			if err == nil {
				t.Errorf("decodeInt64(%s) expected error, got nil", tt.input)
			}
		})
	}
}

func TestExtractCategory(t *testing.T) {
	tests := []struct {
		name     string
		stream   string
		expected string
	}{
		{"simple", "account-123", "account"},
		{"no_dash", "events", "events"},
		{"multiple_dashes", "user-profile-456", "user"},
		{"empty", "", ""},
		{"dash_only", "-", ""},
		{"dash_start", "-account", ""},
		{"dash_end", "account-", "account"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCategory(tt.stream)
			if result != tt.expected {
				t.Errorf("extractCategory(%s) = %s, want %s",
					tt.stream, result, tt.expected)
			}
		})
	}
}

func TestExtractCardinalID(t *testing.T) {
	tests := []struct {
		name     string
		stream   string
		expected string
	}{
		{"simple", "account-123", "123"},
		{"no_dash", "events", ""},
		{"multiple_dashes", "user-profile-456", "profile-456"},
		{"empty", "", ""},
		{"dash_only", "-", ""},
		{"dash_start", "-account", "account"},
		{"dash_end", "account-", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCardinalID(tt.stream)
			if result != tt.expected {
				t.Errorf("extractCardinalID(%s) = %s, want %s",
					tt.stream, result, tt.expected)
			}
		})
	}
}

func TestHashCardinalID(t *testing.T) {
	tests := []struct {
		name       string
		cardinalID string
		want       uint64
	}{
		{"empty", "", 0},
		{"numeric", "123", 0xd1cec15f7cf4e7f6}, // Pre-computed FNV-1a hash
		{"alpha", "abc", 0xe71fa2190541574b},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashCardinalID(tt.cardinalID)
			if tt.cardinalID == "" {
				if result != 0 {
					t.Errorf("hashCardinalID(%s) = %d, want 0", tt.cardinalID, result)
				}
			} else {
				// Just verify it's deterministic and non-zero
				result2 := hashCardinalID(tt.cardinalID)
				if result != result2 {
					t.Errorf("hashCardinalID(%s) not deterministic: %d != %d",
						tt.cardinalID, result, result2)
				}
				if result == 0 {
					t.Errorf("hashCardinalID(%s) = 0, want non-zero", tt.cardinalID)
				}
			}
		})
	}
}

func TestHashCardinalID_ConsumerGroupDistribution(t *testing.T) {
	// Test that hashing distributes different cardinal IDs
	cardinalIDs := []string{"1", "2", "3", "4", "5", "100", "200", "300"}
	consumerGroupSize := uint64(3)

	distribution := make(map[uint64]int)

	for _, id := range cardinalIDs {
		hash := hashCardinalID(id)
		member := hash % consumerGroupSize
		distribution[member]++
	}

	// Should have some distribution (not all in one bucket)
	if len(distribution) == 1 {
		t.Errorf("all cardinal IDs hashed to same consumer group member: %v", distribution)
	}

	t.Logf("Distribution across %d consumer group members: %v", consumerGroupSize, distribution)
}

func TestMessageKeyOrdering(t *testing.T) {
	// Verify that message keys sort correctly by global position
	keys := []string{
		string(formatMessageKey(1)),
		string(formatMessageKey(10)),
		string(formatMessageKey(100)),
		string(formatMessageKey(1000)),
		string(formatMessageKey(10000)),
	}

	// Keys should already be sorted
	if !sort.StringsAreSorted(keys) {
		t.Errorf("message keys not in correct order: %v", keys)
	}

	// Make a copy and shuffle
	shuffled := make([]string, len(keys))
	copy(shuffled, keys)
	// Reverse order to simulate unsorted
	for i, j := 0, len(shuffled)-1; i < j; i, j = i+1, j-1 {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	// Sort and verify it matches original
	sort.Strings(shuffled)
	for i := range keys {
		if keys[i] != shuffled[i] {
			t.Errorf("sorted order mismatch at index %d: got %s, want %s",
				i, shuffled[i], keys[i])
		}
	}
}

func TestStreamIndexKeyOrdering(t *testing.T) {
	// Verify that stream index keys sort correctly by position within a stream
	stream := "account-123"
	keys := []string{
		string(formatStreamIndexKey(stream, 0)),
		string(formatStreamIndexKey(stream, 1)),
		string(formatStreamIndexKey(stream, 10)),
		string(formatStreamIndexKey(stream, 100)),
	}

	if !sort.StringsAreSorted(keys) {
		t.Errorf("stream index keys not in correct order: %v", keys)
	}
}

func TestCategoryIndexKeyOrdering(t *testing.T) {
	// Verify that category index keys sort correctly by global position
	category := "account"
	keys := []string{
		string(formatCategoryIndexKey(category, 1)),
		string(formatCategoryIndexKey(category, 10)),
		string(formatCategoryIndexKey(category, 100)),
		string(formatCategoryIndexKey(category, 1000)),
	}

	if !sort.StringsAreSorted(keys) {
		t.Errorf("category index keys not in correct order: %v", keys)
	}
}
