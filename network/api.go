package network

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/hashicorp/go-retryablehttp"
)

type prepareUploadRequest struct {
	CacheKey           string `json:"cache_key"`
	ArchiveSizeInBytes int64  `json:"archive_size_in_bytes"`
}

type prepareUploadResponse struct {
	ID        string `json:"id"`
	UploadURL string `json:"url"`
}

type apiClient struct {
	httpClient  *retryablehttp.Client
	baseURL     string
	accessToken string
}

func newApiClient(client *retryablehttp.Client, baseURL string, accessToken string) apiClient {
	return apiClient{
		httpClient:  client,
		baseURL:     baseURL,
		accessToken: accessToken,
	}
}

func (c apiClient) prepareUpload(requestBody prepareUploadRequest) (prepareUploadResponse, error) {
	url := fmt.Sprintf("%s/upload", c.baseURL)

	body, err := json.Marshal(requestBody)
	if err != nil {
		return prepareUploadResponse{}, err
	}

	req, err := retryablehttp.NewRequest("POST", url, body)
	if err != nil {
		return prepareUploadResponse{}, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("Content-type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return prepareUploadResponse{}, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		errorResp, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return prepareUploadResponse{}, err
		}
		return prepareUploadResponse{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorResp)
	}

	var response prepareUploadResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return prepareUploadResponse{}, err
	}

	return response, nil
}

func (c apiClient) uploadArchive(archivePath string, archiveSize int64, uploadURL string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}

	req, err := retryablehttp.NewRequest("PUT", uploadURL, file)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Content-Length-Range", fmt.Sprintf("%d,%d", archiveSize, archiveSize))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		return nil
	} else {
		errorResp, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorResp)
	}
}

func (c apiClient) acknowledgeUpload(uploadID string) error {
	url := fmt.Sprintf("%s/upload/%s/acknowledge", c.baseURL, uploadID)

	req, err := retryablehttp.NewRequest("PATCH", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		return nil
	} else {
		errorResp, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorResp)
	}

}
