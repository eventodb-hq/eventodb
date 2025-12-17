package sqlite

import (
	"context"
	"fmt"

	"github.com/message-db/message-db/internal/store"
)

// GetStreamMessages retrieves messages from a stream
// Implementation will be completed in Phase MDB001_5A
func (s *SQLiteStore) GetStreamMessages(ctx context.Context, namespace, streamName string, opts *store.GetOpts) ([]*store.Message, error) {
	return nil, fmt.Errorf("GetStreamMessages not yet implemented in Phase MDB001_4A")
}

// GetCategoryMessages retrieves messages from a category
// Implementation will be completed in Phase MDB001_5A
func (s *SQLiteStore) GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *store.CategoryOpts) ([]*store.Message, error) {
	return nil, fmt.Errorf("GetCategoryMessages not yet implemented in Phase MDB001_4A")
}

// GetLastStreamMessage retrieves the last message from a stream
// Implementation will be completed in Phase MDB001_5A
func (s *SQLiteStore) GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*store.Message, error) {
	return nil, fmt.Errorf("GetLastStreamMessage not yet implemented in Phase MDB001_4A")
}

// GetStreamVersion retrieves the current version of a stream
// Implementation will be completed in Phase MDB001_5A
func (s *SQLiteStore) GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error) {
	return -1, fmt.Errorf("GetStreamVersion not yet implemented in Phase MDB001_4A")
}
