package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/sqlite"
	_ "modernc.org/sqlite"
)

// BenchmarkReadOperations measures the impact of slice pre-allocation
func BenchmarkReadOperations(b *testing.B) {
	ctx := context.Background()

	// Create in-memory metadata database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	// Initialize store
	config := &sqlite.Config{
		TestMode: true,
		DataDir:  b.TempDir(),
	}
	s, err := sqlite.New(db, config)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	// Create test namespace
	namespace := "bench-test"
	if err := s.CreateNamespace(ctx, namespace, "test-hash", "Benchmark test"); err != nil {
		b.Fatal(err)
	}
	defer s.DeleteNamespace(ctx, namespace)

	// Write test data
	streamName := "test-stream-123"
	for i := 0; i < 100; i++ {
		msg := &store.Message{
			Type: "TestEvent",
			Data: map[string]interface{}{
				"index":   i,
				"payload": "test data for benchmarking",
			},
		}
		if _, err := s.WriteMessage(ctx, namespace, streamName, msg); err != nil {
			b.Fatal(err)
		}
	}

	b.Run("GetStreamMessages_BatchSize10", func(b *testing.B) {
		opts := &store.GetOpts{
			Position:  0,
			BatchSize: 10,
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := s.GetStreamMessages(ctx, namespace, streamName, opts)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetStreamMessages_BatchSize100", func(b *testing.B) {
		opts := &store.GetOpts{
			Position:  0,
			BatchSize: 100,
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := s.GetStreamMessages(ctx, namespace, streamName, opts)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetStreamMessages_BatchSize1000", func(b *testing.B) {
		opts := &store.GetOpts{
			Position:  0,
			BatchSize: 1000,
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := s.GetStreamMessages(ctx, namespace, streamName, opts)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// Category queries
	categoryName := "test-stream"
	b.Run("GetCategoryMessages_BatchSize10", func(b *testing.B) {
		opts := &store.CategoryOpts{
			Position:  0,
			BatchSize: 10,
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := s.GetCategoryMessages(ctx, namespace, categoryName, opts)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GetCategoryMessages_BatchSize100", func(b *testing.B) {
		opts := &store.CategoryOpts{
			Position:  0,
			BatchSize: 100,
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := s.GetCategoryMessages(ctx, namespace, categoryName, opts)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
