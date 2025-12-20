package timescale

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/eventodb/eventodb/internal/store"
)

// WriteMessage writes a message to a stream with optimistic locking support
func (s *TimescaleStore) WriteMessage(ctx context.Context, namespace, streamName string, msg *store.Message) (*store.WriteResult, error) {
	// 1. Get schema name for namespace
	schemaName, err := s.getSchemaName(namespace)
	if err != nil {
		return nil, err
	}

	// 2. Generate UUID if not provided
	if msg.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("failed to generate UUID: %w", err)
		}
		msg.ID = id.String()
	}

	// 3. Validate UUID format
	if _, err := uuid.Parse(msg.ID); err != nil {
		return nil, fmt.Errorf("invalid UUID format: %w", err)
	}

	// 4. Serialize data and metadata to JSONB
	var dataParam interface{} = nil
	var metadataParam interface{} = nil

	if msg.Data != nil {
		dataJSON, err := json.Marshal(msg.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
		dataParam = string(dataJSON)
	}

	if msg.Metadata != nil {
		metadataJSON, err := json.Marshal(msg.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataParam = string(metadataJSON)
	}

	// 5. Call write_message stored procedure
	query := fmt.Sprintf(
		`SELECT "%s".write_message($1, $2, $3, $4::jsonb, $5::jsonb, $6)`,
		schemaName,
	)

	var position int64
	err = s.db.QueryRowContext(
		ctx,
		query,
		msg.ID,
		streamName,
		msg.Type,
		dataParam,
		metadataParam,
		msg.ExpectedVersion,
	).Scan(&position)

	if err != nil {
		// Check for version conflict error
		// pgx doesn't prefix errors with "pq:", so check for the actual error message
		if strings.Contains(err.Error(), "Wrong expected version") {
			return nil, store.ErrVersionConflict
		}
		return nil, fmt.Errorf("failed to write message: %w", err)
	}

	// 6. Query for global_position
	globalQuery := fmt.Sprintf(
		`SELECT global_position FROM "%s".messages WHERE stream_name = $1 AND position = $2`,
		schemaName,
	)

	var globalPosition int64
	err = s.db.QueryRowContext(ctx, globalQuery, streamName, position).Scan(&globalPosition)
	if err != nil {
		return nil, fmt.Errorf("failed to get global position: %w", err)
	}

	return &store.WriteResult{
		Position:       position,
		GlobalPosition: globalPosition,
	}, nil
}
