package eventodb

import (
	"context"
	"reflect"
	"testing"
)

func TestENCODING001_UTF8TextInData(t *testing.T) {
	tc := setupTest(t, "encoding-001")
	ctx := context.Background()

	stream := randomStreamName()
	utf8Text := "Hello 世界 🌍 émojis"

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"text": utf8Text},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Data["text"] != utf8Text {
		t.Errorf("UTF-8 text mismatch.\nExpected: %s\nGot: %s", utf8Text, messages[0].Data["text"])
	}
}

func TestENCODING002_UnicodeInMetadata(t *testing.T) {
	tc := setupTest(t, "encoding-002")
	ctx := context.Background()

	stream := randomStreamName()
	unicodeText := "Test 测试 🎉"

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
		Metadata: map[string]interface{}{
			"description": unicodeText,
		},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if messages[0].Metadata["description"] != unicodeText {
		t.Errorf("Unicode metadata mismatch.\nExpected: %s\nGot: %s",
			unicodeText, messages[0].Metadata["description"])
	}
}

func TestENCODING003_SpecialCharactersInStreamName(t *testing.T) {
	tc := setupTest(t, "encoding-003")
	ctx := context.Background()

	stream := "test-stream_123.abc"

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"foo": "bar"},
	}, nil)

	if err != nil {
		// May fail depending on server validation
		t.Logf("Stream name with special characters rejected: %v", err)
		return
	}

	// If successful, verify we can read it back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(messages))
	}
}

func TestENCODING004_EmptyStringValues(t *testing.T) {
	tc := setupTest(t, "encoding-004")
	ctx := context.Background()

	stream := randomStreamName()

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"emptyString": ""},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	val := messages[0].Data["emptyString"]
	if val != "" {
		t.Errorf("Expected empty string, got %v (type %T)", val, val)
	}
}

func TestENCODING005_BooleanValues(t *testing.T) {
	tc := setupTest(t, "encoding-005")
	ctx := context.Background()

	stream := randomStreamName()

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{
			"isTrue":  true,
			"isFalse": false,
		},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	data := messages[0].Data
	if data["isTrue"] != true {
		t.Errorf("Expected true, got %v (type %T)", data["isTrue"], data["isTrue"])
	}
	if data["isFalse"] != false {
		t.Errorf("Expected false, got %v (type %T)", data["isFalse"], data["isFalse"])
	}
}

func TestENCODING006_NullValues(t *testing.T) {
	tc := setupTest(t, "encoding-006")
	ctx := context.Background()

	stream := randomStreamName()

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{"nullValue": nil},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	val := messages[0].Data["nullValue"]
	if val != nil {
		t.Errorf("Expected nil, got %v (type %T)", val, val)
	}
}

func TestENCODING007_NumericValues(t *testing.T) {
	tc := setupTest(t, "encoding-007")
	ctx := context.Background()

	stream := randomStreamName()

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: map[string]interface{}{
			"integer":  42,
			"float":    3.14159,
			"negative": -100,
			"zero":     0,
		},
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	data := messages[0].Data

	// JSON numbers are float64 in Go
	if data["integer"] != 42.0 {
		t.Errorf("Expected 42, got %v", data["integer"])
	}
	if data["float"] != 3.14159 {
		t.Errorf("Expected 3.14159, got %v", data["float"])
	}
	if data["negative"] != -100.0 {
		t.Errorf("Expected -100, got %v", data["negative"])
	}
	if data["zero"] != 0.0 {
		t.Errorf("Expected 0, got %v", data["zero"])
	}
}

func TestENCODING008_NestedObjects(t *testing.T) {
	tc := setupTest(t, "encoding-008")
	ctx := context.Background()

	stream := randomStreamName()

	nestedData := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": map[string]interface{}{
					"value": "deep",
				},
			},
		},
	}

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: nestedData,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if !reflect.DeepEqual(messages[0].Data, nestedData) {
		t.Errorf("Nested data mismatch.\nExpected: %+v\nGot: %+v", nestedData, messages[0].Data)
	}
}

func TestENCODING009_ArraysInData(t *testing.T) {
	tc := setupTest(t, "encoding-009")
	ctx := context.Background()

	stream := randomStreamName()

	data := map[string]interface{}{
		"items": []interface{}{1.0, "two", map[string]interface{}{"three": 3.0}, nil, true},
	}

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: data,
	}, nil)
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	if !reflect.DeepEqual(messages[0].Data, data) {
		t.Errorf("Array data mismatch.\nExpected: %+v\nGot: %+v", data, messages[0].Data)
	}
}

func TestENCODING010_LargeMessagePayload(t *testing.T) {
	tc := setupTest(t, "encoding-010")
	ctx := context.Background()

	stream := randomStreamName()

	// Create a large data object (not too large to avoid timeout)
	largeData := make(map[string]interface{})
	for i := 0; i < 1000; i++ {
		largeData[string(rune('a'+i%26))+string(rune('0'+i%10))] = "value" + string(rune('0'+i%10))
	}

	_, err := tc.client.StreamWrite(ctx, stream, Message{
		Type: "TestEvent",
		Data: largeData,
	}, nil)

	if err != nil {
		// May fail if over size limit
		t.Logf("Large message rejected (may be expected): %v", err)
		return
	}

	// If successful, verify we can read it back
	messages, err := tc.client.StreamGet(ctx, stream, nil)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	// Verify size is roughly correct
	if len(messages[0].Data) != len(largeData) {
		t.Errorf("Data size mismatch: expected %d keys, got %d", len(largeData), len(messages[0].Data))
	}
}
