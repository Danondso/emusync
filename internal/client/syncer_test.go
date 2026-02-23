package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/model"
	"github.com/dublin/emusync/internal/server"
)

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func newTestSyncer(t *testing.T, serverURL string, savesDir string) *Syncer {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "state.json")
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "unused", Port: 0, AuthToken: "test-token"},
		Client: config.ClientConfig{DeviceID: "test-device", SavesPath: savesDir},
	}
	return &Syncer{
		cfg:    cfg,
		client: NewAPIClient(serverURL, "test-token"),
		state:  loadState(statePath),
	}
}

func newFullTestStack(t *testing.T) (*httptest.Server, *Syncer, string) {
	t.Helper()
	storage := server.NewStorage(t.TempDir(), 10)
	handlers := server.NewHandlers(storage)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)
	srv := httptest.NewServer(server.AuthMiddleware("test-token", mux))
	t.Cleanup(srv.Close)

	savesDir := t.TempDir()
	syncer := newTestSyncer(t, srv.URL, savesDir)
	return srv, syncer, savesDir
}

func uploadToServer(t *testing.T, client *APIClient, emulator, path, content string) {
	t.Helper()
	hash := sha256hex(content)
	meta := model.FileEntry{
		SHA256:    hash,
		Size:      int64(len(content)),
		Timestamp: time.Now().UTC(),
		DeviceID:  "other-device",
	}
	_, err := client.UploadFile(context.Background(), emulator, path, strings.NewReader(content), meta, "")
	if err != nil {
		t.Fatal(err)
	}
}

func writeLocalFile(t *testing.T, savesDir, emulator, filename, content string) {
	t.Helper()
	dir := filepath.Join(savesDir, emulator)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncer_SyncAfterExit_UploadsNew(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", "save-data-1")

	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	result, err := syncer.SyncAfterExit(context.Background(), emu)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(result.Uploaded))
	}
	if !strings.Contains(result.Uploaded[0], "slot1.srm") {
		t.Errorf("uploaded path = %q, expected it to contain slot1.srm", result.Uploaded[0])
	}
}

func TestSyncer_SyncAfterExit_SkipsUnchanged(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", "save-data-1")

	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	// First sync uploads
	result, err := syncer.SyncAfterExit(context.Background(), emu)
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	if len(result.Uploaded) != 1 {
		t.Fatalf("first sync: expected 1 upload, got %d", len(result.Uploaded))
	}

	// Second sync without changes should not upload
	result, err = syncer.SyncAfterExit(context.Background(), emu)
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}
	if len(result.Uploaded) != 0 {
		t.Errorf("second sync: expected 0 uploads, got %d: %v", len(result.Uploaded), result.Uploaded)
	}
}

func TestSyncer_SyncBeforeLaunch_Downloads(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	// Upload a file to the server from another device
	uploadToServer(t, syncer.client, "retroarch", "retroarch/slot1.srm", "remote-save-data")

	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	result, err := syncer.SyncBeforeLaunch(context.Background(), emu)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Downloaded) != 1 {
		t.Fatalf("expected 1 download, got %d", len(result.Downloaded))
	}

	// Verify the file was written locally
	localPath := filepath.Join(savesDir, "retroarch", "slot1.srm")
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("file not found locally: %v", err)
	}
	if string(data) != "remote-save-data" {
		t.Errorf("file content = %q, want %q", string(data), "remote-save-data")
	}
}

func TestSyncer_SyncBeforeLaunch_SkipsMatching(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	content := "identical-save-data"

	// Upload to server
	uploadToServer(t, syncer.client, "retroarch", "retroarch/slot1.srm", content)

	// Write the same content locally
	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", content)

	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	result, err := syncer.SyncBeforeLaunch(context.Background(), emu)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Downloaded) != 0 {
		t.Errorf("expected 0 downloads when files match, got %d: %v", len(result.Downloaded), result.Downloaded)
	}
}

func TestSyncer_PushAll_MultipleEmulators(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", "ra-save")
	writeLocalFile(t, savesDir, "dolphin", "game.gci", "dolphin-save")

	emulators := []model.EmulatorConfig{
		{Name: "retroarch", SavePaths: []string{"retroarch"}},
		{Name: "dolphin", SavePaths: []string{"dolphin"}},
	}

	result, err := syncer.PushAll(context.Background(), emulators)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Uploaded) != 2 {
		t.Fatalf("expected 2 uploads, got %d: %v", len(result.Uploaded), result.Uploaded)
	}

	// Verify both emulators had files uploaded
	hasRA := false
	hasDolphin := false
	for _, path := range result.Uploaded {
		if strings.Contains(path, "retroarch") {
			hasRA = true
		}
		if strings.Contains(path, "dolphin") {
			hasDolphin = true
		}
	}
	if !hasRA {
		t.Error("missing retroarch upload")
	}
	if !hasDolphin {
		t.Error("missing dolphin upload")
	}
}

func TestSyncer_PullAll_MultipleEmulators(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	uploadToServer(t, syncer.client, "retroarch", "retroarch/slot1.srm", "ra-save")
	uploadToServer(t, syncer.client, "dolphin", "dolphin/game.gci", "dolphin-save")

	emulators := []model.EmulatorConfig{
		{Name: "retroarch", SavePaths: []string{"retroarch"}},
		{Name: "dolphin", SavePaths: []string{"dolphin"}},
	}

	result, err := syncer.PullAll(context.Background(), emulators)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Downloaded) != 2 {
		t.Fatalf("expected 2 downloads, got %d: %v", len(result.Downloaded), result.Downloaded)
	}

	// Verify files exist locally
	raData, err := os.ReadFile(filepath.Join(savesDir, "retroarch", "slot1.srm"))
	if err != nil {
		t.Fatalf("retroarch file not found: %v", err)
	}
	if string(raData) != "ra-save" {
		t.Errorf("retroarch content = %q, want %q", string(raData), "ra-save")
	}

	dolphinData, err := os.ReadFile(filepath.Join(savesDir, "dolphin", "game.gci"))
	if err != nil {
		t.Fatalf("dolphin file not found: %v", err)
	}
	if string(dolphinData) != "dolphin-save" {
		t.Errorf("dolphin content = %q, want %q", string(dolphinData), "dolphin-save")
	}
}

func TestSyncer_Status_DetectsChanges(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	// Upload and sync a file first
	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", "original-data")

	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	_, err := syncer.SyncAfterExit(context.Background(), emu)
	if err != nil {
		t.Fatalf("initial sync error: %v", err)
	}

	// Modify the local file
	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", "modified-data")

	changed, _, err := syncer.Status(context.Background(), emu)
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if len(changed) == 0 {
		t.Fatal("expected at least 1 changed file, got 0")
	}

	found := false
	for _, c := range changed {
		if strings.Contains(c, "slot1.srm") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("changed list %v does not contain slot1.srm", changed)
	}
}

func TestSyncer_StatePersistedAcrossRestarts(t *testing.T) {
	_, syncer, savesDir := newFullTestStack(t)

	writeLocalFile(t, savesDir, "retroarch", "slot1.srm", "persistent-data")

	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	// Sync to populate state
	result, err := syncer.SyncAfterExit(context.Background(), emu)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(result.Uploaded) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(result.Uploaded))
	}

	// Read the persisted state file to verify it was written
	statePath := syncer.state.path
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state file not found: %v", err)
	}

	var persisted SyncState
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshaling state: %v", err)
	}

	// Create a new syncer using the same state file
	newSyncer := &Syncer{
		cfg:    syncer.cfg,
		client: syncer.client,
		state:  loadState(statePath),
	}

	// The base hash should be preserved
	serverKey := result.Uploaded[0]
	baseHash := newSyncer.getBaseHash("retroarch", serverKey)
	expectedHash := sha256hex("persistent-data")
	if baseHash != expectedHash {
		t.Errorf("base hash after restart = %q, want %q", baseHash, expectedHash)
	}
}
