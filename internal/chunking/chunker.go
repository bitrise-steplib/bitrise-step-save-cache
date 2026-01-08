package chunking

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"

	chunkers "github.com/PlakarKorp/go-cdc-chunkers"
)

const (
	// MinChunkSize is the minimum chunk size (512 KB)
	MinChunkSize = 512 * 1024
	// AvgChunkSize is the average chunk size (1 MB)
	AvgChunkSize = 1024 * 1024
	// MaxChunkSize is the maximum chunk size (8 MB)
	MaxChunkSize = 8 * 1024 * 1024
)

// Chunk represents a single chunk of data
type Chunk struct {
	Hash   string `json:"hash"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
}

// ChunkFile splits a file into content-defined chunks using FastCDC
func ChunkFile(filePath string) ([]Chunk, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return ChunkReader(file)
}

// ChunkReader splits a reader into content-defined chunks using FastCDC
func ChunkReader(reader io.Reader) ([]Chunk, error) {
	// Create FastCDC chunker with min, avg (normal), max chunk sizes
	opts := &chunkers.ChunkerOpts{
		MinSize:    MinChunkSize,
		NormalSize: AvgChunkSize,
		MaxSize:    MaxChunkSize,
	}

	chunker, err := chunkers.NewChunker("fastcdc", reader, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunker: %w", err)
	}

	var chunks []Chunk
	var offset int64

	for {
		chunkData, err := chunker.Next()
		if err == io.EOF {
			// Check if there's any data in the last chunk
			if len(chunkData) > 0 {
				// Calculate SHA-256 hash of the final chunk
				hash := sha256.Sum256(chunkData)
				hashHex := fmt.Sprintf("%x", hash)

				chunks = append(chunks, Chunk{
					Hash:   hashHex,
					Size:   int64(len(chunkData)),
					Offset: offset,
				})
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to chunk data: %w", err)
		}

		// Calculate SHA-256 hash of the chunk
		hash := sha256.Sum256(chunkData)
		hashHex := fmt.Sprintf("%x", hash)

		chunks = append(chunks, Chunk{
			Hash:   hashHex,
			Size:   int64(len(chunkData)),
			Offset: offset,
		})

		offset += int64(len(chunkData))
	}

	return chunks, nil
}

// ReadChunk reads a specific chunk from a file by offset and size
func ReadChunk(filePath string, offset, size int64) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek to offset %d: %w", offset, err)
	}

	data := make([]byte, size)
	n, err := io.ReadFull(file, data)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read chunk: %w", err)
	}

	return data[:n], nil
}
