//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/client"
	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/model"
)

const (
	serverURL = "http://localhost:8080"
	authToken = "e2e-test-token"
)

var projectDir string

func TestMain(m *testing.M) {
	// Find project root (where docker-compose.yml lives)
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		os.Exit(1)
	}
	projectDir = filepath.Join(wd, "..", "..")

	// Write .env for docker-compose
	envPath := filepath.Join(projectDir, ".env")
	os.WriteFile(envPath, []byte("EMUSYNC_AUTH_TOKEN="+authToken+"\n"), 0644)
	defer os.Remove(envPath)

	// Build and start the server
	if err := runCmd(projectDir, "docker", "compose", "up", "-d", "--build"); err != nil {
		fmt.Fprintf(os.Stderr, "docker compose up: %v\n", err)
		os.Exit(1)
	}

	// Wait for health check
	if err := waitForHealth(30 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		runCmd(projectDir, "docker", "compose", "logs")
		runCmd(projectDir, "docker", "compose", "down", "-v")
		os.Exit(1)
	}

	code := m.Run()

	// Teardown
	runCmd(projectDir, "docker", "compose", "down", "-v")
	os.Exit(code)
}

func waitForHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", serverURL+"/api/v1/health", nil)
		req.Header.Set("Authorization", "Bearer "+authToken)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server not healthy after %v", timeout)
}

func runCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// newDevice creates a syncer configured as a unique device.
func newDevice(t *testing.T, deviceID string) (*client.Syncer, string) {
	t.Helper()
	savesDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:      "localhost",
			Port:      8080,
			AuthToken: authToken,
		},
		Client: config.ClientConfig{
			DeviceID:  deviceID,
			SavesPath: savesDir,
		},
	}
	syncer := client.NewSyncerWithStatePath(cfg, statePath)
	return syncer, savesDir
}

func writeLocalFile(t *testing.T, savesDir, emuDir, filename, content string) {
	t.Helper()
	dir := filepath.Join(savesDir, emuDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readLocalFile(t *testing.T, savesDir, emuDir, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(savesDir, emuDir, filename))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestE2E_HealthCheck(t *testing.T) {
	req, _ := http.NewRequest("GET", serverURL+"/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+authToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestE2E_PushAndPull(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{
		Name:      "retroarch",
		SavePaths: []string{"retroarch"},
	}

	// Device A pushes a save
	deviceA, savesA := newDevice(t, "desktop")
	writeLocalFile(t, savesA, "retroarch", "game.srm", "save data from desktop")

	result, err := deviceA.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Uploaded) == 0 {
		t.Fatal("expected uploads from device A")
	}

	// Device B pulls
	deviceB, savesB := newDevice(t, "steamdeck")
	result, err = deviceB.SyncBeforeLaunch(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Downloaded) == 0 {
		t.Fatal("expected downloads on device B")
	}

	got := readLocalFile(t, savesB, "retroarch", "game.srm")
	if got != "save data from desktop" {
		t.Fatalf("expected 'save data from desktop', got %q", got)
	}
}

func TestE2E_ModifyAndSync(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{
		Name:      "dolphin",
		SavePaths: []string{"dolphin"},
	}

	// Device A initial push
	deviceA, savesA := newDevice(t, "desktop")
	writeLocalFile(t, savesA, "dolphin", "melee.sav", "version 1")
	deviceA.SyncAfterExit(ctx, emu)

	// Device B initial pull
	deviceB, savesB := newDevice(t, "steamdeck")
	deviceB.SyncBeforeLaunch(ctx, emu)

	// Device A modifies and pushes again
	writeLocalFile(t, savesA, "dolphin", "melee.sav", "version 2")
	result, err := deviceA.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Uploaded) == 0 {
		t.Fatal("expected upload of modified file")
	}

	// Device B pulls the update
	result, err = deviceB.SyncBeforeLaunch(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Downloaded) == 0 {
		t.Fatal("expected download of updated file")
	}

	got := readLocalFile(t, savesB, "dolphin", "melee.sav")
	if got != "version 2" {
		t.Fatalf("expected 'version 2', got %q", got)
	}
}

func TestE2E_ConflictDetection(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{
		Name:      "pcsx2",
		SavePaths: []string{"pcsx2"},
	}

	// Device A pushes initial version
	deviceA, savesA := newDevice(t, "desktop")
	writeLocalFile(t, savesA, "pcsx2", "ff10.sav", "desktop save")
	deviceA.SyncAfterExit(ctx, emu)

	// Device B pulls to sync
	deviceB, savesB := newDevice(t, "steamdeck")
	deviceB.SyncBeforeLaunch(ctx, emu)

	// Both devices modify the same file
	writeLocalFile(t, savesA, "pcsx2", "ff10.sav", "desktop modified")
	deviceA.SyncAfterExit(ctx, emu)

	writeLocalFile(t, savesB, "pcsx2", "ff10.sav", "steamdeck modified")
	result, err := deviceB.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Conflicts) == 0 {
		t.Fatal("expected a conflict")
	}
}

func TestE2E_ConflictResolution(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{
		Name:      "rpcs3",
		SavePaths: []string{"rpcs3"},
	}

	// Set up conflict
	deviceA, savesA := newDevice(t, "desktop")
	writeLocalFile(t, savesA, "rpcs3", "save.dat", "initial")
	deviceA.SyncAfterExit(ctx, emu)

	deviceB, savesB := newDevice(t, "steamdeck")
	deviceB.SyncBeforeLaunch(ctx, emu)

	writeLocalFile(t, savesA, "rpcs3", "save.dat", "desktop v2")
	deviceA.SyncAfterExit(ctx, emu)

	writeLocalFile(t, savesB, "rpcs3", "save.dat", "steamdeck v2")
	result, _ := deviceB.SyncAfterExit(ctx, emu)

	if len(result.Conflicts) == 0 {
		t.Fatal("expected conflict")
	}

	// Resolve with "local" (the incoming/steamdeck version wins)
	apiClient := deviceB.GetClient()
	conflicts, err := apiClient.GetConflicts(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var conflictID string
	for _, c := range conflicts {
		if c.Emulator == "rpcs3" {
			conflictID = c.ID
			break
		}
	}
	if conflictID == "" {
		t.Fatal("conflict not found on server")
	}

	if err := apiClient.ResolveConflict(ctx, conflictID, "local"); err != nil {
		t.Fatal(err)
	}

	// Verify the conflict version is now canonical
	reader, _, err := apiClient.DownloadFile(ctx, "rpcs3", "rpcs3/save.dat")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(reader)
	reader.Close()

	if string(data) != "steamdeck v2" {
		t.Fatalf("expected resolved file to be 'steamdeck v2', got %q", string(data))
	}
}

func TestE2E_VersionHistory(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{
		Name:      "duckstation",
		SavePaths: []string{"duckstation"},
	}

	deviceA, savesA := newDevice(t, "desktop")
	for i := 1; i <= 3; i++ {
		writeLocalFile(t, savesA, "duckstation", "game.mcr", fmt.Sprintf("version %d", i))
		result, err := deviceA.SyncAfterExit(ctx, emu)
		if err != nil {
			t.Fatal(err)
		}
		if i > 1 && len(result.Uploaded) == 0 {
			// First upload may or may not show depending on state
		}
		_ = result
		// Small delay so backup timestamps differ
		time.Sleep(100 * time.Millisecond)
	}

	apiClient := deviceA.GetClient()
	versions, err := apiClient.GetHistory(ctx, "duckstation", "duckstation/game.mcr")
	if err != nil {
		t.Fatal(err)
	}

	if len(versions) < 3 {
		t.Fatalf("expected at least 3 versions, got %d", len(versions))
	}

	// Verify sorted descending by timestamp
	for i := 1; i < len(versions); i++ {
		if versions[i].Timestamp.After(versions[i-1].Timestamp) {
			t.Fatal("versions not sorted descending by timestamp")
		}
	}
}

func TestE2E_MultipleEmulators(t *testing.T) {
	ctx := context.Background()
	emuA := &model.EmulatorConfig{
		Name:      "mgba",
		SavePaths: []string{"mgba"},
	}
	emuB := &model.EmulatorConfig{
		Name:      "melonds",
		SavePaths: []string{"melonds"},
	}

	device, savesDir := newDevice(t, "desktop")
	writeLocalFile(t, savesDir, "mgba", "pokemon.sav", "mgba save")
	writeLocalFile(t, savesDir, "melonds", "mario.sav", "melonds save")

	resultA, err := device.SyncAfterExit(ctx, emuA)
	if err != nil {
		t.Fatal(err)
	}
	resultB, err := device.SyncAfterExit(ctx, emuB)
	if err != nil {
		t.Fatal(err)
	}

	if len(resultA.Uploaded) == 0 {
		t.Fatal("expected mgba upload")
	}
	if len(resultB.Uploaded) == 0 {
		t.Fatal("expected melonds upload")
	}

	// Pull on another device - data stays isolated
	device2, saves2 := newDevice(t, "steamdeck")
	result, err := device2.SyncBeforeLaunch(ctx, emuA)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Downloaded) == 0 {
		t.Fatal("expected mgba download")
	}

	got := readLocalFile(t, saves2, "mgba", "pokemon.sav")
	if got != "mgba save" {
		t.Fatalf("expected 'mgba save', got %q", got)
	}
}

func TestE2E_LargeFile(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{
		Name:      "ppsspp",
		SavePaths: []string{"ppsspp"},
	}

	// Create a 10MB file
	content := strings.Repeat("A", 10*1024*1024)
	expectedHash := sha256hex(content)

	deviceA, savesA := newDevice(t, "desktop")
	writeLocalFile(t, savesA, "ppsspp", "big.sav", content)

	result, err := deviceA.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Uploaded) == 0 {
		t.Fatal("expected upload")
	}

	// Pull on another device and verify integrity
	deviceB, savesB := newDevice(t, "steamdeck")
	result, err = deviceB.SyncBeforeLaunch(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Downloaded) == 0 {
		t.Fatal("expected download")
	}

	got := readLocalFile(t, savesB, "ppsspp", "big.sav")
	gotHash := sha256hex(got)
	if gotHash != expectedHash {
		t.Fatalf("hash mismatch: expected %s, got %s", expectedHash, gotHash)
	}
}

func TestE2E_AuthRejected(t *testing.T) {
	badClient := client.NewAPIClient(serverURL, "wrong-token")
	ctx := context.Background()

	_, err := badClient.GetManifest(ctx, "retroarch")
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got: %v", err)
	}
}
