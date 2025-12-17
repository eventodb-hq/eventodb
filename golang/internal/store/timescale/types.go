package timescale

import "time"

// ChunkInfo contains information about a TimescaleDB chunk
type ChunkInfo struct {
	ChunkName    string    `json:"chunk_name"`
	RangeStart   time.Time `json:"range_start"`
	RangeEnd     time.Time `json:"range_end"`
	IsCompressed bool      `json:"is_compressed"`
	SizeBytes    int64     `json:"size_bytes"`
	MessageCount int64     `json:"message_count"`
}

// CompressionStats contains compression statistics for a namespace
type CompressionStats struct {
	TotalChunks        int   `json:"total_chunks"`
	CompressedChunks   int   `json:"compressed_chunks"`
	UncompressedChunks int   `json:"uncompressed_chunks"`
	TotalSizeBytes     int64 `json:"total_size_bytes"`
	CompressedBytes    int64 `json:"compressed_bytes"`
	UncompressedBytes  int64 `json:"uncompressed_bytes"`
}
