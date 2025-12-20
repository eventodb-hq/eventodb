package eventodb

import "time"

// Message represents a message to be written to a stream
type Message struct {
	Type     string                 `json:"type"`
	Data     map[string]interface{} `json:"data"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// WriteOptions configures message write operations
type WriteOptions struct {
	ID              *string `json:"id,omitempty"`
	ExpectedVersion *int64  `json:"expectedVersion,omitempty"`
}

// WriteResult contains the result of a write operation
type WriteResult struct {
	Position       int64 `json:"position"`
	GlobalPosition int64 `json:"globalPosition"`
}

// StreamMessage represents a message from stream.get
// Format: [id, type, position, globalPosition, data, metadata, time]
type StreamMessage struct {
	ID             string
	Type           string
	Position       int64
	GlobalPosition int64
	Data           map[string]interface{}
	Metadata       map[string]interface{}
	Time           time.Time
}

// CategoryMessage represents a message from category.get
// Format: [id, streamName, type, position, globalPosition, data, metadata, time]
type CategoryMessage struct {
	ID             string
	StreamName     string
	Type           string
	Position       int64
	GlobalPosition int64
	Data           map[string]interface{}
	Metadata       map[string]interface{}
	Time           time.Time
}

// GetStreamOptions configures stream read operations
type GetStreamOptions struct {
	Position       *int64 `json:"position,omitempty"`
	GlobalPosition *int64 `json:"globalPosition,omitempty"`
	BatchSize      *int   `json:"batchSize,omitempty"`
}

// GetCategoryOptions configures category read operations
type GetCategoryOptions struct {
	Position      *int64         `json:"position,omitempty"`
	BatchSize     *int           `json:"batchSize,omitempty"`
	Correlation   *string        `json:"correlation,omitempty"`
	ConsumerGroup *ConsumerGroup `json:"consumerGroup,omitempty"`
}

// ConsumerGroup for distributed consumption
type ConsumerGroup struct {
	Member int `json:"member"`
	Size   int `json:"size"`
}

// GetLastOptions configures stream.last operations
type GetLastOptions struct {
	Type *string `json:"type,omitempty"`
}

// CreateNamespaceOptions configures namespace creation
type CreateNamespaceOptions struct {
	Description *string `json:"description,omitempty"`
	Token       *string `json:"token,omitempty"`
}

// NamespaceResult contains result of namespace creation
type NamespaceResult struct {
	Namespace string    `json:"namespace"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
}

// DeleteNamespaceResult contains result of namespace deletion
type DeleteNamespaceResult struct {
	Namespace       string    `json:"namespace"`
	DeletedAt       time.Time `json:"deletedAt"`
	MessagesDeleted int64     `json:"messagesDeleted"`
}

// NamespaceInfo contains namespace metadata
type NamespaceInfo struct {
	Namespace    string    `json:"namespace"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
	MessageCount int64     `json:"messageCount"`
}

// HealthStatus contains server health info
type HealthStatus struct {
	Status string `json:"status"`
}
