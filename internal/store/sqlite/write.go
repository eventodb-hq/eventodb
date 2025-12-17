package sqlite

import (
	"context"
	"fmt"

	"github.com/message-db/message-db/internal/store"
)

// WriteMessage writes a message to a stream in the specified namespace
// Implementation will be completed in Phase MDB001_5A
func (s *SQLiteStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	return nil, fmt.Errorf("WriteMessage not yet implemented in Phase MDB001_4A")
}
