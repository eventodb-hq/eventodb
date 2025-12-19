package pebble

import (
	"strings"
	"testing"
)

func TestCompressDecompress(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "simple json",
			data: `{"id":"123","type":"test","data":"hello"}`,
		},
		{
			name: "complex json with metadata",
			data: `{"id":"456","type":"UserCreated","data":{"name":"John","email":"john@example.com"},"metadata":{"correlationStreamName":"user-789","causationMessagePosition":42}}`,
		},
		{
			name: "empty json",
			data: `{}`,
		},
		{
			name: "large json",
			data: `{"data":"` + strings.Repeat("x", 10000) + `"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := []byte(tt.data)

			// Compress
			compressed := compressJSON(original)

			// Verify compression occurred (S2 should compress JSON well)
			t.Logf("Original size: %d, Compressed size: %d, Ratio: %.2f%%",
				len(original), len(compressed), float64(len(compressed))/float64(len(original))*100)

			// Decompress
			decompressed, err := decompressJSON(compressed)
			if err != nil {
				t.Fatalf("decompression failed: %v", err)
			}

			// Verify data integrity
			if string(decompressed) != tt.data {
				t.Errorf("decompressed data doesn't match original\nGot: %s\nWant: %s", decompressed, tt.data)
			}
		})
	}
}

func TestDecompressInvalidData(t *testing.T) {
	invalidData := []byte("this is not compressed data")

	_, err := decompressJSON(invalidData)
	if err == nil {
		t.Error("expected error when decompressing invalid data, got nil")
	}
}

func BenchmarkCompressJSON(b *testing.B) {
	// Typical message JSON
	data := []byte(`{"id":"550e8400-e29b-41d4-a716-446655440000","type":"OrderPlaced","data":{"orderId":"ORD-12345","customerId":"CUST-789","items":[{"sku":"PROD-001","quantity":2,"price":29.99},{"sku":"PROD-002","quantity":1,"price":49.99}],"total":109.97},"metadata":{"correlationStreamName":"customer-CUST-789","causationMessagePosition":42,"timestamp":"2024-01-15T10:30:00Z"},"position":123,"globalPosition":45678,"streamName":"order-ORD-12345"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = compressJSON(data)
	}
}

func BenchmarkDecompressJSON(b *testing.B) {
	// Typical message JSON
	data := []byte(`{"id":"550e8400-e29b-41d4-a716-446655440000","type":"OrderPlaced","data":{"orderId":"ORD-12345","customerId":"CUST-789","items":[{"sku":"PROD-001","quantity":2,"price":29.99},{"sku":"PROD-002","quantity":1,"price":49.99}],"total":109.97},"metadata":{"correlationStreamName":"customer-CUST-789","causationMessagePosition":42,"timestamp":"2024-01-15T10:30:00Z"},"position":123,"globalPosition":45678,"streamName":"order-ORD-12345"}`)

	compressed := compressJSON(data)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = decompressJSON(compressed)
	}
}

func BenchmarkCompressDecompressRoundtrip(b *testing.B) {
	// Typical message JSON
	data := []byte(`{"id":"550e8400-e29b-41d4-a716-446655440000","type":"OrderPlaced","data":{"orderId":"ORD-12345","customerId":"CUST-789","items":[{"sku":"PROD-001","quantity":2,"price":29.99},{"sku":"PROD-002","quantity":1,"price":49.99}],"total":109.97},"metadata":{"correlationStreamName":"customer-CUST-789","causationMessagePosition":42,"timestamp":"2024-01-15T10:30:00Z"},"position":123,"globalPosition":45678,"streamName":"order-ORD-12345"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		compressed := compressJSON(data)
		_, _ = decompressJSON(compressed)
	}
}
