package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/hashicorp/go-retryablehttp"
)

// CheckChunksRequest represents the request to check which chunks already exist
type CheckChunksRequest struct {
	Hashes []string `json:"hashes"`
}

// CheckChunksResponse represents the response with missing chunk hashes
type CheckChunksResponse struct {
	Missing []string `json:"missing"`
}

// CreateChunkedUploadRequest represents the request to create a chunked upload
type CreateChunkedUploadRequest struct {
	CacheKey     string   `json:"cache_key"`
	ChunkHashes  []string `json:"chunk_hashes"`
	TotalSize    int64    `json:"total_size"`
	ManifestJSON string   `json:"manifest"`
}

// ChunkURL represents a presigned URL for uploading a chunk
type ChunkURL struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// CreateChunkedUploadResponse represents the response with presigned URLs
type CreateChunkedUploadResponse struct {
	ID        string              `json:"id"`
	ChunkURLs map[string]ChunkURL `json:"chunk_urls"`
}

// AcknowledgeChunkedUploadRequest represents the request to acknowledge a chunked upload
type AcknowledgeChunkedUploadRequest struct {
	UploadID   string `json:"upload_id"`
	Successful bool   `json:"successful"`
}

// AcknowledgeResponse represents the server response after acknowledging an upload
type AcknowledgeResponse struct {
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// CDCClient handles CDC-specific API operations
type CDCClient struct {
	httpClient  *retryablehttp.Client
	baseURL     string
	accessToken string
	logger      log.Logger
}

// NewCDCClient creates a new CDC API client
func NewCDCClient(httpClient *retryablehttp.Client, baseURL string, accessToken string, logger log.Logger) *CDCClient {
	return &CDCClient{
		httpClient:  httpClient,
		baseURL:     baseURL,
		accessToken: accessToken,
		logger:      logger,
	}
}

// CheckChunks queries the server for which chunks already exist
func (c *CDCClient) CheckChunks(hashes []string) (*CheckChunksResponse, error) {
	url := fmt.Sprintf("%s/v1/check-chunks", c.baseURL)

	requestBody := CheckChunksRequest{
		Hashes: hashes,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.unwrapError(resp)
	}

	var response CheckChunksResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// CreateChunkedUpload creates a new chunked upload and gets presigned URLs
func (c *CDCClient) CreateChunkedUpload(request CreateChunkedUploadRequest) (*CreateChunkedUploadResponse, error) {
	url := fmt.Sprintf("%s/v1/create-chunked-upload", c.baseURL)

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.unwrapError(resp)
	}

	var response CreateChunkedUploadResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// AcknowledgeChunkedUpload acknowledges the completion of a chunked upload
func (c *CDCClient) AcknowledgeChunkedUpload(uploadID string, successful bool) (*AcknowledgeResponse, error) {
	url := fmt.Sprintf("%s/v1/acknowledge-chunked-upload", c.baseURL)

	requestBody := AcknowledgeChunkedUploadRequest{
		UploadID:   uploadID,
		Successful: successful,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-Type", "application/json")
	if buildSlug := os.Getenv("BITRISE_BUILD_SLUG"); buildSlug != "" {
		req.Header.Set("X-Build-Slug", buildSlug)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.unwrapError(resp)
	}

	var response AcknowledgeResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

func (c *CDCClient) unwrapError(resp *http.Response) error {
	errorResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorResp)
}

