package store

import (
	"context"
	"time"
)

// Store is the interface for message storage backends
type Store interface {
	// Message Operations
	WriteMessage(ctx context.Context, namespace, streamName string, msg *Message) (*WriteResult, error)
	GetStreamMessages(ctx context.Context, namespace, streamName string, opts *GetOpts) ([]*Message, error)
	GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *CategoryOpts) ([]*Message, error)
	GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*Message, error)
	GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error)

	// Namespace Operations
	CreateNamespace(ctx context.Context, id, tokenHash, description string) error
	DeleteNamespace(ctx context.Context, id string) error
	GetNamespace(ctx context.Context, id string) (*Namespace, error)
	ListNamespaces(ctx context.Context) ([]*Namespace, error)

	// Utility Functions (Message DB compatible)
	Category(streamName string) string
	ID(streamName string) string
	CardinalID(streamName string) string
	IsCategory(name string) bool
	Hash64(value string) int64

	// Lifecycle
	Close() error
}

// Message represents a message in the message store
type Message struct {
	ID             string                 // UUID v4 (RFC 4122)
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

// StandardMetadata represents standard metadata fields (Message DB compatible)
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
