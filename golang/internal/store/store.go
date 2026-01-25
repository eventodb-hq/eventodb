// Package store provides a unified interface for message storage with support
// for multiple backend implementations (Postgres and SQLite).
//
// The store implements a EventoDB-compatible storage layer with physical namespace
// isolation, optimistic locking, category queries, and consumer group support.
//
// Basic usage:
//
//	db, _ := sql.Open("sqlite", ":memory:")
//	st, _ := sqlite.New(db, true, "")
//	defer st.Close()
//
//	ctx := context.Background()
//	st.CreateNamespace(ctx, "myapp", "token_hash", "My Application")
//
//	msg := &store.Message{
//		StreamName: "account-123",
//		Type:       "AccountCreated",
//		Data:       map[string]interface{}{"balance": 0},
//	}
//
//	result, _ := st.WriteMessage(ctx, "myapp", msg.StreamName, msg)
package store

import (
	"context"
	"time"
)

// Store is the interface for message storage backends.
//
// It provides operations for writing and reading messages, managing namespaces,
// and utility functions for stream name parsing and hashing.
//
// Two implementations are provided:
//   - postgres.PostgresStore: Production-grade Postgres backend with stored procedures
//   - sqlite.SQLiteStore: Lightweight SQLite backend with in-memory and file modes
//
// Both backends provide identical functionality and can be used interchangeably.
type Store interface {
	// Message Operations

	// WriteMessage writes a message to a stream in the specified namespace.
	//
	// The message's ID will be auto-generated if not provided. The Position is
	// determined automatically based on the stream's current version.
	//
	// If msg.ExpectedVersion is set, optimistic locking is enforced. The write
	// will fail with ErrVersionConflict if the stream's current version doesn't
	// match the expected version.
	//
	// Returns the position and global position where the message was written.
	WriteMessage(ctx context.Context, namespace, streamName string, msg *Message) (*WriteResult, error)

	// ImportBatch writes messages with explicit positions (for import/restore).
	//
	// Unlike WriteMessage, this method preserves the original Position and GlobalPosition
	// from the messages. This is used for importing exported data from another namespace
	// or restoring from backup.
	//
	// All messages in the batch are inserted in a single transaction. If any message
	// has a GlobalPosition that already exists in the namespace, the entire batch
	// fails with ErrPositionExists.
	//
	// Messages should have ID, StreamName, Type, Position, GlobalPosition, Data,
	// Metadata, and Time already set.
	ImportBatch(ctx context.Context, namespace string, messages []*Message) error

	// GetStreamMessages retrieves messages from a specific stream.
	//
	// Use opts.Position to specify the starting stream position (default: 0).
	// Use opts.BatchSize to limit the number of messages returned (default: 1000).
	//
	// Messages are returned in stream position order (oldest first).
	GetStreamMessages(ctx context.Context, namespace, streamName string, opts *GetOpts) ([]*Message, error)

	// GetCategoryMessages retrieves messages from all streams in a category.
	//
	// A category is the portion of a stream name before the '-' separator.
	// For example, "account-123" and "account-456" are both in the "account" category.
	//
	// Use opts.Position to specify the starting global position (default: 1).
	// Use opts.BatchSize to limit the number of messages returned (default: 1000).
	// Use opts.Correlation to filter by metadata.correlationStreamName.
	// Use opts.ConsumerMember and opts.ConsumerSize for consumer group partitioning.
	//
	// Messages are returned in global position order.
	GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *CategoryOpts) ([]*Message, error)

	// GetLastStreamMessage retrieves the last message from a stream.
	//
	// If msgType is nil, returns the last message of any type.
	// If msgType is specified, returns the last message of that type.
	//
	// Returns ErrStreamNotFound if the stream doesn't exist or has no messages
	// matching the criteria.
	GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*Message, error)

	// GetStreamVersion returns the current version (position of last message) of a stream.
	//
	// Returns -1 if the stream doesn't exist or has no messages.
	// Useful for optimistic locking with WriteMessage.
	GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error)

	// Namespace Operations

	// CreateNamespace creates a new namespace with physical isolation.
	//
	// In Postgres, this creates a new schema with the message store structure.
	// In SQLite, this creates a new database file (or in-memory database).
	//
	// The tokenHash should be a cryptographically hashed authentication token.
	// The description is for human-readable documentation.
	//
	// Returns ErrNamespaceExists if the namespace already exists.
	CreateNamespace(ctx context.Context, id, tokenHash, description string) error

	// DeleteNamespace permanently deletes a namespace and all its data.
	//
	// This operation cannot be undone. All messages in the namespace are deleted.
	//
	// Returns ErrNamespaceNotFound if the namespace doesn't exist.
	DeleteNamespace(ctx context.Context, id string) error

	// GetNamespace retrieves information about a namespace.
	//
	// Returns ErrNamespaceNotFound if the namespace doesn't exist.
	GetNamespace(ctx context.Context, id string) (*Namespace, error)

	// ListNamespaces returns all namespaces in the store.
	ListNamespaces(ctx context.Context) ([]*Namespace, error)

	// GetNamespaceMessageCount returns the total number of messages in a namespace.
	//
	// This is useful for getting statistics about a namespace.
	// Returns 0 if the namespace exists but has no messages.
	// Returns an error if the namespace doesn't exist.
	GetNamespaceMessageCount(ctx context.Context, namespace string) (int64, error)

	// Utility Functions (EventoDB compatible)

	// Category extracts the category name from a stream name.
	//
	// Examples:
	//   Category("account-123") → "account"
	//   Category("account") → "account"
	Category(streamName string) string

	// ID extracts the ID portion from a stream name.
	//
	// Examples:
	//   ID("account-123") → "123"
	//   ID("account-123+deposit") → "123+deposit"
	//   ID("account") → ""
	ID(streamName string) string

	// CardinalID extracts the cardinal ID (before '+') from a stream name.
	//
	// Used for consumer group partitioning with compound IDs.
	// Examples:
	//   CardinalID("account-123") → "123"
	//   CardinalID("account-123+deposit") → "123"
	//   CardinalID("account") → ""
	CardinalID(streamName string) string

	// IsCategory determines if a name represents a category (no ID part).
	//
	// Examples:
	//   IsCategory("account") → true
	//   IsCategory("account-123") → false
	IsCategory(name string) bool

	// Hash64 computes a 64-bit hash compatible with EventoDB.
	//
	// Uses MD5 hash, takes first 8 bytes, converts to int64.
	// Critical for consumer group compatibility with EventoDB.
	Hash64(value string) int64

	// Lifecycle

	// Close closes the store and releases all resources.
	//
	// Should be called when the store is no longer needed.
	// After Close is called, the store should not be used.
	Close() error
}

// Message represents a message in the message store
type Message struct {
	ID             string                 // UUID v7 (RFC 9562) - time-ordered UUID
	StreamName     string                 // Format: category-id or category-cardinalId+compoundPart
	Type           string                 // Message type name
	Position       int64                  // Stream position (gapless, 0-indexed)
	GlobalPosition int64                  // Global position (may have gaps)
	Data           map[string]interface{} // Message payload (JSON)
	Metadata       map[string]interface{} // Message metadata (JSON)
	Time           time.Time              // UTC timestamp (no timezone)

	// Optional field for optimistic locking (not stored, used for writes)
	ExpectedVersion *int64 // Expected stream version for optimistic locking
}

// StandardMetadata represents standard metadata fields (EventoDB compatible)
type StandardMetadata struct {
	CorrelationStreamName string `json:"correlationStreamName,omitempty"` // For category correlation filtering
	// Other standard fields can be added here
}

// WriteResult contains the result of a write operation
type WriteResult struct {
	Position       int64 // Stream position where message was written
	GlobalPosition int64 // Global position assigned to the message
}

// GetOpts specifies options for getting stream messages
type GetOpts struct {
	Position       int64   // Stream position (default: 0)
	GlobalPosition *int64  // Alternative: global position (mutually exclusive with Position)
	BatchSize      int64   // Number of messages (default: 1000, -1 for unlimited)
	Condition      *string // DEPRECATED: SQL condition (do not implement - security risk)
}

// CategoryOpts specifies options for getting category messages
type CategoryOpts struct {
	Position       int64   // Global position for category (default: 1)
	GlobalPosition *int64  // Alternative (same as Position for categories)
	BatchSize      int64   // Number of messages (default: 1000, -1 for unlimited)
	Correlation    *string // Filter by metadata.correlationStreamName category
	ConsumerMember *int64  // Consumer group member number (0-indexed)
	ConsumerSize   *int64  // Consumer group total size
	Condition      *string // DEPRECATED: SQL condition (do not implement - security risk)
}

// Namespace represents a namespace in the message store
type Namespace struct {
	ID          string                 // Namespace identifier
	TokenHash   string                 // Hashed authentication token
	Description string                 // Human-readable description
	CreatedAt   time.Time              // When the namespace was created
	Metadata    map[string]interface{} // Additional metadata (JSON)

	// Backend-specific fields (not exposed in interface)
	SchemaName string // Postgres: schema name
	DBPath     string // SQLite: database file path
}

// NewGetOpts creates GetOpts with default values
func NewGetOpts() *GetOpts {
	return &GetOpts{
		Position:  0,
		BatchSize: 1000,
	}
}

// NewCategoryOpts creates CategoryOpts with default values
func NewCategoryOpts() *CategoryOpts {
	return &CategoryOpts{
		Position:  1,
		BatchSize: 1000,
	}
}
