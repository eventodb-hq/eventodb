// Package main provides the import CLI command.
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ImportConfig holds configuration for the import command
type ImportConfig struct {
	URL   string
	Token string
	Gzip  bool
	Input string
}

// ImportProgressEvent represents progress from the server
type ImportProgressEvent struct {
	Imported int64  `json:"imported"`
	GPos     int64  `json:"gpos"`
	Done     bool   `json:"done"`
	Elapsed  string `json:"elapsed"`
	Error    string `json:"error"`
	Message  string `json:"message"`
	Line     int64  `json:"line"`
}

func parseImportFlags(args []string) (*ImportConfig, error) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)

	url := fs.String("url", "", "EventoDB server URL (required)")
	token := fs.String("token", "", "Namespace token (required)")
	useGzip := fs.Bool("gzip", false, "Decompress input with gzip")
	input := fs.String("input", "", "Input file path (default: stdin)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `
Usage: eventodb import [OPTIONS]

Import events from NDJSON file to EventoDB.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  eventodb import --url http://localhost:8080 --token $TOKEN --input backup.ndjson
  eventodb import --url http://localhost:8080 --token $TOKEN --gzip --input backup.ndjson.gz
  cat backup.ndjson | eventodb import --url http://localhost:8080 --token $TOKEN
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

	return &ImportConfig{
		URL:   *url,
		Token: *token,
		Gzip:  *useGzip,
		Input: *input,
	}, nil
}

func runImport(cfg *ImportConfig) error {
	// Set up input reader
	var inputReader io.Reader = os.Stdin
	var closeInput func() error

	if cfg.Input != "" {
		f, err := os.Open(cfg.Input)
		if err != nil {
			return fmt.Errorf("failed to open input file: %w", err)
		}
		inputReader = f
		closeInput = f.Close
	}

	// Wrap with gzip decompression if requested
	if cfg.Gzip {
		gzReader, err := gzip.NewReader(inputReader)
		if err != nil {
			if closeInput != nil {
				closeInput()
			}
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		inputReader = gzReader
	}

	// Create pipe to stream data to HTTP request
	pr, pw := io.Pipe()

	// Start goroutine to copy input to pipe
	copyErr := make(chan error, 1)
	go func() {
		defer pw.Close()
		_, err := io.Copy(pw, inputReader)
		copyErr <- err
	}()

	// Create HTTP request
	req, err := http.NewRequest("POST", cfg.URL+"/import", pr)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Transfer-Encoding", "chunked")

	// Send request
	client := &http.Client{
		Timeout: 0, // No timeout for streaming
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for non-OK status (before streaming starts)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Check copy error
	if err := <-copyErr; err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	// Close input file if opened
	if closeInput != nil {
		if err := closeInput(); err != nil {
			return fmt.Errorf("failed to close input file: %w", err)
		}
	}

	// Parse SSE response
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE format: "data: {...}"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		var event ImportProgressEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse event: %v\n", err)
			continue
		}

		// Handle error event
		if event.Error != "" {
			return fmt.Errorf("%s: %s (line %d)", event.Error, event.Message, event.Line)
		}

		// Handle done event
		if event.Done {
			fmt.Fprintf(os.Stderr, "\rImported: %d events in %s\n", event.Imported, event.Elapsed)
			return nil
		}

		// Handle progress event
		fmt.Fprintf(os.Stderr, "\rImported: %d events (gpos: %d)...", event.Imported, event.GPos)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	return nil
}
