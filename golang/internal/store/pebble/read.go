package pebble

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/eventodb/eventodb/internal/store"
)

// GetStreamMessages retrieves messages from a specific stream
func (s *PebbleStore) GetStreamMessages(ctx context.Context, namespace, streamName string, opts *store.GetOpts) ([]*store.Message, error) {
	// Validate stream name
	if streamName == "" {
		return nil, fmt.Errorf("stream name cannot be empty")
	}

	// Get namespace handle
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Set defaults
	batchSize := int64(1000)
	if opts != nil && opts.BatchSize > 0 {
		batchSize = opts.BatchSize
	} else if opts != nil && opts.BatchSize == -1 {
		batchSize = -1 // unlimited
	}

	// Use GlobalPosition filter if specified, otherwise use Position
	var messages []*store.Message
	if opts != nil && opts.GlobalPosition != nil {
		// Filter by global position - need to iterate through stream and filter
		position := int64(0)
		if opts.Position > 0 {
			position = opts.Position
		}

		startKey := formatStreamIndexKey(streamName, position)
		endKey := formatStreamIndexKey(streamName, 999999999999999999)

		iter, err := handle.db.NewIter(&pebble.IterOptions{
			LowerBound: startKey,
			UpperBound: endKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create iterator: %w", err)
		}
		defer iter.Close()

		capacity := batchSize
		if capacity <= 0 {
			capacity = 1000
		}
		messages = make([]*store.Message, 0, capacity)

		for iter.First(); iter.Valid(); iter.Next() {
			gpBytes := iter.Value()
			gp, err := decodeInt64(gpBytes)
			if err != nil {
				return nil, fmt.Errorf("failed to decode global position: %w", err)
			}

			// Skip messages before the global position filter
			if gp < *opts.GlobalPosition {
				continue
			}

			// Check batch size limit
			if batchSize != -1 && int64(len(messages)) >= batchSize {
				break
			}

			msgKey := formatMessageKey(gp)
			compressedData, closer, err := handle.db.Get(msgKey)
			if err != nil {
				return nil, fmt.Errorf("failed to get message at gp=%d: %w", gp, err)
			}

			msgData, err := decompressJSON(compressedData)
			closer.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to decompress message: %w", err)
			}

			var msg store.Message
			if err := json.Unmarshal(msgData, &msg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal message: %w", err)
			}

			messages = append(messages, &msg)
		}

		if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("iterator error: %w", err)
		}
	} else {
		// Filter by position (default)
		position := int64(0)
		if opts != nil && opts.Position > 0 {
			position = opts.Position
		}

		startKey := formatStreamIndexKey(streamName, position)
		endKey := formatStreamIndexKey(streamName, 999999999999999999)

		iter, err := handle.db.NewIter(&pebble.IterOptions{
			LowerBound: startKey,
			UpperBound: endKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create iterator: %w", err)
		}
		defer iter.Close()

		capacity := batchSize
		if capacity <= 0 {
			capacity = 1000
		}
		messages = make([]*store.Message, 0, capacity)

		for iter.First(); iter.Valid(); iter.Next() {
			// Check batch size limit
			if batchSize != -1 && int64(len(messages)) >= batchSize {
				break
			}

			gpBytes := iter.Value()
			gp, err := decodeInt64(gpBytes)
			if err != nil {
				return nil, fmt.Errorf("failed to decode global position: %w", err)
			}

			msgKey := formatMessageKey(gp)
			compressedData, closer, err := handle.db.Get(msgKey)
			if err != nil {
				return nil, fmt.Errorf("failed to get message at gp=%d: %w", gp, err)
			}

			msgData, err := decompressJSON(compressedData)
			closer.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to decompress message: %w", err)
			}

			var msg store.Message
			if err := json.Unmarshal(msgData, &msg); err != nil {
				return nil, fmt.Errorf("failed to unmarshal message: %w", err)
			}

			messages = append(messages, &msg)
		}

		if err := iter.Error(); err != nil {
			return nil, fmt.Errorf("iterator error: %w", err)
		}
	}

	return messages, nil
}

// GetCategoryMessages retrieves messages from all streams in a category
// If categoryName is empty, returns all messages ordered by global position
func (s *PebbleStore) GetCategoryMessages(ctx context.Context, namespace, categoryName string, opts *store.CategoryOpts) ([]*store.Message, error) {
	// Get namespace handle
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Set defaults
	globalPosition := int64(1)
	if opts != nil && opts.Position > 0 {
		globalPosition = opts.Position
	}
	batchSize := int64(1000)
	if opts != nil && opts.BatchSize > 0 {
		batchSize = opts.BatchSize
	} else if opts != nil && opts.BatchSize == -1 {
		batchSize = -1 // unlimited
	}

	// Empty category = all messages by global position
	if categoryName == "" {
		return s.getAllMessages(ctx, handle, globalPosition, batchSize, opts)
	}

	// Create range scan iterator over category index
	// Start: CI:{category}:{globalPosition_20}
	// End: CI:{category}:{max_int64}
	startKey := formatCategoryIndexKey(categoryName, globalPosition)
	endKey := formatCategoryIndexKey(categoryName, 999999999999999999) // Max 18-digit number

	iter, err := handle.db.NewIter(&pebble.IterOptions{
		LowerBound: startKey,
		UpperBound: endKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	// Parse consumer group options
	var consumerMember *int64
	var consumerSize *int64
	if opts != nil {
		consumerMember = opts.ConsumerMember
		consumerSize = opts.ConsumerSize
	}
	hasConsumerGroup := consumerMember != nil && consumerSize != nil && *consumerSize > 0

	// Parse correlation filter
	var correlationCategory *string
	if opts != nil && opts.Correlation != nil && *opts.Correlation != "" {
		// Extract category from correlation stream name
		cat := extractCategory(*opts.Correlation)
		correlationCategory = &cat
	}

	// Collect messages
	messages := make([]*store.Message, 0, batchSize)
	scannedCount := int64(0)
	maxScan := batchSize
	if hasConsumerGroup && batchSize != -1 {
		// Read more keys for consumer group filtering
		maxScan = batchSize * (*consumerSize)
	}

	for iter.First(); iter.Valid(); iter.Next() {
		// Check if we've scanned enough keys
		if maxScan != -1 && scannedCount >= maxScan {
			break
		}
		scannedCount++

		// Extract global position from key
		// Key format: CI:{category}:{gp_20}
		keyBytes := iter.Key()
		gp, err := extractGlobalPositionFromCategoryKey(keyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to extract global position from key: %w", err)
		}

		// Extract stream name from value
		streamName := string(iter.Value())

		// Apply consumer group filter if specified
		if hasConsumerGroup {
			if !store.IsAssignedToConsumerMember(streamName, *consumerMember, *consumerSize) {
				continue // Skip this message
			}
		}

		// Point lookup message: M:{gp}
		msgKey := formatMessageKey(gp)
		compressedData, closer, err := handle.db.Get(msgKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get message at gp=%d: %w", gp, err)
		}

		// Decompress S2-compressed data
		msgData, err := decompressJSON(compressedData)
		closer.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decompress message: %w", err)
		}

		// Deserialize message using jsoniter
		var msg store.Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}

		// Apply correlation filter if specified
		if correlationCategory != nil {
			if msg.Metadata == nil {
				continue // Skip messages without metadata
			}
			corrVal, ok := msg.Metadata["correlationStreamName"]
			if !ok {
				continue // Skip messages without correlation
			}
			corr, ok := corrVal.(string)
			if !ok {
				continue // Skip messages with non-string correlation
			}
			corrCat := extractCategory(corr)
			if corrCat != *correlationCategory {
				continue // Correlation doesn't match
			}
		}

		messages = append(messages, &msg)

		// Check if we've collected enough messages
		if batchSize != -1 && int64(len(messages)) >= batchSize {
			break
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	return messages, nil
}

// getAllMessages retrieves all messages ordered by global position
func (s *PebbleStore) getAllMessages(ctx context.Context, handle *namespaceHandle, startPosition, batchSize int64, opts *store.CategoryOpts) ([]*store.Message, error) {
	// Iterate over M: prefix (all messages by global position)
	startKey := formatMessageKey(startPosition)
	endKey := formatMessageKey(999999999999999999) // Max 18-digit number

	iter, err := handle.db.NewIter(&pebble.IterOptions{
		LowerBound: startKey,
		UpperBound: endKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	// Parse correlation filter
	var correlationCategory *string
	if opts != nil && opts.Correlation != nil && *opts.Correlation != "" {
		cat := extractCategory(*opts.Correlation)
		correlationCategory = &cat
	}

	messages := make([]*store.Message, 0, batchSize)

	for iter.First(); iter.Valid(); iter.Next() {
		// Decompress and deserialize message
		compressedData := iter.Value()
		msgData, err := decompressJSON(compressedData)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress message: %w", err)
		}

		var msg store.Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}

		// Apply correlation filter if specified
		if correlationCategory != nil {
			if msg.Metadata == nil {
				continue
			}
			corrVal, ok := msg.Metadata["correlationStreamName"]
			if !ok {
				continue
			}
			corr, ok := corrVal.(string)
			if !ok {
				continue
			}
			corrCat := extractCategory(corr)
			if corrCat != *correlationCategory {
				continue
			}
		}

		messages = append(messages, &msg)

		if batchSize != -1 && int64(len(messages)) >= batchSize {
			break
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	return messages, nil
}

// GetLastStreamMessage retrieves the last message from a stream
func (s *PebbleStore) GetLastStreamMessage(ctx context.Context, namespace, streamName string, msgType *string) (*store.Message, error) {
	// Validate stream name
	if streamName == "" {
		return nil, fmt.Errorf("stream name cannot be empty")
	}

	// Get namespace handle
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Get stream version (last position)
	versionKey := formatVersionIndexKey(streamName)
	versionData, closer, err := handle.db.Get(versionKey)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, store.ErrStreamNotFound
		}
		return nil, fmt.Errorf("failed to get stream version: %w", err)
	}

	lastPosition, err := decodeInt64(versionData)
	closer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to decode stream version: %w", err)
	}

	// If no type filter, get message at last position directly
	if msgType == nil {
		// Get global position from stream index
		siKey := formatStreamIndexKey(streamName, lastPosition)
		gpData, closer, err := handle.db.Get(siKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get global position: %w", err)
		}

		gp, err := decodeInt64(gpData)
		closer.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode global position: %w", err)
		}

		// Get message
		msgKey := formatMessageKey(gp)
		compressedData, closer, err := handle.db.Get(msgKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get message: %w", err)
		}

		// Decompress S2-compressed data
		msgData, err := decompressJSON(compressedData)
		closer.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decompress message: %w", err)
		}

		var msg store.Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}

		return &msg, nil
	}

	// With type filter: iterate backwards through stream index
	startKey := formatStreamIndexKey(streamName, 0)
	endKey := formatStreamIndexKey(streamName, 999999999999999999) // Max 18-digit number

	iter, err := handle.db.NewIter(&pebble.IterOptions{
		LowerBound: startKey,
		UpperBound: endKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	// Iterate backwards from last position
	for valid := iter.Last(); valid; valid = iter.Prev() {
		// Extract global position from value
		gpData := iter.Value()
		gp, err := decodeInt64(gpData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode global position: %w", err)
		}

		// Get message
		msgKey := formatMessageKey(gp)
		compressedData, closer, err := handle.db.Get(msgKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get message at gp=%d: %w", gp, err)
		}

		// Decompress S2-compressed data
		msgData, err := decompressJSON(compressedData)
		closer.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decompress message: %w", err)
		}

		var msg store.Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}

		// Check if type matches
		if msg.Type == *msgType {
			return &msg, nil
		}
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	// No message found with specified type
	return nil, store.ErrStreamNotFound
}

// GetStreamVersion returns the current version (position of last message) of a stream
func (s *PebbleStore) GetStreamVersion(ctx context.Context, namespace, streamName string) (int64, error) {
	// Validate stream name
	if streamName == "" {
		return -1, fmt.Errorf("stream name cannot be empty")
	}

	// Get namespace handle
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return -1, err
	}

	// Get stream version
	versionKey := formatVersionIndexKey(streamName)
	versionData, closer, err := handle.db.Get(versionKey)
	if err != nil {
		if err == pebble.ErrNotFound {
			return -1, nil // Stream doesn't exist
		}
		return -1, fmt.Errorf("failed to get stream version: %w", err)
	}
	defer closer.Close()

	version, err := decodeInt64(versionData)
	if err != nil {
		return -1, fmt.Errorf("failed to decode stream version: %w", err)
	}

	return version, nil
}

// ListStreams returns streams in a namespace with optional prefix filtering and pagination.
// It iterates the version index (VI:{stream}) for stream names, then looks up
// the last message for version and lastActivity.
func (s *PebbleStore) ListStreams(ctx context.Context, namespace string, opts *store.ListStreamsOpts) ([]*store.StreamInfo, error) {
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return nil, err
	}

	if opts == nil {
		opts = &store.ListStreamsOpts{Limit: 100}
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Iterate VI: prefix to find all streams
	viStart := []byte(prefixVersionIndex)
	viEnd := []byte(prefixVersionIndex + "\xff")

	iter, err := handle.db.NewIter(&pebble.IterOptions{
		LowerBound: viStart,
		UpperBound: viEnd,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create version index iterator: %w", err)
	}
	defer iter.Close()

	results := make([]*store.StreamInfo, 0, limit)

	for iter.First(); iter.Valid(); iter.Next() {
		if int64(len(results)) >= limit {
			break
		}

		// Extract stream name: key = VI:{stream}
		key := string(iter.Key())
		streamName := strings.TrimPrefix(key, prefixVersionIndex)

		// Apply prefix filter
		if opts.Prefix != "" && !strings.HasPrefix(streamName, opts.Prefix) {
			continue
		}

		// Apply cursor (exclusive)
		if opts.Cursor != "" && streamName <= opts.Cursor {
			continue
		}

		// Get version from VI value
		version, err := decodeInt64(iter.Value())
		if err != nil {
			return nil, fmt.Errorf("failed to decode version for stream %s: %w", streamName, err)
		}

		// Look up last message to get lastActivity: SI:{stream}:{version}
		siKey := formatStreamIndexKey(streamName, version)
		gpData, closer, err := handle.db.Get(siKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get stream index for %s: %w", streamName, err)
		}
		gp, err := decodeInt64(gpData)
		closer.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decode global position for stream %s: %w", streamName, err)
		}

		// Get message to find time
		msgKey := formatMessageKey(gp)
		compressedData, closer, err := handle.db.Get(msgKey)
		if err != nil {
			return nil, fmt.Errorf("failed to get message at gp=%d for stream %s: %w", gp, streamName, err)
		}
		msgData, err := decompressJSON(compressedData)
		closer.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decompress message: %w", err)
		}

		var msg store.Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message: %w", err)
		}

		results = append(results, &store.StreamInfo{
			StreamName:   streamName,
			Version:      version,
			LastActivity: msg.Time.UTC(),
		})
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("iterator error: %w", err)
	}

	return results, nil
}

// ListCategories returns distinct categories in a namespace with stream and message counts.
// It iterates the version index (VI:{stream}) to enumerate streams, derives categories,
// then counts messages via the category index (CI:{category}:*).
func (s *PebbleStore) ListCategories(ctx context.Context, namespace string) ([]*store.CategoryInfo, error) {
	handle, err := s.getNamespaceDB(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// First pass: collect distinct streams grouped by category from the version index
	type catAccum struct {
		streams map[string]struct{}
	}
	catMap := map[string]*catAccum{}

	viStart := []byte(prefixVersionIndex)
	viEnd := []byte(prefixVersionIndex + "\xff")

	iter, err := handle.db.NewIter(&pebble.IterOptions{
		LowerBound: viStart,
		UpperBound: viEnd,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create version index iterator: %w", err)
	}

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		streamName := strings.TrimPrefix(key, prefixVersionIndex)
		cat := extractCategory(streamName)
		if _, ok := catMap[cat]; !ok {
			catMap[cat] = &catAccum{streams: map[string]struct{}{}}
		}
		catMap[cat].streams[streamName] = struct{}{}
	}
	iterErr := iter.Error()
	iter.Close()
	if iterErr != nil {
		return nil, fmt.Errorf("iterator error: %w", iterErr)
	}

	if len(catMap) == 0 {
		return []*store.CategoryInfo{}, nil
	}

	// Second pass: count messages per category using the category index (CI:{cat}:*)
	results := make([]*store.CategoryInfo, 0, len(catMap))
	for cat, accum := range catMap {
		ciStart := formatCategoryIndexKey(cat, 0)
		// End key: CI:{cat}0 (0x30 is '0', which is the first digit; use '\xff' suffix)
		ciEnd := []byte(fmt.Sprintf("%s%s\xff", prefixCategoryIndex, cat+":"))

		ciIter, err := handle.db.NewIter(&pebble.IterOptions{
			LowerBound: ciStart,
			UpperBound: ciEnd,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create category index iterator: %w", err)
		}

		var msgCount int64
		for ciIter.First(); ciIter.Valid(); ciIter.Next() {
			msgCount++
		}
		ciIterErr := ciIter.Error()
		ciIter.Close()
		if ciIterErr != nil {
			return nil, fmt.Errorf("category iterator error: %w", ciIterErr)
		}

		results = append(results, &store.CategoryInfo{
			Category:     cat,
			StreamCount:  int64(len(accum.streams)),
			MessageCount: msgCount,
		})
	}

	// Sort lexicographically by category
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Category > results[j].Category {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// extractGlobalPositionFromCategoryKey extracts global position from category index key
// Key format: CI:{category}:{gp_20}
func extractGlobalPositionFromCategoryKey(key []byte) (int64, error) {
	// Skip "CI:" prefix
	if len(key) < 3 || key[0] != 'C' || key[1] != 'I' || key[2] != ':' {
		return 0, fmt.Errorf("invalid category key format")
	}

	// Find last ':' separator
	lastColon := -1
	for i := len(key) - 1; i >= 3; i-- {
		if key[i] == ':' {
			lastColon = i
			break
		}
	}

	if lastColon == -1 || lastColon+1 >= len(key) {
		return 0, fmt.Errorf("invalid category key format: no global position")
	}

	// Extract and decode global position
	gpBytes := key[lastColon+1:]
	return decodeInt64(gpBytes)
}
