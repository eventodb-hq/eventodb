package pebble

import (
	"github.com/klauspost/compress/s2"
)

// compressJSON compresses JSON bytes using S2 (Snappy successor)
// S2 is optimized for speed and works excellently with JSON data
func compressJSON(data []byte) []byte {
	return s2.Encode(nil, data)
}

// decompressJSON decompresses S2-compressed JSON bytes
func decompressJSON(compressed []byte) ([]byte, error) {
	return s2.Decode(nil, compressed)
}
