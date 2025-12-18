// Load testing tool for Message DB profiling
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	duration   = flag.Duration("duration", 30*time.Second, "Test duration")
	workers    = flag.Int("workers", 10, "Number of concurrent workers")
	baseURL    = flag.String("url", "http://localhost:8080", "Server URL")
	profileDir = flag.String("profile-dir", "", "Profile output directory")
)

type Stats struct {
	writes       atomic.Int64
	reads        atomic.Int64
	errors       atomic.Int64
	totalLatency atomic.Int64 // microseconds
}

func main() {
	flag.Parse()

	stats := &Stats{}
	done := make(chan struct{})
	var wg sync.WaitGroup

	// Get default namespace token
	token := getDefaultToken()
	if token == "" {
		fmt.Fprintf(os.Stderr, "Error: Could not get default token\n")
		os.Exit(1)
	}

	fmt.Printf("Starting load test:\n")
	fmt.Printf("  Duration: %v\n", *duration)
	fmt.Printf("  Workers:  %d\n", *workers)
	fmt.Printf("  URL:      %s\n", *baseURL)
	fmt.Printf("\n")

	// Start progress reporter
	go reportProgress(done, stats)

	// Start workers
	startTime := time.Now()
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go worker(done, stats, token, i, &wg)
	}

	// Run for duration
	time.Sleep(*duration)
	close(done)
	wg.Wait()

	// Print results
	elapsed := time.Since(startTime)
	printStats(stats, elapsed)

	// Save results if profile dir specified
	if *profileDir != "" {
		saveStats(stats, elapsed, *profileDir)
	}
}

func worker(done chan struct{}, stats *Stats, token string, workerID int, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
		},
	}
	streamID := workerID * 1000000 // Offset per worker to reduce contention

	for {
		select {
		case <-done:
			return
		default:
			// Write message
			start := time.Now()
			if err := writeMessage(client, token, fmt.Sprintf("test-stream-%d", streamID%100)); err != nil {
				stats.errors.Add(1)
			} else {
				stats.writes.Add(1)
				stats.totalLatency.Add(time.Since(start).Microseconds())
			}

			// Read messages (every 10 writes)
			if streamID%10 == 0 {
				start := time.Now()
				if err := readMessages(client, token, fmt.Sprintf("test-stream-%d", streamID%100)); err != nil {
					stats.errors.Add(1)
				} else {
					stats.reads.Add(1)
					stats.totalLatency.Add(time.Since(start).Microseconds())
				}
			}

			streamID++
		}
	}
}

func writeMessage(client *http.Client, token, stream string) error {
	payload := []interface{}{
		"stream.write",
		stream,
		map[string]interface{}{
			"type": "TestEvent",
			"data": map[string]interface{}{
				"counter":   time.Now().UnixNano(),
				"payload":   "test data for profiling with some content to make it realistic",
				"nested": map[string]interface{}{
					"field1": "value1",
					"field2": 42,
					"field3": true,
				},
			},
			"metadata": map[string]interface{}{
				"correlationStreamName": "test-correlation",
				"source":                "load-test",
			},
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", *baseURL+"/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func readMessages(client *http.Client, token, stream string) error {
	payload := []interface{}{
		"stream.get",
		stream,
		map[string]interface{}{
			"position":  0,
			"batchSize": 10,
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", *baseURL+"/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func getDefaultToken() string {
	// Try to get token from environment first
	if token := os.Getenv("DEFAULT_TOKEN"); token != "" {
		return token
	}

	// Try to fetch from server's /admin/namespaces endpoint
	// This works when the server has just started with a fresh database
	client := &http.Client{Timeout: 5 * time.Second}
	
	// First try to list namespaces (works without auth)
	resp, err := client.Get(*baseURL + "/admin/namespaces")
	if err != nil {
		// Fallback to deterministic token for test mode (SQLite)
		return "ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		var namespaces []map[string]interface{}
		if err := json.Unmarshal(body, &namespaces); err == nil {
			// Find default namespace and extract token from it
			for _, ns := range namespaces {
				if id, ok := ns["id"].(string); ok && id == "default" {
					if token, ok := ns["token"].(string); ok && token != "" {
						return token
					}
				}
			}
		}
	}

	// Fallback to deterministic token for test mode (SQLite)
	return "ns_ZGVmYXVsdA_71d7e890c5bb4666a234cc1a9ec3f5f15b67c1a73257a3c92e1c0b0c5e0f8e9a"
}

func reportProgress(done chan struct{}, stats *Stats) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastWrites := int64(0)
	lastReads := int64(0)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			writes := stats.writes.Load()
			reads := stats.reads.Load()
			errors := stats.errors.Load()

			writeRate := float64(writes-lastWrites) / 5.0
			readRate := float64(reads-lastReads) / 5.0

			fmt.Printf("Progress: %d writes (%.0f/s), %d reads (%.0f/s), %d errors\n",
				writes, writeRate, reads, readRate, errors)

			lastWrites = writes
			lastReads = reads
		}
	}
}

func printStats(stats *Stats, duration time.Duration) {
	writes := stats.writes.Load()
	reads := stats.reads.Load()
	errors := stats.errors.Load()
	totalOps := writes + reads
	totalLatency := stats.totalLatency.Load()

	fmt.Printf("\n=== Load Test Results ===\n")
	fmt.Printf("Duration:      %v\n", duration)
	fmt.Printf("Total Ops:     %d\n", totalOps)
	fmt.Printf("  Writes:      %d\n", writes)
	fmt.Printf("  Reads:       %d\n", reads)
	fmt.Printf("  Errors:      %d\n", errors)
	fmt.Printf("Throughput:    %.0f ops/sec\n", float64(totalOps)/duration.Seconds())
	if totalOps > 0 {
		fmt.Printf("Avg Latency:   %.2f ms\n", float64(totalLatency)/float64(totalOps)/1000.0)
	}
	fmt.Printf("\n")
}

func saveStats(stats *Stats, duration time.Duration, dir string) {
	writes := stats.writes.Load()
	reads := stats.reads.Load()
	errors := stats.errors.Load()
	totalOps := writes + reads
	totalLatency := stats.totalLatency.Load()

	results := map[string]interface{}{
		"timestamp":   time.Now().Format(time.RFC3339),
		"duration_s":  duration.Seconds(),
		"total_ops":   totalOps,
		"writes":      writes,
		"reads":       reads,
		"errors":      errors,
		"ops_per_sec": float64(totalOps) / duration.Seconds(),
	}

	if totalOps > 0 {
		results["avg_latency_ms"] = float64(totalLatency) / float64(totalOps) / 1000.0
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(dir+"/load-test-results.json", data, 0644)
	fmt.Printf("Results saved to: %s/load-test-results.json\n", dir)
}
