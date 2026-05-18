package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	storage := NewStorage(t.TempDir(), 10)
	handlers := NewHandlers(storage)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)
	token := "test-token"
	server := httptest.NewServer(AuthMiddleware(token, mux))
	t.Cleanup(server.Close)
	return server, token
}

func putFile(t *testing.T, serverURL, token, emulator, path, content, baseHash string) *http.Response {
	t.Helper()
	hash := sha256hex(content)
	req, _ := http.NewRequest("PUT",
		fmt.Sprintf("%s/api/v1/files/%s/%s", serverURL, emulator, path),
		strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Device-ID", "test-device")
	req.Header.Set("X-SHA256", hash)
	req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))
	req.Header.Set("X-Base-Hash", baseHash)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestHandler_ReadyCheck(t *testing.T) {
	storage := NewStorage(t.TempDir(), 10)
	h := NewHandlers(storage)
	srv := httptest.NewServer(http.HandlerFunc(h.HandleReadyCheck))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestHandler_Health(t *testing.T) {
	srv, token := newTestServer(t)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestHandler_GetManifest_Empty(t *testing.T) {
	srv, token := newTestServer(t)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/manifest/retroarch", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var manifest struct {
		Emulator string                 `json:"emulator"`
		Files    map[string]interface{} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Emulator != "retroarch" {
		t.Fatalf("expected emulator retroarch, got %q", manifest.Emulator)
	}
	if len(manifest.Files) != 0 {
		t.Fatalf("expected empty files map, got %d entries", len(manifest.Files))
	}
}

func TestHandler_PutFile_Success(t *testing.T) {
	srv, token := newTestServer(t)

	content := "save-data-content-v1"
	resp := putFile(t, srv.URL, token, "retroarch", "saves/game.srm", content, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var putBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&putBody); err != nil {
		t.Fatal(err)
	}
	if putBody["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", putBody["status"])
	}

	// GET the file back and verify content
	getReq, _ := http.NewRequest("GET", srv.URL+"/api/v1/files/retroarch/saves/game.srm", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", getResp.StatusCode)
	}

	got, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestHandler_PutFile_MissingHeaders(t *testing.T) {
	srv, token := newTestServer(t)

	tests := []struct {
		name      string
		omitHeader string
	}{
		{"missing X-Device-ID", "X-Device-ID"},
		{"missing X-SHA256", "X-SHA256"},
		{"missing X-Timestamp", "X-Timestamp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "some-data"
			hash := sha256hex(content)
			req, _ := http.NewRequest("PUT",
				srv.URL+"/api/v1/files/retroarch/saves/game.srm",
				strings.NewReader(content))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Device-ID", "test-device")
			req.Header.Set("X-SHA256", hash)
			req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

			// Remove the header under test
			req.Header.Del(tt.omitHeader)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandler_PutFile_LongDeviceID(t *testing.T) {
	srv, token := newTestServer(t)

	content := "some-data"
	hash := sha256hex(content)
	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/files/retroarch/saves/game.srm",
		strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Device-ID", strings.Repeat("x", 129))
	req.Header.Set("X-SHA256", hash)
	req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_PutFile_InvalidSHA256(t *testing.T) {
	srv, token := newTestServer(t)

	tests := []struct {
		name string
		hash string
	}{
		{"not hex", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"too short", "abc123"},
		{"too long", strings.Repeat("a", 65)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("PUT",
				srv.URL+"/api/v1/files/retroarch/saves/game.srm",
				strings.NewReader("some-data"))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Device-ID", "test-device")
			req.Header.Set("X-SHA256", tt.hash)
			req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandler_PutFile_HashMismatch(t *testing.T) {
	srv, token := newTestServer(t)

	content := "actual-content"
	wrongHash := sha256hex("different-content")

	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/files/retroarch/saves/game.srm",
		strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Device-ID", "test-device")
	req.Header.Set("X-SHA256", wrongHash)
	req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for hash mismatch, got %d", resp.StatusCode)
	}

	// File must not exist after a hash mismatch
	getReq, _ := http.NewRequest("GET", srv.URL+"/api/v1/files/retroarch/saves/game.srm", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("file should not exist after hash mismatch, got %d", getResp.StatusCode)
	}
}

func TestHandler_PutFile_InvalidTimestamp(t *testing.T) {
	srv, token := newTestServer(t)

	content := "some-data"
	hash := sha256hex(content)
	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/files/retroarch/saves/game.srm",
		strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Device-ID", "test-device")
	req.Header.Set("X-SHA256", hash)
	req.Header.Set("X-Timestamp", "not-a-valid-timestamp")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_PutFile_ConflictReturns409(t *testing.T) {
	srv, token := newTestServer(t)

	// First upload succeeds
	resp1 := putFile(t, srv.URL, token, "retroarch", "saves/game.srm", "version-1", "")
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first PUT expected 200, got %d", resp1.StatusCode)
	}

	// Second upload with a wrong base hash triggers conflict
	wrongBaseHash := sha256hex("does-not-exist")
	resp2 := putFile(t, srv.URL, token, "retroarch", "saves/game.srm", "version-2", wrongBaseHash)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 409, got %d: %s", resp2.StatusCode, body)
	}

	var conflictBody struct {
		Status   string                 `json:"status"`
		Conflict map[string]interface{} `json:"conflict"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&conflictBody); err != nil {
		t.Fatal(err)
	}
	if conflictBody.Status != "conflict" {
		t.Fatalf("expected status conflict, got %q", conflictBody.Status)
	}
	if conflictBody.Conflict == nil {
		t.Fatal("expected conflict object in response")
	}
	if _, ok := conflictBody.Conflict["id"]; !ok {
		t.Fatal("expected conflict to have an id field")
	}
}

func TestHandler_GetFile_NotFound(t *testing.T) {
	srv, token := newTestServer(t)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/files/retroarch/saves/nonexistent.srm", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestHandler_UploadDownloadCycle(t *testing.T) {
	srv, token := newTestServer(t)

	// Binary-ish payload with non-UTF8 bytes
	payload := string([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD, 0x80, 0x90})
	hash := sha256hex(payload)

	req, _ := http.NewRequest("PUT",
		srv.URL+"/api/v1/files/dolphin/saves/game.sav",
		strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Device-ID", "my-device")
	req.Header.Set("X-SHA256", hash)
	ts := time.Now().UTC().Format(time.RFC3339)
	req.Header.Set("X-Timestamp", ts)

	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT expected 200, got %d", putResp.StatusCode)
	}

	// GET it back
	getReq, _ := http.NewRequest("GET", srv.URL+"/api/v1/files/dolphin/saves/game.sav", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", getResp.StatusCode)
	}

	got, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("binary content mismatch: got %x, want %x", got, []byte(payload))
	}

	// Verify response headers
	if getResp.Header.Get("X-SHA256") != hash {
		t.Fatalf("X-SHA256 header mismatch: got %q, want %q", getResp.Header.Get("X-SHA256"), hash)
	}
	if getResp.Header.Get("X-Device-ID") != "my-device" {
		t.Fatalf("X-Device-ID header mismatch: got %q, want %q", getResp.Header.Get("X-Device-ID"), "my-device")
	}
	if getResp.Header.Get("X-Timestamp") == "" {
		t.Fatal("expected X-Timestamp header to be set")
	}
	if getResp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("expected Content-Type application/octet-stream, got %q", getResp.Header.Get("Content-Type"))
	}
}

func TestHandler_ListConflicts_Empty(t *testing.T) {
	srv, token := newTestServer(t)

	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/conflicts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Conflicts []interface{} `json:"conflicts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	// conflicts may be null or empty slice; either is acceptable for "empty"
	if len(body.Conflicts) != 0 {
		t.Fatalf("expected empty conflicts list, got %d", len(body.Conflicts))
	}
}

func TestHandler_ResolveConflict_LocalChoice(t *testing.T) {
	srv, token := newTestServer(t)

	// Step 1: Upload initial version
	resp1 := putFile(t, srv.URL, token, "retroarch", "saves/game.srm", "original-content", "")
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first PUT expected 200, got %d", resp1.StatusCode)
	}

	// Step 2: Upload with wrong base hash to trigger conflict
	wrongHash := sha256hex("wrong-base")
	newContent := "updated-content"
	resp2 := putFile(t, srv.URL, token, "retroarch", "saves/game.srm", newContent, wrongHash)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 409, got %d: %s", resp2.StatusCode, body)
	}

	var conflictResp struct {
		Status   string `json:"status"`
		Conflict struct {
			ID string `json:"id"`
		} `json:"conflict"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&conflictResp); err != nil {
		t.Fatal(err)
	}
	conflictID := conflictResp.Conflict.ID
	if conflictID == "" {
		t.Fatal("expected non-empty conflict ID")
	}

	// Step 3: Verify conflict appears in list
	listReq, _ := http.NewRequest("GET", srv.URL+"/api/v1/conflicts", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	var listBody struct {
		Conflicts []struct {
			ID string `json:"id"`
		} `json:"conflicts"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(listBody.Conflicts))
	}
	if listBody.Conflicts[0].ID != conflictID {
		t.Fatalf("conflict ID mismatch: got %q, want %q", listBody.Conflicts[0].ID, conflictID)
	}

	// Step 4: Resolve with "local" choice
	resolveBody := `{"choice":"local"}`
	resolveReq, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/api/v1/conflicts/%s/resolve", srv.URL, conflictID),
		strings.NewReader(resolveBody))
	resolveReq.Header.Set("Authorization", "Bearer "+token)
	resolveReq.Header.Set("Content-Type", "application/json")
	resolveResp, err := http.DefaultClient.Do(resolveReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resolveResp.Body.Close()

	if resolveResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resolveResp.Body)
		t.Fatalf("resolve expected 200, got %d: %s", resolveResp.StatusCode, body)
	}

	var resolveResult map[string]string
	if err := json.NewDecoder(resolveResp.Body).Decode(&resolveResult); err != nil {
		t.Fatal(err)
	}
	if resolveResult["status"] != "resolved" {
		t.Fatalf("expected status resolved, got %q", resolveResult["status"])
	}

	// Step 5: Verify conflict list is now empty
	listReq2, _ := http.NewRequest("GET", srv.URL+"/api/v1/conflicts", nil)
	listReq2.Header.Set("Authorization", "Bearer "+token)
	listResp2, err := http.DefaultClient.Do(listReq2)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp2.Body.Close()

	var listBody2 struct {
		Conflicts []interface{} `json:"conflicts"`
	}
	if err := json.NewDecoder(listResp2.Body).Decode(&listBody2); err != nil {
		t.Fatal(err)
	}
	if len(listBody2.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts after resolve, got %d", len(listBody2.Conflicts))
	}

	// Step 6: Verify the canonical file is now the local (new) content
	getReq, _ := http.NewRequest("GET", srv.URL+"/api/v1/files/retroarch/saves/game.srm", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()

	got, err := io.ReadAll(getResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newContent {
		t.Fatalf("after local resolve, content should be %q, got %q", newContent, got)
	}
}

func TestHandler_ResolveConflict_InvalidChoice(t *testing.T) {
	srv, token := newTestServer(t)

	resolveBody := `{"choice":"invalid-choice"}`
	req, _ := http.NewRequest("POST",
		srv.URL+"/api/v1/conflicts/some-id/resolve",
		strings.NewReader(resolveBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_ResolveConflict_BadBody(t *testing.T) {
	srv, token := newTestServer(t)

	tests := []struct {
		name string
		body string
	}{
		{"invalid JSON", "not-json-at-all"},
		{"empty choice", `{"choice":""}`},
		{"missing choice field", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST",
				srv.URL+"/api/v1/conflicts/some-id/resolve",
				strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandler_GetHistory(t *testing.T) {
	srv, token := newTestServer(t)

	// Write 3 versions of the same file with small delays to ensure distinct timestamps
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("version-%d", i)
		var baseHash string
		if i > 1 {
			// Use the hash of the previous version as base hash to avoid conflict
			baseHash = sha256hex(fmt.Sprintf("version-%d", i-1))
		}
		resp := putFile(t, srv.URL, token, "retroarch", "saves/game.srm", content, baseHash)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT version %d expected 200, got %d", i, resp.StatusCode)
		}
		// Backups use second-resolution timestamps; must wait for distinct filenames
		time.Sleep(1100 * time.Millisecond)
	}

	// Retrieve history
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/history/retroarch/saves/game.srm", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var historyBody struct {
		Emulator string        `json:"emulator"`
		Path     string        `json:"path"`
		Versions []interface{} `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&historyBody); err != nil {
		t.Fatal(err)
	}

	if historyBody.Emulator != "retroarch" {
		t.Fatalf("expected emulator retroarch, got %q", historyBody.Emulator)
	}
	if historyBody.Path != "saves/game.srm" {
		t.Fatalf("expected path saves/game.srm, got %q", historyBody.Path)
	}
	// Current version + 2 backups = 3 versions
	if len(historyBody.Versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(historyBody.Versions))
	}
}

func TestHandler_AllRoutesRequireAuth(t *testing.T) {
	srv, _ := newTestServer(t)

	routes := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/health", ""},
		{"GET", "/api/v1/manifest/retroarch", ""},
		{"GET", "/api/v1/files/retroarch/saves/game.srm", ""},
		{"PUT", "/api/v1/files/retroarch/saves/game.srm", "data"},
		{"GET", "/api/v1/conflicts", ""},
		{"POST", "/api/v1/conflicts/some-id/resolve", `{"choice":"local"}`},
		{"GET", "/api/v1/history/retroarch/saves/game.srm", ""},
	}

	for _, route := range routes {
		t.Run(fmt.Sprintf("%s %s", route.method, route.path), func(t *testing.T) {
			var bodyReader io.Reader
			if route.body != "" {
				bodyReader = strings.NewReader(route.body)
			}
			req, _ := http.NewRequest(route.method, srv.URL+route.path, bodyReader)
			// Deliberately omit Authorization header
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.StatusCode)
			}
		})
	}
}
