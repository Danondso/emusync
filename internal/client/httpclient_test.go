package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/model"
)

func TestAPIClient_GetManifest_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	manifest := model.Manifest{
		Emulator: "retroarch",
		Files: map[string]model.FileEntry{
			"saves/slot1.srm": {
				SHA256:    "abc123",
				Size:      1024,
				Timestamp: now,
				DeviceID:  "device-1",
			},
		},
		UpdatedAt: now,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/manifest/retroarch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	got, err := client.GetManifest(context.Background(), "retroarch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Emulator != "retroarch" {
		t.Errorf("emulator = %q, want %q", got.Emulator, "retroarch")
	}
	if len(got.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(got.Files))
	}
	entry, ok := got.Files["saves/slot1.srm"]
	if !ok {
		t.Fatal("missing file entry saves/slot1.srm")
	}
	if entry.SHA256 != "abc123" {
		t.Errorf("sha256 = %q, want %q", entry.SHA256, "abc123")
	}
	if entry.Size != 1024 {
		t.Errorf("size = %d, want %d", entry.Size, 1024)
	}
}

func TestAPIClient_GetManifest_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	_, err := client.GetManifest(context.Background(), "retroarch")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500, got: %v", err)
	}
}

func TestAPIClient_DownloadFile_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	content := "save-file-data-here"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/files/retroarch/saves/slot1.srm" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("X-SHA256", "deadbeef")
		w.Header().Set("X-Timestamp", now.Format(time.RFC3339))
		w.Header().Set("X-Device-ID", "deck")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	reader, entry, err := client.DownloadFile(context.Background(), "retroarch", "saves/slot1.srm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(body) != content {
		t.Errorf("body = %q, want %q", string(body), content)
	}
	if entry.SHA256 != "deadbeef" {
		t.Errorf("sha256 = %q, want %q", entry.SHA256, "deadbeef")
	}
	if !entry.Timestamp.Equal(now) {
		t.Errorf("timestamp = %v, want %v", entry.Timestamp, now)
	}
	if entry.DeviceID != "deck" {
		t.Errorf("device_id = %q, want %q", entry.DeviceID, "deck")
	}
	if entry.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", entry.Size, len(content))
	}
}

func TestAPIClient_DownloadFile_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	_, _, err := client.DownloadFile(context.Background(), "retroarch", "saves/missing.srm")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

func TestAPIClient_UploadFile_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	content := "uploaded-save-data"

	var receivedHeaders http.Header
	var receivedBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		receivedHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	meta := model.FileEntry{
		SHA256:    "aabbcc",
		Size:      int64(len(content)),
		Timestamp: now,
		DeviceID:  "deck",
	}

	conflict, err := client.UploadFile(context.Background(), "retroarch", "saves/slot1.srm",
		strings.NewReader(content), meta, "previoushash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflict != nil {
		t.Fatal("expected nil conflict on 200")
	}

	// Verify headers were sent correctly
	if got := receivedHeaders.Get("X-Device-ID"); got != "deck" {
		t.Errorf("X-Device-ID = %q, want %q", got, "deck")
	}
	if got := receivedHeaders.Get("X-SHA256"); got != "aabbcc" {
		t.Errorf("X-SHA256 = %q, want %q", got, "aabbcc")
	}
	if got := receivedHeaders.Get("X-Timestamp"); got != now.Format(time.RFC3339) {
		t.Errorf("X-Timestamp = %q, want %q", got, now.Format(time.RFC3339))
	}
	if got := receivedHeaders.Get("X-Base-Hash"); got != "previoushash" {
		t.Errorf("X-Base-Hash = %q, want %q", got, "previoushash")
	}
	if receivedBody != content {
		t.Errorf("body = %q, want %q", receivedBody, content)
	}
}

func TestAPIClient_UploadFile_Conflict(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	conflictResp := map[string]any{
		"conflict": model.Conflict{
			ID:       "conflict-1",
			Emulator: "retroarch",
			Path:     "saves/slot1.srm",
			Local: model.FileEntry{
				SHA256:    "local-hash",
				Timestamp: now,
				DeviceID:  "deck",
			},
			Remote: model.FileEntry{
				SHA256:    "remote-hash",
				Timestamp: now.Add(-time.Hour),
				DeviceID:  "desktop",
			},
			DetectedAt: now,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the body to avoid connection reset
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(conflictResp)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	content := "0123456789"
	meta := model.FileEntry{
		SHA256:    "local-hash",
		Size:      int64(len(content)),
		Timestamp: now,
		DeviceID:  "deck",
	}

	conflict, err := client.UploadFile(context.Background(), "retroarch", "saves/slot1.srm",
		strings.NewReader(content), meta, "old-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conflict == nil {
		t.Fatal("expected non-nil conflict on 409")
	}
	if conflict.ID != "conflict-1" {
		t.Errorf("conflict ID = %q, want %q", conflict.ID, "conflict-1")
	}
	if conflict.Emulator != "retroarch" {
		t.Errorf("conflict emulator = %q, want %q", conflict.Emulator, "retroarch")
	}
}

func TestAPIClient_GetConflicts(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	conflicts := []model.Conflict{
		{
			ID:         "c1",
			Emulator:   "retroarch",
			Path:       "saves/slot1.srm",
			DetectedAt: now,
		},
		{
			ID:         "c2",
			Emulator:   "dolphin",
			Path:       "saves/game.gci",
			DetectedAt: now,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/conflicts" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"conflicts": conflicts})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	got, err := client.GetConflicts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(got))
	}
	if got[0].ID != "c1" {
		t.Errorf("first conflict ID = %q, want %q", got[0].ID, "c1")
	}
	if got[1].ID != "c2" {
		t.Errorf("second conflict ID = %q, want %q", got[1].ID, "c2")
	}
}

func TestAPIClient_ResolveConflict_Success(t *testing.T) {
	var receivedBody map[string]string
	var receivedPath string
	var receivedMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "resolved"})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	err := client.ResolveConflict(context.Background(), "conflict-42", "local")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedMethod != "POST" {
		t.Errorf("method = %q, want POST", receivedMethod)
	}
	if receivedPath != "/api/v1/conflicts/conflict-42/resolve" {
		t.Errorf("path = %q, want /api/v1/conflicts/conflict-42/resolve", receivedPath)
	}
	if receivedBody["choice"] != "local" {
		t.Errorf("choice = %q, want %q", receivedBody["choice"], "local")
	}
}

func TestAPIClient_ResolveConflict_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain body
		io.ReadAll(r.Body)
		http.Error(w, "bad choice", http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	err := client.ResolveConflict(context.Background(), "conflict-42", "invalid")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention 400, got: %v", err)
	}
}

func TestAPIClient_GetHistory(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	versions := []model.VersionEntry{
		{
			SHA256:    "hash-v2",
			Size:      2048,
			Timestamp: now,
			DeviceID:  "deck",
		},
		{
			SHA256:    "hash-v1",
			Size:      1024,
			Timestamp: now.Add(-24 * time.Hour),
			DeviceID:  "desktop",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/history/retroarch/saves/slot1.srm" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"emulator": "retroarch",
			"path":     "saves/slot1.srm",
			"versions": versions,
		})
	}))
	defer srv.Close()

	client := NewAPIClient(srv.URL, "")
	got, err := client.GetHistory(context.Background(), "retroarch", "saves/slot1.srm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(got))
	}
	if got[0].SHA256 != "hash-v2" {
		t.Errorf("first version sha256 = %q, want %q", got[0].SHA256, "hash-v2")
	}
	if got[1].SHA256 != "hash-v1" {
		t.Errorf("second version sha256 = %q, want %q", got[1].SHA256, "hash-v1")
	}
	if got[0].Size != 2048 {
		t.Errorf("first version size = %d, want %d", got[0].Size, 2048)
	}
}

func TestAPIClient_AuthHeaderSent(t *testing.T) {
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(model.Manifest{
			Emulator: "retroarch",
			Files:    make(map[string]model.FileEntry),
		})
	}))
	defer srv.Close()

	// Test with token
	client := NewAPIClient(srv.URL, "my-secret-token")
	_, err := client.GetManifest(context.Background(), "retroarch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q, want %q", receivedAuth, "Bearer my-secret-token")
	}

	// Test without token
	receivedAuth = ""
	clientNoAuth := NewAPIClient(srv.URL, "")
	_, err = clientNoAuth.GetManifest(context.Background(), "retroarch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuth != "" {
		t.Errorf("expected no Authorization header when token is empty, got %q", receivedAuth)
	}
}
