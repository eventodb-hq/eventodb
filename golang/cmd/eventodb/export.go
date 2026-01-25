// Package main provides the export CLI command.
package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ExportConfig holds configuration for the export command
type ExportConfig struct {
	URL        string
	Token      string
	Categories []string
	Since      *time.Time
	Until      *time.Time
	Gzip       bool
	Output     string
}

// ExportRecord represents the NDJSON format for export/import
type ExportRecord struct {
	ID       string                 `json:"id"`
	Stream   string                 `json:"stream"`
	Type     string                 `json:"type"`
	Position int64                  `json:"pos"`
	GPos     int64                  `json:"gpos"`
	Data     map[string]interface{} `json:"data"`
	Meta     map[string]interface{} `json:"meta"`
	Time     string                 `json:"time"`
}

// CategoryMessage represents a message from category.get RPC
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

func parseExportFlags(args []string) (*ExportConfig, error) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)

	url := fs.String("url", "", "EventoDB server URL (required)")
	token := fs.String("token", "", "Namespace token (required)")
	categories := fs.String("categories", "", "Comma-separated category list (optional, empty = all)")
	since := fs.String("since", "", "Start date (inclusive, RFC3339 or YYYY-MM-DD)")
	until := fs.String("until", "", "End date (exclusive, RFC3339 or YYYY-MM-DD)")
	useGzip := fs.Bool("gzip", false, "Compress output with gzip")
	output := fs.String("output", "", "Output file path (default: stdout)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `
Usage: eventodb export [OPTIONS]

Export events from EventoDB as NDJSON.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  eventodb export --url http://localhost:8080 --token $TOKEN --output backup.ndjson
  eventodb export --url http://localhost:8080 --token $TOKEN --categories user,order --since 2025-01-01
  eventodb export --url http://localhost:8080 --token $TOKEN --gzip --output backup.ndjson.gz
`)
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Validate required flags
	if *url == "" {
		return nil, fmt.Errorf("--url is required")
	}
	if *token == "" {
		return nil, fmt.Errorf("--token is required")
	}

	cfg := &ExportConfig{
		URL:    *url,
		Token:  *token,
		Gzip:   *useGzip,
		Output: *output,
	}

	// Parse categories
	if *categories != "" {
		cfg.Categories = strings.Split(*categories, ",")
		for i, c := range cfg.Categories {
			cfg.Categories[i] = strings.TrimSpace(c)
		}
	}

	// Parse since
	if *since != "" {
		t, err := parseDate(*since)
		if err != nil {
			return nil, fmt.Errorf("invalid --since: %w", err)
		}
		cfg.Since = &t
	}

	// Parse until
	if *until != "" {
		t, err := parseDate(*until)
		if err != nil {
			return nil, fmt.Errorf("invalid --until: %w", err)
		}
		cfg.Until = &t
	}

	return cfg, nil
}

func parseDate(s string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Try YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339 format")
}

func runExport(cfg *ExportConfig) error {
	ctx := context.Background()

	// Set up output writer
	var out io.Writer = os.Stdout
	var closeOutput func() error

	if cfg.Output != "" {
		f, err := os.Create(cfg.Output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		out = f
		closeOutput = f.Close
	}

	// Wrap with gzip if requested
	if cfg.Gzip {
		gzWriter := gzip.NewWriter(out)
		defer gzWriter.Close()
		out = gzWriter
	}

	encoder := json.NewEncoder(out)
	client := &http.Client{Timeout: 30 * time.Second}

	var exported int64

	// If no categories specified, use empty string to fetch all messages
	categories := cfg.Categories
	if len(categories) == 0 {
		categories = []string{""} // Empty = all messages
	}

	for _, category := range categories {
		var position int64

		for {
			messages, err := fetchCategoryBatch(ctx, client, cfg.URL, cfg.Token, category, position)
			if err != nil {
				return fmt.Errorf("failed to fetch messages: %w", err)
			}

			if len(messages) == 0 {
				break
			}

			for _, msg := range messages {
				// Apply time filtering
				if cfg.Since != nil && msg.Time.Before(*cfg.Since) {
					continue
				}
				if cfg.Until != nil && !msg.Time.Before(*cfg.Until) {
					continue
				}

				record := messageToRecord(&msg)
				if err := encoder.Encode(record); err != nil {
					return fmt.Errorf("failed to write record: %w", err)
				}
				exported++
			}

			// Update position for next batch
			position = messages[len(messages)-1].GlobalPosition + 1

			// Log progress to stderr
			fmt.Fprintf(os.Stderr, "\rExported: %d events...", exported)
		}
	}

	fmt.Fprintf(os.Stderr, "\rExported: %d events total\n", exported)

	// Close output file if needed
	if closeOutput != nil {
		if err := closeOutput(); err != nil {
			return fmt.Errorf("failed to close output file: %w", err)
		}
	}

	return nil
}

func fetchCategoryBatch(ctx context.Context, client *http.Client, baseURL, token, category string, position int64) ([]CategoryMessage, error) {
	// Build RPC request: ["category.get", category, {position: X, batchSize: 1000}]
	opts := map[string]interface{}{
		"position":  position,
		"batchSize": 1000,
	}
	reqBody := []interface{}{"category.get", category, opts}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/rpc", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response: array of arrays
	// Format: [[id, streamName, type, position, globalPosition, data, metadata, time], ...]
	var raw [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	messages := make([]CategoryMessage, len(raw))
	for i, msg := range raw {
		if err := parseCategoryMsg(&messages[i], msg); err != nil {
			return nil, fmt.Errorf("failed to parse message %d: %w", i, err)
		}
	}

	return messages, nil
}

func parseCategoryMsg(msg *CategoryMessage, raw []interface{}) error {
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

func messageToRecord(msg *CategoryMessage) *ExportRecord {
	return &ExportRecord{
		ID:       msg.ID,
		Stream:   msg.StreamName,
		Type:     msg.Type,
		Position: msg.Position,
		GPos:     msg.GlobalPosition,
		Data:     msg.Data,
		Meta:     msg.Metadata,
		Time:     msg.Time.UTC().Format(time.RFC3339),
	}
}
