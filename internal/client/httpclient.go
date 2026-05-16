package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/dublin/emusync/internal/model"
)

// encodeEmulatorPath escapes a single path segment (emulator name, conflict id).
func encodeEmulatorPath(s string) string {
	return url.PathEscape(s)
}

// encodeFilePathSegments splits a logical save path on "/" (after normalizing to slash),
// escapes each segment, and rejoins. Empty logical path returns "".
func encodeFilePathSegments(filePath string) string {
	rel := filepath.ToSlash(strings.TrimPrefix(filePath, "/"))
	if rel == "" || rel == "." {
		return ""
	}
	parts := strings.Split(rel, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// APIClient communicates with the emusync server.
type APIClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewAPIClient creates a new API client.
func NewAPIClient(baseURL, authToken string) *APIClient {
	return &APIClient{
		baseURL:   baseURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (c *APIClient) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	var resp *http.Response
	var err error
	canRetry := req.Body == nil || req.GetBody != nil
	maxAttempts := 3
	if !canRetry {
		maxAttempts = 1
	}
	for attempt := range maxAttempts {
		resp, err = c.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		if attempt < maxAttempts-1 {
			slog.Debug("request failed, retrying", "attempt", attempt+1, "error", err)
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	return nil, fmt.Errorf("request failed after %d attempt(s): %w", maxAttempts, err)
}

// GetManifest retrieves the file manifest for an emulator.
func (c *APIClient) GetManifest(ctx context.Context, emulator string) (*model.Manifest, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/manifest/%s", c.baseURL, encodeEmulatorPath(emulator)), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(body))
		if msg != "" {
			return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var manifest model.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decoding manifest: %w", err)
	}
	return &manifest, nil
}

// DownloadFile downloads a file from the server.
func (c *APIClient) DownloadFile(ctx context.Context, emulator, path string) (io.ReadCloser, *model.FileEntry, error) {
	enc := encodeFilePathSegments(path)
	if enc == "" {
		return nil, nil, fmt.Errorf("file path is empty")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files/%s/%s", c.baseURL, encodeEmulatorPath(emulator), enc), nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	ts, _ := time.Parse(time.RFC3339, resp.Header.Get("X-Timestamp"))
	entry := &model.FileEntry{
		SHA256:    resp.Header.Get("X-SHA256"),
		Timestamp: ts,
		DeviceID:  resp.Header.Get("X-Device-ID"),
		Size:      resp.ContentLength,
	}

	return resp.Body, entry, nil
}

// UploadFile uploads a file to the server. Returns a conflict if detected.
func (c *APIClient) UploadFile(ctx context.Context, emulator, path string, r io.Reader, meta model.FileEntry, baseHash string) (*model.Conflict, error) {
	enc := encodeFilePathSegments(path)
	if enc == "" {
		return nil, fmt.Errorf("file path is empty")
	}
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/api/v1/files/%s/%s", c.baseURL, encodeEmulatorPath(emulator), enc), r)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Device-ID", meta.DeviceID)
	req.Header.Set("X-SHA256", meta.SHA256)
	req.Header.Set("X-Timestamp", meta.Timestamp.Format(time.RFC3339))
	req.Header.Set("X-Base-Hash", baseHash)
	if meta.Size > 0 {
		req.ContentLength = meta.Size
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))

	if resp.StatusCode == http.StatusConflict {
		var result struct {
			Conflict model.Conflict `json:"conflict"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("decoding conflict response: %w", err)
		}
		return &result.Conflict, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return nil, nil
}

// GetConflicts lists all unresolved conflicts.
func (c *APIClient) GetConflicts(ctx context.Context) ([]model.Conflict, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/conflicts", c.baseURL), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Conflicts []model.Conflict `json:"conflicts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Conflicts, nil
}

// ResolveConflict resolves a conflict on the server.
func (c *APIClient) ResolveConflict(ctx context.Context, id string, choice string) error {
	body, _ := json.Marshal(map[string]string{"choice": choice})
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/conflicts/%s/resolve", c.baseURL, encodeEmulatorPath(id)), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetHistory retrieves version history for a file.
func (c *APIClient) GetHistory(ctx context.Context, emulator, path string) ([]model.VersionEntry, error) {
	enc := encodeFilePathSegments(path)
	if enc == "" {
		return nil, fmt.Errorf("file path is empty")
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/history/%s/%s", c.baseURL, encodeEmulatorPath(emulator), enc), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Versions []model.VersionEntry `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Versions, nil
}
