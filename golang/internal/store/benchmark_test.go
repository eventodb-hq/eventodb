package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/message-db/message-db/internal/store"
	"github.com/message-db/message-db/internal/store/postgres"
	"github.com/message-db/message-db/internal/store/sqlite"
)

// BenchmarkWriteMessage_Postgres benchmarks WriteMessage operation on Postgres backend
// Target: <10ms per operation
func BenchmarkWriteMessage_Postgres(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	pgStore := setupPostgresForBenchmark(b)
	defer pgStore.Close()

	// Create namespace once
	err := pgStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}
	defer pgStore.DeleteNamespace(ctx, "bench_ns")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &store.Message{
			StreamName: fmt.Sprintf("account-%d", i),
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"accountId": i,
				"name":      "Test Account",
			},
		}
		_, err := pgStore.WriteMessage(ctx, "bench_ns", msg.StreamName, msg)
		if err != nil {
			b.Fatalf("WriteMessage failed: %v", err)
		}
	}
}

// BenchmarkWriteMessage_SQLiteFile benchmarks WriteMessage operation on SQLite file backend
// Target: <5ms per operation
func BenchmarkWriteMessage_SQLiteFile(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	sqliteStore := setupSQLiteFileForBenchmark(b)
	defer sqliteStore.Close()

	// Create namespace once
	err := sqliteStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}
	defer sqliteStore.DeleteNamespace(ctx, "bench_ns")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &store.Message{
			StreamName: fmt.Sprintf("account-%d", i),
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"accountId": i,
				"name":      "Test Account",
			},
		}
		_, err := sqliteStore.WriteMessage(ctx, "bench_ns", msg.StreamName, msg)
		if err != nil {
			b.Fatalf("WriteMessage failed: %v", err)
		}
	}
}

// BenchmarkWriteMessage_SQLiteMemory benchmarks WriteMessage operation on SQLite in-memory backend
// Target: <1ms per operation
func BenchmarkWriteMessage_SQLiteMemory(b *testing.B) {
	ctx := context.Background()
	sqliteStore := setupSQLiteMemoryForBenchmark(b)
	defer sqliteStore.Close()

	// Create namespace once
	err := sqliteStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &store.Message{
			StreamName: fmt.Sprintf("account-%d", i),
			Type:       "AccountCreated",
			Data: map[string]interface{}{
				"accountId": i,
				"name":      "Test Account",
			},
		}
		_, err := sqliteStore.WriteMessage(ctx, "bench_ns", msg.StreamName, msg)
		if err != nil {
			b.Fatalf("WriteMessage failed: %v", err)
		}
	}
}

// BenchmarkGetStreamMessages_Postgres benchmarks GetStreamMessages operation on Postgres
// Target: <15ms for 10 messages
func BenchmarkGetStreamMessages_Postgres(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	pgStore := setupPostgresForBenchmark(b)
	defer pgStore.Close()

	// Setup: create namespace and write 100 messages
	err := pgStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}
	defer pgStore.DeleteNamespace(ctx, "bench_ns")

	streamName := "account-123"
	for i := 0; i < 100; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "AccountEvent",
			Data: map[string]interface{}{
				"sequence": i,
			},
		}
		_, err := pgStore.WriteMessage(ctx, "bench_ns", streamName, msg)
		if err != nil {
			b.Fatalf("WriteMessage failed: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := &store.GetOpts{
			Position:  0,
			BatchSize: 10,
		}
		msgs, err := pgStore.GetStreamMessages(ctx, "bench_ns", streamName, opts)
		if err != nil {
			b.Fatalf("GetStreamMessages failed: %v", err)
		}
		if len(msgs) != 10 {
			b.Fatalf("Expected 10 messages, got %d", len(msgs))
		}
	}
}

// BenchmarkGetStreamMessages_SQLiteFile benchmarks GetStreamMessages on SQLite file backend
// Target: <8ms for 10 messages
func BenchmarkGetStreamMessages_SQLiteFile(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	sqliteStore := setupSQLiteFileForBenchmark(b)
	defer sqliteStore.Close()

	// Setup: create namespace and write 100 messages
	err := sqliteStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}
	defer sqliteStore.DeleteNamespace(ctx, "bench_ns")

	streamName := "account-123"
	for i := 0; i < 100; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "AccountEvent",
			Data: map[string]interface{}{
				"sequence": i,
			},
		}
		_, err := sqliteStore.WriteMessage(ctx, "bench_ns", streamName, msg)
		if err != nil {
			b.Fatalf("WriteMessage failed: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := &store.GetOpts{
			Position:  0,
			BatchSize: 10,
		}
		msgs, err := sqliteStore.GetStreamMessages(ctx, "bench_ns", streamName, opts)
		if err != nil {
			b.Fatalf("GetStreamMessages failed: %v", err)
		}
		if len(msgs) != 10 {
			b.Fatalf("Expected 10 messages, got %d", len(msgs))
		}
	}
}

// BenchmarkGetStreamMessages_SQLiteMemory benchmarks GetStreamMessages on SQLite in-memory
// Target: <2ms for 10 messages
func BenchmarkGetStreamMessages_SQLiteMemory(b *testing.B) {
	ctx := context.Background()
	sqliteStore := setupSQLiteMemoryForBenchmark(b)
	defer sqliteStore.Close()

	// Setup: create namespace and write 100 messages
	err := sqliteStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}

	streamName := "account-123"
	for i := 0; i < 100; i++ {
		msg := &store.Message{
			StreamName: streamName,
			Type:       "AccountEvent",
			Data: map[string]interface{}{
				"sequence": i,
			},
		}
		_, err := sqliteStore.WriteMessage(ctx, "bench_ns", streamName, msg)
		if err != nil {
			b.Fatalf("WriteMessage failed: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := &store.GetOpts{
			Position:  0,
			BatchSize: 10,
		}
		msgs, err := sqliteStore.GetStreamMessages(ctx, "bench_ns", streamName, opts)
		if err != nil {
			b.Fatalf("GetStreamMessages failed: %v", err)
		}
		if len(msgs) != 10 {
			b.Fatalf("Expected 10 messages, got %d", len(msgs))
		}
	}
}

// BenchmarkGetCategoryMessages_Postgres benchmarks GetCategoryMessages on Postgres
// Target: <50ms for 100 messages
func BenchmarkGetCategoryMessages_Postgres(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	pgStore := setupPostgresForBenchmark(b)
	defer pgStore.Close()

	// Setup: create namespace and write messages to multiple streams in same category
	err := pgStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}
	defer pgStore.DeleteNamespace(ctx, "bench_ns")

	// Write 100 messages across 10 streams
	for streamIdx := 0; streamIdx < 10; streamIdx++ {
		for msgIdx := 0; msgIdx < 10; msgIdx++ {
			msg := &store.Message{
				StreamName: fmt.Sprintf("account-%d", streamIdx),
				Type:       "AccountEvent",
				Data: map[string]interface{}{
					"sequence": msgIdx,
				},
			}
			_, err := pgStore.WriteMessage(ctx, "bench_ns", msg.StreamName, msg)
			if err != nil {
				b.Fatalf("WriteMessage failed: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := &store.CategoryOpts{
			Position:  1,
			BatchSize: 100,
		}
		msgs, err := pgStore.GetCategoryMessages(ctx, "bench_ns", "account", opts)
		if err != nil {
			b.Fatalf("GetCategoryMessages failed: %v", err)
		}
		if len(msgs) != 100 {
			b.Fatalf("Expected 100 messages, got %d", len(msgs))
		}
	}
}

// BenchmarkGetCategoryMessages_SQLiteFile benchmarks GetCategoryMessages on SQLite file
// Target: <30ms for 100 messages
func BenchmarkGetCategoryMessages_SQLiteFile(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	sqliteStore := setupSQLiteFileForBenchmark(b)
	defer sqliteStore.Close()

	// Setup: create namespace and write messages to multiple streams
	err := sqliteStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}
	defer sqliteStore.DeleteNamespace(ctx, "bench_ns")

	// Write 100 messages across 10 streams
	for streamIdx := 0; streamIdx < 10; streamIdx++ {
		for msgIdx := 0; msgIdx < 10; msgIdx++ {
			msg := &store.Message{
				StreamName: fmt.Sprintf("account-%d", streamIdx),
				Type:       "AccountEvent",
				Data: map[string]interface{}{
					"sequence": msgIdx,
				},
			}
			_, err := sqliteStore.WriteMessage(ctx, "bench_ns", msg.StreamName, msg)
			if err != nil {
				b.Fatalf("WriteMessage failed: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := &store.CategoryOpts{
			Position:  1,
			BatchSize: 100,
		}
		msgs, err := sqliteStore.GetCategoryMessages(ctx, "bench_ns", "account", opts)
		if err != nil {
			b.Fatalf("GetCategoryMessages failed: %v", err)
		}
		if len(msgs) != 100 {
			b.Fatalf("Expected 100 messages, got %d", len(msgs))
		}
	}
}

// BenchmarkGetCategoryMessages_SQLiteMemory benchmarks GetCategoryMessages on SQLite in-memory
// Target: <10ms for 100 messages
func BenchmarkGetCategoryMessages_SQLiteMemory(b *testing.B) {
	ctx := context.Background()
	sqliteStore := setupSQLiteMemoryForBenchmark(b)
	defer sqliteStore.Close()

	// Setup: create namespace and write messages
	err := sqliteStore.CreateNamespace(ctx, "bench_ns", "bench_token_hash", "Benchmark namespace")
	if err != nil {
		b.Fatalf("Failed to create namespace: %v", err)
	}

	// Write 100 messages across 10 streams
	for streamIdx := 0; streamIdx < 10; streamIdx++ {
		for msgIdx := 0; msgIdx < 10; msgIdx++ {
			msg := &store.Message{
				StreamName: fmt.Sprintf("account-%d", streamIdx),
				Type:       "AccountEvent",
				Data: map[string]interface{}{
					"sequence": msgIdx,
				},
			}
			_, err := sqliteStore.WriteMessage(ctx, "bench_ns", msg.StreamName, msg)
			if err != nil {
				b.Fatalf("WriteMessage failed: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opts := &store.CategoryOpts{
			Position:  1,
			BatchSize: 100,
		}
		msgs, err := sqliteStore.GetCategoryMessages(ctx, "bench_ns", "account", opts)
		if err != nil {
			b.Fatalf("GetCategoryMessages failed: %v", err)
		}
		if len(msgs) != 100 {
			b.Fatalf("Expected 100 messages, got %d", len(msgs))
		}
	}
}

// Helper functions for benchmark setup

func setupPostgresForBenchmark(b *testing.B) store.Store {
	b.Helper()

	connStr := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		b.Fatalf("Failed to connect to Postgres: %v", err)
	}

	pgStore, err := postgres.New(db)
	if err != nil {
		b.Fatalf("Failed to create Postgres store: %v", err)
	}

	return pgStore
}

func setupSQLiteFileForBenchmark(b *testing.B) store.Store {
	b.Helper()

	// Create temp directory for benchmark databases
	tmpDir := fmt.Sprintf("/tmp/messagedb-bench-%d", os.Getpid())
	os.MkdirAll(tmpDir, 0755)

	metadataPath := fmt.Sprintf("%s/metadata.db", tmpDir)
	db, err := sql.Open("sqlite", metadataPath)
	if err != nil {
		b.Fatalf("Failed to create SQLite database: %v", err)
	}

	config := &sqlite.Config{
		TestMode: false,
		DataDir:  tmpDir,
	}

	sqliteStore, err := sqlite.New(db, config)
	if err != nil {
		b.Fatalf("Failed to create SQLite store: %v", err)
	}

	b.Cleanup(func() {
		sqliteStore.Close()
		os.RemoveAll(tmpDir)
	})

	return sqliteStore
}

func setupSQLiteMemoryForBenchmark(b *testing.B) store.Store {
	b.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		b.Fatalf("Failed to create in-memory SQLite database: %v", err)
	}

	config := &sqlite.Config{
		TestMode: true,
		DataDir:  "",
	}

	sqliteStore, err := sqlite.New(db, config)
	if err != nil {
		b.Fatalf("Failed to create SQLite store: %v", err)
	}

	return sqliteStore
}
