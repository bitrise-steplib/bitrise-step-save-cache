package uploader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/retryhttp"
	"github.com/bitrise-steplib/bitrise-step-save-cache/internal/api"
	"github.com/bitrise-steplib/bitrise-step-save-cache/internal/chunking"
	"github.com/docker/go-units"
	"github.com/klauspost/compress/zstd"
)

// ChunkedUploader handles uploading files using content-defined chunking
type ChunkedUploader struct {
	client *api.CDCClient
	logger log.Logger
}

// NewChunkedUploader creates a new chunked uploader
func NewChunkedUploader(apiBaseURL, accessToken string, logger log.Logger) *ChunkedUploader {
	httpClient := retryhttp.NewClient(logger)
	client := api.NewCDCClient(httpClient, apiBaseURL, accessToken, logger)
	return &ChunkedUploader{
		client: client,
		logger: logger,
	}
}

// UploadParams contains parameters for uploading a file
type UploadParams struct {
	FilePath         string
	CacheKey         string
	CompressChunks   bool
	CompressionLevel int
}

// Upload uploads a file using content-defined chunking with deduplication
func (u *ChunkedUploader) Upload(ctx context.Context, params UploadParams) error {
	u.logger.Infof("Starting chunked upload for file: %s", params.FilePath)
	if params.CompressChunks {
		u.logger.Infof("Per-chunk compression enabled (zstd level %d)", params.CompressionLevel)
	}

	// Step 1: Chunk the file using FastCDC
	u.logger.Infof("Chunking file using FastCDC (min=%s, avg=%s, max=%s)...",
		units.HumanSize(float64(chunking.MinChunkSize)),
		units.HumanSize(float64(chunking.AvgChunkSize)),
		units.HumanSize(float64(chunking.MaxChunkSize)))

	chunks, err := chunking.ChunkFile(params.FilePath)
	if err != nil {
		return fmt.Errorf("failed to chunk file: %w", err)
	}

	u.logger.Donef("File split into %d chunks", len(chunks))

	// Calculate total size
	var totalSize int64
	for _, chunk := range chunks {
		totalSize += chunk.Size
	}

	// Step 2: Check which chunks already exist (deduplication)
	u.logger.Println()
	u.logger.Infof("Checking for existing chunks...")

	chunkHashes := make([]string, len(chunks))
	for i, chunk := range chunks {
		chunkHashes[i] = chunk.Hash
	}

	checkResp, err := u.client.CheckChunks(chunkHashes)
	if err != nil {
		return fmt.Errorf("failed to check chunks: %w", err)
	}

	existingChunks := len(chunks) - len(checkResp.Missing)
	u.logger.Donef("Found %d existing chunks, %d new chunks to upload", existingChunks, len(checkResp.Missing))

	if existingChunks > 0 {
		var deduplicatedSize int64
		missingSet := make(map[string]bool)
		for _, hash := range checkResp.Missing {
			missingSet[hash] = true
		}
		for _, chunk := range chunks {
			if !missingSet[chunk.Hash] {
				deduplicatedSize += chunk.Size
			}
		}
		u.logger.Infof("💾 Deduplication saved %s of data", units.HumanSizeWithPrecision(float64(deduplicatedSize), 3))
	}

	// Step 3: Create manifest
	manifest := chunking.NewManifest(chunks, totalSize)
	manifestJSON, err := manifest.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to create manifest: %w", err)
	}

	// Step 4: Create chunked upload and get presigned URLs
	u.logger.Println()
	u.logger.Infof("Creating chunked upload...")

	createReq := api.CreateChunkedUploadRequest{
		CacheKey:     params.CacheKey,
		ChunkHashes:  checkResp.Missing,
		TotalSize:    totalSize,
		ManifestJSON: string(manifestJSON),
	}

	createResp, err := u.client.CreateChunkedUpload(createReq)
	if err != nil {
		return fmt.Errorf("failed to create chunked upload: %w", err)
	}

	u.logger.Donef("Upload ID: %s", createResp.ID)

	// Step 5: Upload missing chunks in parallel
	if len(checkResp.Missing) > 0 {
		u.logger.Println()
		if params.CompressChunks {
			u.logger.Infof("Uploading %d chunks (with zstd compression)...", len(checkResp.Missing))
		} else {
			u.logger.Infof("Uploading %d chunks...", len(checkResp.Missing))
		}

		err = u.uploadChunks(ctx, params.FilePath, chunks, checkResp.Missing, createResp.ChunkURLs, params.CompressChunks, params.CompressionLevel)
		if err != nil {
			// Try to acknowledge failure
			u.logger.Warnf("Upload failed, notifying server...")
			if _, ackErr := u.client.AcknowledgeChunkedUpload(createResp.ID, false); ackErr != nil {
				u.logger.Errorf("Failed to acknowledge upload failure: %v", ackErr)
			}
			return fmt.Errorf("failed to upload chunks: %w", err)
		}

		u.logger.Donef("All chunks uploaded successfully")
	} else {
		u.logger.Println()
		u.logger.Infof("No chunks to upload (all deduplicated)")
	}

	// Step 6: Acknowledge successful upload
	u.logger.Println()
	u.logger.Infof("Finalizing upload...")

	ackResp, err := u.client.AcknowledgeChunkedUpload(createResp.ID, true)
	if err != nil {
		return fmt.Errorf("failed to acknowledge upload: %w", err)
	}

	if ackResp.Message != "" {
		u.logResponse(ackResp)
	}

	return nil
}

// uploadChunks uploads all missing chunks in parallel
func (u *ChunkedUploader) uploadChunks(ctx context.Context, filePath string, allChunks []chunking.Chunk, missingHashes []string, chunkURLs map[string]api.ChunkURL, compressChunks bool, compressionLevel int) error {
	// Create a map of missing hashes for quick lookup
	missingSet := make(map[string]bool)
	for _, hash := range missingHashes {
		missingSet[hash] = true
	}

	// Find chunks that need to be uploaded
	var chunksToUpload []chunking.Chunk
	for _, chunk := range allChunks {
		if missingSet[chunk.Hash] {
			chunksToUpload = append(chunksToUpload, chunk)
		}
	}

	// Upload chunks in parallel
	concurrency := getDefaultConcurrency()
	semaphore := make(chan struct{}, concurrency)
	errChan := make(chan error, len(chunksToUpload))
	var wg sync.WaitGroup

	stats := &uploadStatistics{}

	for i, chunk := range chunksToUpload {
		wg.Add(1)
		go func(index int, c chunking.Chunk) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			chunkURL, ok := chunkURLs[c.Hash]
			if !ok {
				errChan <- fmt.Errorf("no URL provided for chunk %s", c.Hash)
				return
			}

			if err := u.uploadChunk(ctx, filePath, c, chunkURL, index+1, len(chunksToUpload), stats, compressChunks, compressionLevel); err != nil {
				errChan <- fmt.Errorf("failed to upload chunk %d: %w", index+1, err)
			}
		}(i, chunk)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err
	}

	return nil
}

// uploadStatistics tracks upload performance
type uploadStatistics struct {
	sum            time.Duration
	finishedChunks int64
	mu             sync.Mutex
}

func (s *uploadStatistics) update(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sum += d
	s.finishedChunks++
}

func (s *uploadStatistics) average() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finishedChunks == 0 {
		return 0
	}
	return s.sum / time.Duration(s.finishedChunks)
}

func (s *uploadStatistics) getFinishedCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finishedChunks
}

// uploadChunk uploads a single chunk with retry logic
func (u *ChunkedUploader) uploadChunk(ctx context.Context, filePath string, chunk chunking.Chunk, chunkURL api.ChunkURL, index, total int, stats *uploadStatistics, compressChunks bool, compressionLevel int) error {
	maxRetries := 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("chunk %d upload cancelled: %w", index, ctx.Err())
		default:
		}

		u.logger.Debugf("Uploading chunk %d/%d (attempt %d/%d) [finished=%d] [avg=%v]",
			index, total, attempt+1, maxRetries,
			stats.getFinishedCount(), stats.average().Round(time.Second))

		start := time.Now()

		// Read chunk data from file
		chunkData, err := chunking.ReadChunk(filePath, chunk.Offset, chunk.Size)
		if err != nil {
			return fmt.Errorf("failed to read chunk data: %w", err)
		}

		originalSize := len(chunkData)
		uploadData := chunkData

		// Compress chunk if enabled
		if compressChunks {
			compressedData, err := compressWithZstd(chunkData, compressionLevel)
			if err != nil {
				return fmt.Errorf("failed to compress chunk: %w", err)
			}
			uploadData = compressedData
		}

		// Upload chunk
		err = u.uploadChunkData(ctx, uploadData, chunkURL)
		if err == nil {
			took := time.Since(start)
			stats.update(took)
			if compressChunks {
				ratio := float64(len(uploadData)) / float64(originalSize) * 100
				u.logger.Infof("Chunk %d/%d uploaded in %v (%s → %s, %.1f%%)",
					index, total, took.Round(time.Second),
					units.HumanSizeWithPrecision(float64(originalSize), 3),
					units.HumanSizeWithPrecision(float64(len(uploadData)), 3),
					ratio)
			} else {
				u.logger.Infof("Chunk %d/%d uploaded successfully in %v (%s)",
					index, total, took.Round(time.Second),
					units.HumanSizeWithPrecision(float64(len(uploadData)), 3))
			}
			return nil
		}

		u.logger.Warnf("Chunk %d attempt %d failed: %v", index, attempt+1, err)

		if attempt < maxRetries-1 {
			backoff := time.Duration((attempt+1)*2) * time.Second
			u.logger.Warnf("Retrying after %v...", backoff)
			time.Sleep(backoff)
		}
	}

	return fmt.Errorf("chunk %d failed after %d attempts", index, maxRetries)
}

// compressWithZstd compresses data using zstd
func compressWithZstd(data []byte, level int) ([]byte, error) {
	// Map level 1-19 to zstd encoder levels
	var encoderLevel zstd.EncoderLevel
	switch {
	case level <= 3:
		encoderLevel = zstd.SpeedFastest
	case level <= 6:
		encoderLevel = zstd.SpeedDefault
	case level <= 12:
		encoderLevel = zstd.SpeedBetterCompression
	default:
		encoderLevel = zstd.SpeedBestCompression
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(encoderLevel))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}
	defer encoder.Close()

	return encoder.EncodeAll(data, nil), nil
}

// uploadChunkData uploads chunk data to the presigned URL
func (u *ChunkedUploader) uploadChunkData(ctx context.Context, data []byte, chunkURL api.ChunkURL) error {
	req, err := http.NewRequestWithContext(ctx, chunkURL.Method, chunkURL.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers from presigned URL
	for key, value := range chunkURL.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))

	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxConnsPerHost:     20,
			IdleConnTimeout:     10 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
			Proxy:               http.ProxyFromEnvironment,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (u *ChunkedUploader) logResponse(response *api.AcknowledgeResponse) {
	if response.Message == "" || response.Severity == "" {
		return
	}

	var loggerFn func(format string, v ...interface{})
	switch response.Severity {
	case "debug":
		loggerFn = u.logger.Debugf
	case "info":
		loggerFn = u.logger.Infof
	case "warning":
		loggerFn = u.logger.Warnf
	case "error":
		loggerFn = u.logger.Errorf
	default:
		loggerFn = u.logger.Printf
	}

	loggerFn("\n")
	loggerFn(response.Message)
	loggerFn("\n")
}

func getDefaultConcurrency() int {
	c := runtime.NumCPU() * 3

	if c > 20 {
		c = 20
	}

	if c < 2 {
		c = 2
	}

	return c
}
