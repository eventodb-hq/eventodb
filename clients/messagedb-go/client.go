// Package messagedb provides a Go client for MessageDB - a simple, fast message store.
//
// Basic usage:
//
//	client := messagedb.NewClient("http://localhost:8080",
//		messagedb.WithToken("ns_..."))
//
//	ctx := context.Background()
//
//	// Write message
//	result, err := client.StreamWrite(ctx, "account-123", messagedb.Message{
//		Type: "Deposited",
//		Data: map[string]interface{}{"amount": 100},
//	}, nil)
//
//	// Read stream
//	messages, err := client.StreamGet(ctx, "account-123", nil)
package messagedb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a MessageDB client
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new MessageDB client
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ClientOption configures a Client
type ClientOption func(*Client)

// WithToken sets the authentication token
func WithToken(token string) ClientOption {
	return func(c *Client) {
		c.token = token
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// rpc makes an RPC call to the server
func (c *Client) rpc(ctx context.Context, method string, args ...interface{}) (json.RawMessage, error) {
	// Build RPC request: [method, ...args]
	reqBody := []interface{}{method}
	reqBody = append(reqBody, args...)

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Capture token from response (for test mode auto-creation)
	if newToken := resp.Header.Get("X-MessageDB-Token"); newToken != "" && c.token == "" {
		c.token = newToken
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error *Error `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			return nil, errResp.Error
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetToken returns the current authentication token
func (c *Client) GetToken() string {
	return c.token
}

// Stream Operations

// StreamWrite writes a message to a stream
func (c *Client) StreamWrite(ctx context.Context, streamName string, message Message, opts *WriteOptions) (*WriteResult, error) {
	if opts == nil {
		opts = &WriteOptions{}
	}

	result, err := c.rpc(ctx, "stream.write", streamName, message, opts)
	if err != nil {
		return nil, err
	}

	var wr WriteResult
	if err := json.Unmarshal(result, &wr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &wr, nil
}

// StreamGet reads messages from a stream
func (c *Client) StreamGet(ctx context.Context, streamName string, opts *GetStreamOptions) ([]StreamMessage, error) {
	if opts == nil {
		opts = &GetStreamOptions{}
	}

	result, err := c.rpc(ctx, "stream.get", streamName, opts)
	if err != nil {
		return nil, err
	}

	// Parse array of arrays into StreamMessage structs
	var raw [][]interface{}
	if err := json.Unmarshal(result, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	messages := make([]StreamMessage, len(raw))
	for i, msg := range raw {
		if err := parseStreamMessage(&messages[i], msg); err != nil {
			return nil, fmt.Errorf("failed to parse message %d: %w", i, err)
		}
	}

	return messages, nil
}

// StreamLast gets the last message from a stream
func (c *Client) StreamLast(ctx context.Context, streamName string, opts *GetLastOptions) (*StreamMessage, error) {
	if opts == nil {
		opts = &GetLastOptions{}
	}

	result, err := c.rpc(ctx, "stream.last", streamName, opts)
	if err != nil {
		return nil, err
	}

	// Check for null or empty array
	if string(result) == "null" || string(result) == "[]" {
		return nil, nil
	}

	var raw []interface{}
	if err := json.Unmarshal(result, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	// Empty array means no message
	if len(raw) == 0 {
		return nil, nil
	}

	var msg StreamMessage
	if err := parseStreamMessage(&msg, raw); err != nil {
		return nil, err
	}

	return &msg, nil
}

// StreamVersion gets the current version of a stream
func (c *Client) StreamVersion(ctx context.Context, streamName string) (*int64, error) {
	result, err := c.rpc(ctx, "stream.version", streamName)
	if err != nil {
		return nil, err
	}

	// Check for null (must check before unmarshaling)
	resultStr := string(result)
	if resultStr == "null" || resultStr == "" {
		return nil, nil
	}

	var version int64
	if err := json.Unmarshal(result, &version); err != nil {
		return nil, fmt.Errorf("failed to unmarshal version: %w", err)
	}

	return &version, nil
}

// Category Operations

// CategoryGet reads messages from a category
func (c *Client) CategoryGet(ctx context.Context, categoryName string, opts *GetCategoryOptions) ([]CategoryMessage, error) {
	if opts == nil {
		opts = &GetCategoryOptions{}
	}

	result, err := c.rpc(ctx, "category.get", categoryName, opts)
	if err != nil {
		return nil, err
	}

	var raw [][]interface{}
	if err := json.Unmarshal(result, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	messages := make([]CategoryMessage, len(raw))
	for i, msg := range raw {
		if err := parseCategoryMessage(&messages[i], msg); err != nil {
			return nil, fmt.Errorf("failed to parse message %d: %w", i, err)
		}
	}

	return messages, nil
}

// Namespace Operations

// NamespaceCreate creates a new namespace
func (c *Client) NamespaceCreate(ctx context.Context, namespaceID string, opts *CreateNamespaceOptions) (*NamespaceResult, error) {
	if opts == nil {
		opts = &CreateNamespaceOptions{}
	}

	result, err := c.rpc(ctx, "ns.create", namespaceID, opts)
	if err != nil {
		return nil, err
	}

	var nr NamespaceResult
	if err := json.Unmarshal(result, &nr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	// Update client token if received
	if nr.Token != "" && c.token == "" {
		c.token = nr.Token
	}

	return &nr, nil
}

// NamespaceDelete deletes a namespace
func (c *Client) NamespaceDelete(ctx context.Context, namespaceID string) (*DeleteNamespaceResult, error) {
	result, err := c.rpc(ctx, "ns.delete", namespaceID)
	if err != nil {
		return nil, err
	}

	var dr DeleteNamespaceResult
	if err := json.Unmarshal(result, &dr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &dr, nil
}

// NamespaceList lists all namespaces
func (c *Client) NamespaceList(ctx context.Context) ([]NamespaceInfo, error) {
	result, err := c.rpc(ctx, "ns.list")
	if err != nil {
		return nil, err
	}

	var namespaces []NamespaceInfo
	if err := json.Unmarshal(result, &namespaces); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return namespaces, nil
}

// NamespaceInfo gets information about a namespace
func (c *Client) NamespaceInfo(ctx context.Context, namespaceID string) (*NamespaceInfo, error) {
	result, err := c.rpc(ctx, "ns.info", namespaceID)
	if err != nil {
		return nil, err
	}

	var info NamespaceInfo
	if err := json.Unmarshal(result, &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &info, nil
}

// System Operations

// SystemVersion gets the server version
func (c *Client) SystemVersion(ctx context.Context) (string, error) {
	result, err := c.rpc(ctx, "sys.version")
	if err != nil {
		return "", err
	}

	var version string
	if err := json.Unmarshal(result, &version); err != nil {
		return "", fmt.Errorf("failed to unmarshal version: %w", err)
	}

	return version, nil
}

// SystemHealth gets the server health status
func (c *Client) SystemHealth(ctx context.Context) (*HealthStatus, error) {
	result, err := c.rpc(ctx, "sys.health")
	if err != nil {
		return nil, err
	}

	var health HealthStatus
	if err := json.Unmarshal(result, &health); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health: %w", err)
	}

	return &health, nil
}

// Helper functions for parsing message arrays

func parseStreamMessage(msg *StreamMessage, raw []interface{}) error {
	if len(raw) != 7 {
		return fmt.Errorf("expected 7 fields, got %d", len(raw))
	}

	msg.ID = raw[0].(string)
	msg.Type = raw[1].(string)
	msg.Position = int64(raw[2].(float64))
	msg.GlobalPosition = int64(raw[3].(float64))
	msg.Data = raw[4].(map[string]interface{})

	if raw[5] != nil {
		msg.Metadata = raw[5].(map[string]interface{})
	}

	timeStr := raw[6].(string)
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return fmt.Errorf("failed to parse time: %w", err)
	}
	msg.Time = t

	return nil
}

func parseCategoryMessage(msg *CategoryMessage, raw []interface{}) error {
	if len(raw) != 8 {
		return fmt.Errorf("expected 8 fields, got %d", len(raw))
	}

	msg.ID = raw[0].(string)
	msg.StreamName = raw[1].(string)
	msg.Type = raw[2].(string)
	msg.Position = int64(raw[3].(float64))
	msg.GlobalPosition = int64(raw[4].(float64))
	msg.Data = raw[5].(map[string]interface{})

	if raw[6] != nil {
		msg.Metadata = raw[6].(map[string]interface{})
	}

	timeStr := raw[7].(string)
	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		return fmt.Errorf("failed to parse time: %w", err)
	}
	msg.Time = t

	return nil
}
