package checkpoint

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	checkpointRequestTimeout = 30 * time.Second
	checkpointMaxSSZBytes    = 64 << 20
)

var checkpointHTTPClient = &http.Client{Timeout: checkpointRequestTimeout}

func fetchSSZ(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := checkpointHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d for %s", resp.StatusCode, url)
	}

	body, err := readLimitedSSZ(resp.Body, checkpointMaxSSZBytes)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

func readLimitedSSZ(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("checkpoint response exceeds %d bytes", maxBytes)
	}
	return body, nil
}
