package store

import (
	"strings"
	"testing"
)

// Baseline implementations using SplitN
func CategorySplit(streamName string) string {
	parts := strings.SplitN(streamName, "-", 2)
	return parts[0]
}

func IDSplit(streamName string) string {
	parts := strings.SplitN(streamName, "-", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// Optimized implementations using IndexByte
func CategoryIndex(streamName string) string {
	if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
		return streamName[:idx]
	}
	return streamName
}

func IDIndex(streamName string) string {
	if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
		return streamName[idx+1:]
	}
	return ""
}

func CardinalIDIndex(streamName string) string {
	// Extract ID part
	if idx := strings.IndexByte(streamName, '-'); idx >= 0 {
		id := streamName[idx+1:]
		// Extract part before '+' for compound IDs
		if plusIdx := strings.IndexByte(id, '+'); plusIdx >= 0 {
			return id[:plusIdx]
		}
		return id
	}
	return ""
}

// Benchmarks
func BenchmarkCategory(b *testing.B) {
	testCases := []struct {
		name   string
		stream string
	}{
		{"simple", "account-123"},
		{"compound", "account-123+456"},
		{"no-id", "account"},
		{"long", "accountTransactionHistory-550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tc := range testCases {
		b.Run(tc.name+"/split", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CategorySplit(tc.stream)
			}
		})

		b.Run(tc.name+"/index", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CategoryIndex(tc.stream)
			}
		})
	}
}

func BenchmarkID(b *testing.B) {
	testCases := []struct {
		name   string
		stream string
	}{
		{"simple", "account-123"},
		{"compound", "account-123+456"},
		{"no-id", "account"},
		{"long", "accountTransactionHistory-550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tc := range testCases {
		b.Run(tc.name+"/split", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = IDSplit(tc.stream)
			}
		})

		b.Run(tc.name+"/index", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = IDIndex(tc.stream)
			}
		})
	}
}

func BenchmarkCardinalID(b *testing.B) {
	testCases := []struct {
		name   string
		stream string
	}{
		{"simple", "account-123"},
		{"compound", "account-123+456"},
		{"no-id", "account"},
		{"long-compound", "transaction-550e8400-e29b-41d4-a716-446655440000+region-us-west"},
	}

	for _, tc := range testCases {
		b.Run(tc.name+"/current", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CardinalID(tc.stream)
			}
		})

		b.Run(tc.name+"/optimized", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = CardinalIDIndex(tc.stream)
			}
		})
	}
}

// Correctness tests
func TestCategoryOptimization(t *testing.T) {
	testCases := []struct {
		stream   string
		expected string
	}{
		{"account-123", "account"},
		{"account-123+456", "account"},
		{"account", "account"},
		{"", ""},
		{"accountTransactionHistory-uuid", "accountTransactionHistory"},
	}

	for _, tc := range testCases {
		t.Run(tc.stream, func(t *testing.T) {
			baseline := CategorySplit(tc.stream)
			optimized := CategoryIndex(tc.stream)

			if baseline != tc.expected {
				t.Errorf("baseline failed: got %q, want %q", baseline, tc.expected)
			}
			if optimized != tc.expected {
				t.Errorf("optimized failed: got %q, want %q", optimized, tc.expected)
			}
			if baseline != optimized {
				t.Errorf("mismatch: baseline=%q, optimized=%q", baseline, optimized)
			}
		})
	}
}

func TestIDOptimization(t *testing.T) {
	testCases := []struct {
		stream   string
		expected string
	}{
		{"account-123", "123"},
		{"account-123+456", "123+456"},
		{"account", ""},
		{"", ""},
		{"transaction-uuid-with-dashes", "uuid-with-dashes"},
	}

	for _, tc := range testCases {
		t.Run(tc.stream, func(t *testing.T) {
			baseline := IDSplit(tc.stream)
			optimized := IDIndex(tc.stream)

			if baseline != tc.expected {
				t.Errorf("baseline failed: got %q, want %q", baseline, tc.expected)
			}
			if optimized != tc.expected {
				t.Errorf("optimized failed: got %q, want %q", optimized, tc.expected)
			}
			if baseline != optimized {
				t.Errorf("mismatch: baseline=%q, optimized=%q", baseline, optimized)
			}
		})
	}
}

func TestCardinalIDOptimization(t *testing.T) {
	testCases := []struct {
		stream   string
		expected string
	}{
		{"account-123", "123"},
		{"account-123+456", "123"},
		{"account", ""},
		{"", ""},
		{"transaction-uuid+region", "uuid"},
	}

	for _, tc := range testCases {
		t.Run(tc.stream, func(t *testing.T) {
			baseline := CardinalID(tc.stream)
			optimized := CardinalIDIndex(tc.stream)

			if baseline != tc.expected {
				t.Errorf("baseline failed: got %q, want %q", baseline, tc.expected)
			}
			if optimized != tc.expected {
				t.Errorf("optimized failed: got %q, want %q", optimized, tc.expected)
			}
			if baseline != optimized {
				t.Errorf("mismatch: baseline=%q, optimized=%q", baseline, optimized)
			}
		})
	}
}
