//go:build e2e

// Package e2e runs Docker Compose integration tests against the API as published on localhost.
// Do not use t.Parallel() here: auth and HTTP endpoints are configured once in TestMain.
//
// Optional env for TestMain: EMUSYNC_E2E_PORT, EMUSYNC_E2E_MAX_BACKUPS (default 3; passed to the
// container as EMUSYNC_MAX_BACKUPS for backup-rotation coverage).
package e2e

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/client"
	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/model"
)

const defaultE2EPort = 8080

var (
	projectDir string

	// Set in TestMain before any test runs.
	e2eAuthToken  string
	e2eAdminToken string
	e2eHTTPPort   int
	e2eBaseURL    string
	e2eEmusyncBin string
)

func TestMain(m *testing.M) {
	os.Exit(runE2EMain(m))
}

func runE2EMain(m *testing.M) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		return 1
	}
	projectDir = filepath.Join(wd, "..", "..")

	if err := resolveE2EHTTPPort(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	e2eBaseURL = fmt.Sprintf("http://localhost:%d", e2eHTTPPort)

	envPath := filepath.Join(projectDir, ".env")

	binDir, err := os.MkdirTemp("", "emusync-e2e-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdir temp bin: %v\n", err)
		return 1
	}
	e2eEmusyncBin = filepath.Join(binDir, "emusync")

	teardown := func() {
		logTeardownErr("compose down", runCmd(projectDir, "docker", "compose", "down", "-v"))
		if err := os.Remove(envPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			logTeardownErr("remove .env", err)
		}
		logTeardownErr("remove e2e bin dir", os.RemoveAll(binDir))
	}
	defer teardown()

	tokenBytes := make([]byte, 24)
	if _, err := crand.Read(tokenBytes); err != nil {
		fmt.Fprintf(os.Stderr, "e2e auth token: %v\n", err)
		return 1
	}
	e2eAuthToken = hex.EncodeToString(tokenBytes)
	e2eAdminToken = "e2e-admin-token-fixed"

	// Restrictive perms; root .env is listed in .gitignore — never commit real secrets here.
	maxBackups := strings.TrimSpace(os.Getenv("EMUSYNC_E2E_MAX_BACKUPS"))
	if maxBackups == "" {
		maxBackups = "3"
	}
	envBody := "EMUSYNC_AUTH_TOKEN=" + e2eAuthToken + "\n" +
		"EMUSYNC_ADMIN_TOKEN=" + e2eAdminToken + "\n" +
		"EMUSYNC_MAX_BACKUPS=" + maxBackups + "\n"
	if err := os.WriteFile(envPath, []byte(envBody), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "write .env: %v\n", err)
		return 1
	}

	if err := runCmd(projectDir, "docker", "compose", "up", "-d", "--build"); err != nil {
		fmt.Fprintf(os.Stderr, "docker compose up: %v\n", err)
		return 1
	}

	if err := waitForHealth(30 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		_ = runCmd(projectDir, "docker", "compose", "logs")
		return 1
	}

	if err := runCmd(projectDir, "go", "build", "-o", e2eEmusyncBin, "."); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build emusync cli: %v\n", err)
		return 1
	}

	return m.Run()
}

func resolveE2EHTTPPort() error {
	e2eHTTPPort = defaultE2EPort
	p := strings.TrimSpace(os.Getenv("EMUSYNC_E2E_PORT"))
	if p == "" {
		return nil
	}
	v, err := strconv.Atoi(p)
	if err != nil || v <= 0 || v > 65535 {
		return fmt.Errorf("invalid EMUSYNC_E2E_PORT %q (want 1–65535)", p)
	}
	e2eHTTPPort = v
	return nil
}

func logTeardownErr(label string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e teardown [%s]: %v\n", label, err)
	}
}

func waitForHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest("GET", e2eBaseURL+"/api/v1/health", nil)
		if err != nil {
			return fmt.Errorf("health request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+e2eAuthToken)
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
			Port:      e2eHTTPPort,
			AuthToken: e2eAuthToken,
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
	dest := filepath.Join(savesDir, emuDir, filename)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(content), 0644); err != nil {
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
	req, err := http.NewRequest("GET", e2eBaseURL+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e2eAuthToken)
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

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
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
	result, err := deviceB.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

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
		// Backups use second-resolution timestamps (see Storage.createBackup)
		time.Sleep(1100 * time.Millisecond)
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

	// Pull on another device — data stays isolated
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

	melonPath := filepath.Join(saves2, "melonds", "mario.sav")
	if _, err := os.Stat(melonPath); err == nil {
		t.Fatalf("expected melonds save to be absent on device pulled for mgba only, found %s", melonPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat melonds: %v", err)
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
	badClient := client.NewAPIClient(e2eBaseURL, "wrong-token")
	ctx := context.Background()

	_, err := badClient.GetManifest(ctx, "retroarch")
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got: %v", err)
	}
}

func TestE2E_AdminProfileGET(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, e2eBaseURL+"/admin/api/v1/profile", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+e2eAdminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("admin profile HTTP %s: %s", res.Status, string(body))
	}
	if !strings.Contains(string(body), `"version"`) {
		t.Fatalf("unexpected body: %s", string(body))
	}
}

func e2eAuthorizedDo(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()
	if req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+e2eAuthToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, b
}

func writeE2ECLIConfig(t *testing.T, home, savesRoot, deviceID string, emulators []model.EmulatorConfig) string {
	t.Helper()
	cfgDir := filepath.Join(home, ".config", "emusync")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfgDir, "config.toml")
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:      "localhost",
			Port:      e2eHTTPPort,
			AuthToken: e2eAuthToken,
		},
		Client: config.ClientConfig{
			DeviceID:  deviceID,
			SavesPath: savesRoot,
		},
		Sync: config.SyncConfig{
			PollIntervalMs:   200,
			PostExitDelayMs:  100,
			AutoSyncOnLaunch: true,
			AutoSyncOnClose:  true,
			ConflictStrategy: "prompt",
		},
		Emulators: emulators,
	}
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestE2E_HealthRequiresAuthWhenTokenSet(t *testing.T) {
	resp, err := http.Get(e2eBaseURL + "/api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}
}

func TestE2E_HTTPGetFileNotFound(t *testing.T) {
	req, _ := http.NewRequest("GET", e2eBaseURL+"/api/v1/files/retroarch/retroarch/does-not-exist.srm", nil)
	code, _ := e2eAuthorizedDo(t, req)
	if code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", code)
	}
}

func TestE2E_HTTPInvalidEmulatorName(t *testing.T) {
	req, _ := http.NewRequest("GET", e2eBaseURL+"/api/v1/manifest/bad!name", nil)
	code, _ := e2eAuthorizedDo(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

func TestE2E_HTTPEmptyFilePath(t *testing.T) {
	req, _ := http.NewRequest("GET", e2eBaseURL+"/api/v1/files/retroarch/", nil)
	code, _ := e2eAuthorizedDo(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

func TestE2E_HTTPPutMissingHeaders(t *testing.T) {
	req, err := http.NewRequest("PUT", e2eBaseURL+"/api/v1/files/retroarch/retroarch/x.dat", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-SHA256", sha256hex("x"))
	req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))
	// Deliberately omit X-Device-ID
	code, _ := e2eAuthorizedDo(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

func TestE2E_HTTPPutBadTimestamp(t *testing.T) {
	req, err := http.NewRequest("PUT", e2eBaseURL+"/api/v1/files/retroarch/retroarch/badts.dat", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Device-ID", "d")
	req.Header.Set("X-SHA256", sha256hex("x"))
	req.Header.Set("X-Timestamp", "not-rfc3339")
	code, _ := e2eAuthorizedDo(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", code)
	}
}

func TestE2E_HTTPPutHashMismatch(t *testing.T) {
	req, err := http.NewRequest("PUT", e2eBaseURL+"/api/v1/files/retroarch/retroarch/hashmiss.dat", strings.NewReader("actual-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Device-ID", "d")
	req.Header.Set("X-SHA256", "0000000000000000000000000000000000000000000000000000000000000000")
	req.Header.Set("X-Timestamp", time.Now().UTC().Format(time.RFC3339))
	code, body := e2eAuthorizedDo(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", code, body)
	}
}

func TestE2E_EmptyManifestForNewEmulator(t *testing.T) {
	ctx := context.Background()
	api := client.NewAPIClient(e2eBaseURL, e2eAuthToken)
	mf, err := api.GetManifest(ctx, "e2eemptyemu")
	if err != nil {
		t.Fatal(err)
	}
	if mf == nil || len(mf.Files) != 0 {
		t.Fatalf("expected empty manifest files, got %v", mf)
	}
}

func TestE2E_HTTPResolveConflictBadChoice(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2ebadj", SavePaths: []string{"e2ebadj"}}
	deviceA, savesA := newDevice(t, "a")
	writeLocalFile(t, savesA, "e2ebadj", "c.dat", "one")
	if _, err := deviceA.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}
	deviceB, savesB := newDevice(t, "b")
	if _, err := deviceB.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}
	writeLocalFile(t, savesA, "e2ebadj", "c.dat", "two")
	if _, err := deviceA.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}
	writeLocalFile(t, savesB, "e2ebadj", "c.dat", "three")
	result, err := deviceB.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %v", result.Conflicts)
	}
	id := result.Conflicts[0].ID

	req, err := http.NewRequest("POST", e2eBaseURL+"/api/v1/conflicts/"+id+"/resolve", bytes.NewReader([]byte(`{"choice":"nope"}`)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	code, _ := e2eAuthorizedDo(t, req)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 bad choice, got %d", code)
	}
}

func TestE2E_ConflictResolveRemote(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2eremote", SavePaths: []string{"e2eremote"}}
	deviceA, savesA := newDevice(t, "desk")
	writeLocalFile(t, savesA, "e2eremote", "x.dat", "version-1")
	if _, err := deviceA.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}

	deviceB, savesB := newDevice(t, "deck")
	if _, err := deviceB.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}

	writeLocalFile(t, savesA, "e2eremote", "x.dat", "version-2-desktop")
	if _, err := deviceA.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}

	writeLocalFile(t, savesB, "e2eremote", "x.dat", "deck-edit")
	result, err := deviceB.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %+v", result.Conflicts)
	}

	api := deviceB.GetClient()
	if err := api.ResolveConflict(ctx, result.Conflicts[0].ID, "remote"); err != nil {
		t.Fatal(err)
	}

	r, _, err := api.DownloadFile(ctx, "e2eremote", "e2eremote/x.dat")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "version-2-desktop" {
		t.Fatalf("canonical after remote: got %q", string(b))
	}
}

func TestE2E_ConflictResolveKeepBoth(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2eboth", SavePaths: []string{"e2eboth"}}
	deviceA, savesA := newDevice(t, "desk")
	writeLocalFile(t, savesA, "e2eboth", "y.dat", "A1")
	if _, err := deviceA.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}

	deviceB, savesB := newDevice(t, "deckk")
	if _, err := deviceB.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}

	writeLocalFile(t, savesA, "e2eboth", "y.dat", "A2")
	if _, err := deviceA.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}

	writeLocalFile(t, savesB, "e2eboth", "y.dat", "BINCOMING")
	result, err := deviceB.SyncAfterExit(ctx, emu)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected conflict, got %+v", result.Conflicts)
	}

	api := deviceB.GetClient()
	cid := result.Conflicts[0].ID
	if err := api.ResolveConflict(ctx, cid, "keep-both"); err != nil {
		t.Fatal(err)
	}

	conflicts, err := api.GetConflicts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range conflicts {
		if c.Emulator == "e2eboth" {
			t.Fatalf("unexpected listed conflict for e2eboth: %+v", c)
		}
	}

	mf, err := api.GetManifest(ctx, "e2eboth")
	if err != nil {
		t.Fatal(err)
	}
	sideKey := "e2eboth/y.deckk.dat"
	if _, ok := mf.Files["e2eboth/y.dat"]; !ok {
		t.Fatal("missing canonical key")
	}
	if _, ok := mf.Files[sideKey]; !ok {
		t.Fatalf("missing keep-both key %q in manifest", sideKey)
	}

	r1, _, err := api.DownloadFile(ctx, "e2eboth", "e2eboth/y.dat")
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := io.ReadAll(r1)
	r1.Close()
	if string(b1) != "A2" {
		t.Fatalf("canonical: %q", string(b1))
	}

	r2, _, err := api.DownloadFile(ctx, "e2eboth", sideKey)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(r2)
	r2.Close()
	if string(b2) != "BINCOMING" {
		t.Fatalf("side file: %q", string(b2))
	}
}

func TestE2E_NestedSavePath(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2enested", SavePaths: []string{"e2enested"}}
	a, dirA := newDevice(t, "n1")
	writeLocalFile(t, dirA, "e2enested", filepath.Join("deep", "slot.sav"), "nested-data")
	if _, err := a.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}
	b, dirB := newDevice(t, "n2")
	if _, err := b.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}
	got := readLocalFile(t, dirB, "e2enested", filepath.Join("deep", "slot.sav"))
	if got != "nested-data" {
		t.Fatalf("got %q", got)
	}
}

func TestE2E_DualSavePaths(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2edual", SavePaths: []string{"rootA", "rootB"}}
	a, dirA := newDevice(t, "d1")
	writeLocalFile(t, dirA, "rootA", "fa.sav", "AAA")
	writeLocalFile(t, dirA, "rootB", "fb.sav", "BBB")
	if _, err := a.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}
	b, dirB := newDevice(t, "d2")
	if _, err := b.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}
	if readLocalFile(t, dirB, "rootA", "fa.sav") != "AAA" {
		t.Fatal("rootA")
	}
	if readLocalFile(t, dirB, "rootB", "fb.sav") != "BBB" {
		t.Fatal("rootB")
	}
}

func TestE2E_ColdPullCreatesDirectories(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2ecold", SavePaths: []string{"e2ecold"}}
	a, dirA := newDevice(t, "c1")
	writeLocalFile(t, dirA, "e2ecold", "only.srm", "cold")
	if _, err := a.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: e2eHTTPPort, AuthToken: e2eAuthToken},
		Client: config.ClientConfig{DeviceID: "c2", SavesPath: t.TempDir()},
	}
	syncer := client.NewSyncerWithStatePath(cfg, filepath.Join(t.TempDir(), "state.json"))
	if _, err := syncer.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(cfg.Client.SavesPath, "e2ecold", "only.srm")
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "cold" {
		t.Fatalf("got %q", string(b))
	}
}

func TestE2E_PathWithSpaceInFilename(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2espace", SavePaths: []string{"e2espace"}}
	a, dirA := newDevice(t, "s1")
	writeLocalFile(t, dirA, "e2espace", filepath.Join("sub", "spaced name.srm"), "SPACE")
	if _, err := a.SyncAfterExit(ctx, emu); err != nil {
		t.Fatal(err)
	}
	b, dirB := newDevice(t, "s2")
	if _, err := b.SyncBeforeLaunch(ctx, emu); err != nil {
		t.Fatal(err)
	}
	got := readLocalFile(t, dirB, "e2espace", filepath.Join("sub", "spaced name.srm"))
	if got != "SPACE" {
		t.Fatalf("got %q", got)
	}
}

func TestE2E_MaxBackupRotation(t *testing.T) {
	ctx := context.Background()
	emu := &model.EmulatorConfig{Name: "e2erotate", SavePaths: []string{"e2erotate"}}
	a, dirA := newDevice(t, "rot")
	api := a.GetClient()
	for i := range 8 {
		writeLocalFile(t, dirA, "e2erotate", "h.dat", fmt.Sprintf("v%d", i))
		if _, err := a.SyncAfterExit(ctx, emu); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1100 * time.Millisecond)
	}
	vers, err := api.GetHistory(ctx, "e2erotate", "e2erotate/h.dat")
	if err != nil {
		t.Fatal(err)
	}
	if want := 4; len(vers) != want {
		t.Fatalf("expected exactly %d versions after rotation, got %d", want, len(vers))
	}
}

func TestE2E_CLIPushPullStatusHistory(t *testing.T) {
	if e2eEmusyncBin == "" {
		t.Fatal("e2e binary not built")
	}
	ctx := context.Background()
	home1 := t.TempDir()
	saves1 := filepath.Join(home1, "EmuSaves")
	emu := model.EmulatorConfig{Name: "e2ecli", SavePaths: []string{"e2ecli"}, ProcessNames: []string{"e2ecli"}}
	cfg1 := writeE2ECLIConfig(t, home1, saves1, "cli-a", []model.EmulatorConfig{emu})
	if err := os.MkdirAll(filepath.Join(saves1, "e2ecli"), 0755); err != nil {
		t.Fatal(err)
	}
	gamePath := filepath.Join(saves1, "e2ecli", "game.sav")
	if err := os.WriteFile(gamePath, []byte("cli-upload"), 0644); err != nil {
		t.Fatal(err)
	}

	runCli := func(home string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, e2eEmusyncBin, args...)
		cmd.Env = append(os.Environ(), "HOME="+home)
		return cmd.CombinedOutput()
	}

	out, err := runCli(home1, "push", "e2ecli", "--config", cfg1)
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}

	home2 := t.TempDir()
	saves2 := filepath.Join(home2, "EmuSaves")
	cfg2 := writeE2ECLIConfig(t, home2, saves2, "cli-b", []model.EmulatorConfig{emu})

	out, err = runCli(home2, "pull", "e2ecli", "--config", cfg2)
	if err != nil {
		t.Fatalf("pull: %v\n%s", err, out)
	}
	pulled := filepath.Join(saves2, "e2ecli", "game.sav")
	body, err := os.ReadFile(pulled)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "cli-upload" {
		t.Fatalf("pulled content %q", string(body))
	}

	out, err = runCli(home2, "status", "e2ecli", "--config", cfg2)
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}

	out, err = runCli(home1, "history", "e2ecli", "e2ecli/game.sav", "--config", cfg1)
	if err != nil {
		t.Fatalf("history: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Version history") {
		t.Fatalf("history output: %s", out)
	}
}

func TestE2E_WatchStarts(t *testing.T) {
	if e2eEmusyncBin == "" {
		t.Fatal("e2e binary not built")
	}
	home := t.TempDir()
	saves := filepath.Join(home, "s")
	emu := model.EmulatorConfig{Name: "e2ewatch", SavePaths: []string{"e2ewatch"}, ProcessNames: []string{"nonexistent-proc"}}
	cfg := writeE2ECLIConfig(t, home, saves, "watcher", []model.EmulatorConfig{emu})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, e2eEmusyncBin, "watch", "--config", cfg)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		// Command killed by timeout — watch should have printed.
		if !strings.Contains(string(out), "Watching") {
			t.Fatalf("expected watch banner, got: %s", out)
		}
		return
	}
	if err != nil && !strings.Contains(string(out), "Watching") {
		t.Fatalf("watch: %v\n%s", err, out)
	}
}
