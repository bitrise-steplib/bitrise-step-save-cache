//go:build integration
// +build integration

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/bitrise-io/go-utils/v2/log"

	"github.com/bitrise-steplib/steps-save-cache/network"
)

func TestUpload(t *testing.T) {
	// Given
	cacheKey := "integration-test"
	baseURL := os.Getenv("ABCS_API_URL")
	token := os.Getenv("BITRISEIO_CACHE_SERVICE_ACCESS_TOKEN")
	testFile := "testdata/test.tzst"
	params := network.UploadParams{
		APIBaseURL:  baseURL,
		Token:       token,
		ArchivePath: testFile,
		ArchiveSize: 468,
		CacheKey:    cacheKey,
	}
	logger := log.NewLogger()
	logger.EnableDebugLog(true)

	// When
	err := network.Upload(params, logger)

	// Then
	if err != nil {
		t.Errorf(err.Error())
	}

	bytes, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Errorf(err.Error())
	}
	expectedChecksum := checksumOf(bytes)
	checksum, err := downloadArchive(cacheKey, baseURL, token)
	if err != nil {
		t.Errorf(err.Error())
	}
	assert.Equal(t, expectedChecksum, checksum)
}

// downloadArchive downloads the archive from the API based on cacheKey and returns its SHA256 checksum
func downloadArchive(cacheKey string, baseURL string, token string) (string, error) {
	client := retryablehttp.NewClient()

	// Obtain pre-signed download URL
	url := fmt.Sprintf("%s/restore?cache_keys=%s", baseURL, cacheKey)
	req, err := retryablehttp.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp.Body)

	var parsedResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&parsedResp)
	if err != nil {
		return "", err
	}
	downloadURL := parsedResp["url"].(string)

	// Download archive using pre-signed URL
	resp2, err := retryablehttp.Get(downloadURL)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			panic(err)
		}
	}(resp2.Body)

	bytes, err := ioutil.ReadAll(resp2.Body)
	if err != nil {
		return "", err
	}

	return checksumOf(bytes), nil
}

func checksumOf(bytes []byte) string {
	hash := sha256.New()
	hash.Write(bytes)
	return hex.EncodeToString(hash.Sum(nil))
}