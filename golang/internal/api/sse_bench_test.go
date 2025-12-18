package api

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
)

// Baseline: Current implementation (allocates Poke every time)
func sendPokeBaseline(streamName string, position, globalPosition int64) ([]byte, error) {
	poke := Poke{
		Stream:         streamName,
		Position:       position,
		GlobalPosition: globalPosition,
	}
	return json.Marshal(poke)
}

// Optimized: Using sync.Pool for Poke objects (uses the global pokePool from sse.go)
func sendPokePooled(streamName string, position, globalPosition int64) ([]byte, error) {
	poke := pokePool.Get().(*Poke)
	defer pokePool.Put(poke)

	poke.Stream = streamName
	poke.Position = position
	poke.GlobalPosition = globalPosition

	return json.Marshal(poke)
}

// Even more optimized: Pool both Poke and bytes.Buffer
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func sendPokePooledWithBuffer(streamName string, position, globalPosition int64) ([]byte, error) {
	poke := pokePool.Get().(*Poke)
	defer pokePool.Put(poke)

	poke.Stream = streamName
	poke.Position = position
	poke.GlobalPosition = globalPosition

	buf := bufferPool.Get().(*bytes.Buffer)
	defer bufferPool.Put(buf)
	buf.Reset()

	enc := json.NewEncoder(buf)
	if err := enc.Encode(poke); err != nil {
		return nil, err
	}

	// Need to copy because we're returning the buffer to the pool
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

// Benchmarks
func BenchmarkSendPoke(b *testing.B) {
	testCases := []struct {
		name           string
		streamName     string
		position       int64
		globalPosition int64
	}{
		{"short_stream", "account-123", 42, 1000},
		{"long_stream", "accountTransactionHistory-550e8400-e29b-41d4-a716-446655440000", 999, 50000},
		{"category", "account", 0, 0},
	}

	for _, tc := range testCases {
		b.Run(tc.name+"/baseline", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := sendPokeBaseline(tc.streamName, tc.position, tc.globalPosition)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(tc.name+"/pooled", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := sendPokePooled(tc.streamName, tc.position, tc.globalPosition)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(tc.name+"/pooled_with_buffer", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := sendPokePooledWithBuffer(tc.streamName, tc.position, tc.globalPosition)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Benchmark concurrent access (more realistic for SSE)
func BenchmarkSendPokeConcurrent(b *testing.B) {
	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			streamName := "account-123"
			position := int64(0)
			globalPosition := int64(1000)
			for pb.Next() {
				_, err := sendPokeBaseline(streamName, position, globalPosition)
				if err != nil {
					b.Fatal(err)
				}
				position++
				globalPosition++
			}
		})
	})

	b.Run("pooled", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			streamName := "account-123"
			position := int64(0)
			globalPosition := int64(1000)
			for pb.Next() {
				_, err := sendPokePooled(streamName, position, globalPosition)
				if err != nil {
					b.Fatal(err)
				}
				position++
				globalPosition++
			}
		})
	})

	b.Run("pooled_with_buffer", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			streamName := "account-123"
			position := int64(0)
			globalPosition := int64(1000)
			for pb.Next() {
				_, err := sendPokePooledWithBuffer(streamName, position, globalPosition)
				if err != nil {
					b.Fatal(err)
				}
				position++
				globalPosition++
			}
		})
	})
}

// Correctness test
func TestPokePoolingCorrectness(t *testing.T) {
	testCases := []struct {
		name           string
		streamName     string
		position       int64
		globalPosition int64
	}{
		{"simple", "account-123", 42, 1000},
		{"zero", "test", 0, 0},
		{"large", "stream-999", 999999, 888888},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			baseline, err1 := sendPokeBaseline(tc.streamName, tc.position, tc.globalPosition)
			pooled, err2 := sendPokePooled(tc.streamName, tc.position, tc.globalPosition)
			pooledBuf, err3 := sendPokePooledWithBuffer(tc.streamName, tc.position, tc.globalPosition)

			if err1 != nil || err2 != nil || err3 != nil {
				t.Fatalf("errors: %v, %v, %v", err1, err2, err3)
			}

			// Parse and compare
			var p1, p2, p3 Poke
			if err := json.Unmarshal(baseline, &p1); err != nil {
				t.Fatalf("unmarshal baseline: %v", err)
			}
			if err := json.Unmarshal(pooled, &p2); err != nil {
				t.Fatalf("unmarshal pooled: %v", err)
			}
			// pooledBuf has trailing newline from Encoder
			pooledBuf = bytes.TrimSpace(pooledBuf)
			if err := json.Unmarshal(pooledBuf, &p3); err != nil {
				t.Fatalf("unmarshal pooled_buf: %v", err)
			}

			if p1.Stream != tc.streamName || p2.Stream != tc.streamName || p3.Stream != tc.streamName {
				t.Errorf("stream mismatch: want %s, got baseline=%s, pooled=%s, pooledBuf=%s",
					tc.streamName, p1.Stream, p2.Stream, p3.Stream)
			}
			if p1.Position != tc.position || p2.Position != tc.position || p3.Position != tc.position {
				t.Errorf("position mismatch: want %d, got baseline=%d, pooled=%d, pooledBuf=%d",
					tc.position, p1.Position, p2.Position, p3.Position)
			}
			if p1.GlobalPosition != tc.globalPosition || p2.GlobalPosition != tc.globalPosition || p3.GlobalPosition != tc.globalPosition {
				t.Errorf("globalPosition mismatch: want %d, got baseline=%d, pooled=%d, pooledBuf=%d",
					tc.globalPosition, p1.GlobalPosition, p2.GlobalPosition, p3.GlobalPosition)
			}
		})
	}
}

// Test pool reuse
func TestPokePoolReuse(t *testing.T) {
	// Get two pokes from pool
	poke1 := pokePool.Get().(*Poke)
	poke1.Stream = "test-1"
	poke1.Position = 1
	poke1.GlobalPosition = 100

	// Return to pool
	pokePool.Put(poke1)

	// Get another poke (might be the same object)
	poke2 := pokePool.Get().(*Poke)

	// Should be a valid Poke object (either new or recycled)
	// If recycled, it will have old values that we'll overwrite
	poke2.Stream = "test-2"
	poke2.Position = 2
	poke2.GlobalPosition = 200

	if poke2.Stream != "test-2" {
		t.Errorf("expected Stream to be updated to 'test-2', got %s", poke2.Stream)
	}
	if poke2.Position != 2 {
		t.Errorf("expected Position to be updated to 2, got %d", poke2.Position)
	}

	pokePool.Put(poke2)
}
