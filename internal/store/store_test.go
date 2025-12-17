package store

import (
	"encoding/json"
	"testing"
	"time"
)

// MDB001_1A_T6: Test Message struct creation and JSON serialization
func TestMDB001_1A_T6_MessageStructAndJSONSerialization(t *testing.T) {
	msg := &Message{
		ID:             "550e8400-e29b-41d4-a716-446655440000",
		StreamName:     "account-123",
		Type:           "AccountCreated",
		Position:       5,
		GlobalPosition: 100,
		Data: map[string]interface{}{
			"accountId": "123",
			"name":      "Test Account",
		},
		Metadata: map[string]interface{}{
			"correlationStreamName": "command-456",
			"userId":                "user-1",
		},
		Time: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	// Test JSON serialization of Data
	dataJSON, err := json.Marshal(msg.Data)
	if err != nil {
		t.Fatalf("failed to marshal Data: %v", err)
	}

	var dataBack map[string]interface{}
	if err := json.Unmarshal(dataJSON, &dataBack); err != nil {
		t.Fatalf("failed to unmarshal Data: %v", err)
	}

	if dataBack["accountId"] != "123" {
		t.Errorf("expected accountId '123', got %v", dataBack["accountId"])
	}

	// Test JSON serialization of Metadata
	metaJSON, err := json.Marshal(msg.Metadata)
	if err != nil {
		t.Fatalf("failed to marshal Metadata: %v", err)
	}

	var metaBack map[string]interface{}
	if err := json.Unmarshal(metaJSON, &metaBack); err != nil {
		t.Fatalf("failed to unmarshal Metadata: %v", err)
	}

	if metaBack["correlationStreamName"] != "command-456" {
		t.Errorf("expected correlationStreamName 'command-456', got %v", metaBack["correlationStreamName"])
	}
}

// MDB001_1A_T7: Test WriteResult struct
func TestMDB001_1A_T7_WriteResultStruct(t *testing.T) {
	result := &WriteResult{
		Position:       10,
		GlobalPosition: 500,
	}

	if result.Position != 10 {
		t.Errorf("expected Position 10, got %d", result.Position)
	}

	if result.GlobalPosition != 500 {
		t.Errorf("expected GlobalPosition 500, got %d", result.GlobalPosition)
	}
}

// MDB001_1A_T8: Test GetOpts validation
func TestMDB001_1A_T8_GetOptsValidation(t *testing.T) {
	// Test default values
	opts := NewGetOpts()
	if opts.Position != 0 {
		t.Errorf("expected default Position 0, got %d", opts.Position)
	}
	if opts.BatchSize != 1000 {
		t.Errorf("expected default BatchSize 1000, got %d", opts.BatchSize)
	}

	// Test custom values
	globalPos := int64(100)
	opts = &GetOpts{
		Position:       50,
		GlobalPosition: &globalPos,
		BatchSize:      100,
	}

	if opts.Position != 50 {
		t.Errorf("expected Position 50, got %d", opts.Position)
	}
	if opts.GlobalPosition == nil || *opts.GlobalPosition != 100 {
		t.Errorf("expected GlobalPosition 100, got %v", opts.GlobalPosition)
	}
	if opts.BatchSize != 100 {
		t.Errorf("expected BatchSize 100, got %d", opts.BatchSize)
	}

	// Test unlimited batch size
	opts.BatchSize = -1
	if opts.BatchSize != -1 {
		t.Errorf("expected BatchSize -1 for unlimited, got %d", opts.BatchSize)
	}
}

// MDB001_1A_T9: Test CategoryOpts validation
func TestMDB001_1A_T9_CategoryOptsValidation(t *testing.T) {
	// Test default values
	opts := NewCategoryOpts()
	if opts.Position != 1 {
		t.Errorf("expected default Position 1, got %d", opts.Position)
	}
	if opts.BatchSize != 1000 {
		t.Errorf("expected default BatchSize 1000, got %d", opts.BatchSize)
	}

	// Test consumer group options
	member := int64(0)
	size := int64(4)
	correlation := "account"
	opts = &CategoryOpts{
		Position:       100,
		BatchSize:      50,
		Correlation:    &correlation,
		ConsumerMember: &member,
		ConsumerSize:   &size,
	}

	if opts.Position != 100 {
		t.Errorf("expected Position 100, got %d", opts.Position)
	}
	if opts.ConsumerMember == nil || *opts.ConsumerMember != 0 {
		t.Errorf("expected ConsumerMember 0, got %v", opts.ConsumerMember)
	}
	if opts.ConsumerSize == nil || *opts.ConsumerSize != 4 {
		t.Errorf("expected ConsumerSize 4, got %v", opts.ConsumerSize)
	}
	if opts.Correlation == nil || *opts.Correlation != "account" {
		t.Errorf("expected Correlation 'account', got %v", opts.Correlation)
	}
}
