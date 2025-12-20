package store

import (
	"testing"
)

func TestCategory(t *testing.T) {
	tests := []struct {
		name       string
		streamName string
		expected   string
	}{
		{"simple stream", "account-123", "account"},
		{"compound ID", "account-123+456", "account"},
		{"category only", "account", "account"},
		{"multi-dash", "account-prefix-123", "account"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Category(tt.streamName)
			if result != tt.expected {
				t.Errorf("Category(%s) = %s, expected %s", tt.streamName, result, tt.expected)
			}
		})
	}
}

func TestID(t *testing.T) {
	tests := []struct {
		name       string
		streamName string
		expected   string
	}{
		{"simple stream", "account-123", "123"},
		{"compound ID", "account-123+456", "123+456"},
		{"category only", "account", ""},
		{"multi-dash", "account-prefix-123", "prefix-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ID(tt.streamName)
			if result != tt.expected {
				t.Errorf("ID(%s) = %s, expected %s", tt.streamName, result, tt.expected)
			}
		})
	}
}

func TestCardinalID(t *testing.T) {
	tests := []struct {
		name       string
		streamName string
		expected   string
	}{
		{"simple stream", "account-123", "123"},
		{"compound ID", "account-123+456", "123"},
		{"category only", "account", ""},
		{"multi-part compound", "account-123+456+789", "123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CardinalID(tt.streamName)
			if result != tt.expected {
				t.Errorf("CardinalID(%s) = %s, expected %s", tt.streamName, result, tt.expected)
			}
		})
	}
}

func TestIsCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"category only", "account", true},
		{"stream with ID", "account-123", false},
		{"compound ID", "account-123+456", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCategory(tt.input)
			if result != tt.expected {
				t.Errorf("IsCategory(%s) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHash64(t *testing.T) {
	// Test that Hash64 produces consistent results
	value := "test-value-123"
	hash1 := Hash64(value)
	hash2 := Hash64(value)

	if hash1 != hash2 {
		t.Errorf("Hash64 should produce consistent results: %d != %d", hash1, hash2)
	}

	// Test that different values produce different hashes
	value2 := "different-value"
	hash3 := Hash64(value2)

	if hash1 == hash3 {
		t.Errorf("Hash64 should produce different hashes for different values")
	}

	// Test specific known values (these should match EventoDB's hash function)
	// We'll verify this with integration tests against actual Postgres
	knownTests := []struct {
		input string
		// We don't specify expected value here since we need to verify against EventoDB
	}{
		{"123"},
		{"account-123"},
		{"user-456"},
	}

	for _, tt := range knownTests {
		hash := Hash64(tt.input)
		if hash == 0 {
			t.Errorf("Hash64(%s) returned 0, which is unlikely", tt.input)
		}
	}
}

func TestIsAssignedToConsumerMember(t *testing.T) {
	tests := []struct {
		name       string
		streamName string
		member     int64
		size       int64
		// We can't know the exact expected value without knowing the hash
		// We'll test consistency and boundary conditions
	}{
		{"simple stream member 0", "account-123", 0, 4},
		{"simple stream member 1", "account-123", 1, 4},
		{"compound ID same cardinal", "account-123+456", 0, 4},
		{"compound ID same cardinal 2", "account-123+789", 0, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAssignedToConsumerMember(tt.streamName, tt.member, tt.size)
			// Result is boolean, just verify it doesn't panic
			_ = result
		})
	}

	// Test that compound IDs with same cardinal ID map to same member
	stream1 := "account-123+abc"
	stream2 := "account-123+def"
	size := int64(4)

	var assignedMember int64 = -1
	for member := int64(0); member < size; member++ {
		if IsAssignedToConsumerMember(stream1, member, size) {
			assignedMember = member
			break
		}
	}

	if assignedMember == -1 {
		t.Error("stream1 should be assigned to at least one member")
	}

	// stream2 should be assigned to the same member
	if !IsAssignedToConsumerMember(stream2, assignedMember, size) {
		t.Errorf("streams with same cardinal ID should map to same consumer member")
	}

	// Test boundary conditions
	if IsAssignedToConsumerMember("account-123", -1, 4) {
		t.Error("negative member should return false")
	}

	if IsAssignedToConsumerMember("account-123", 4, 4) {
		t.Error("member >= size should return false")
	}

	if IsAssignedToConsumerMember("account-123", 0, 0) {
		t.Error("size <= 0 should return false")
	}

	if IsAssignedToConsumerMember("account", 0, 4) {
		t.Error("category-only stream should return false")
	}
}

func TestConsumerGroupPartitioning(t *testing.T) {
	// Test that all streams in a category are distributed across consumers
	size := int64(4)
	streams := []string{
		"account-1", "account-2", "account-3", "account-4",
		"account-5", "account-6", "account-7", "account-8",
	}

	distribution := make(map[int64]int)
	for _, stream := range streams {
		for member := int64(0); member < size; member++ {
			if IsAssignedToConsumerMember(stream, member, size) {
				distribution[member]++
				break
			}
		}
	}

	// Each stream should be assigned to exactly one member
	total := 0
	for _, count := range distribution {
		total += count
	}

	if total != len(streams) {
		t.Errorf("expected all %d streams to be assigned, got %d", len(streams), total)
	}

	// All members should receive at least one stream (with high probability for 8 streams)
	// This is probabilistic, so we just ensure distribution happened
	if len(distribution) == 0 {
		t.Error("expected streams to be distributed across members")
	}
}
