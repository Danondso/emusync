package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/model"
)

// --- helpers ---

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func writeFile(t *testing.T, s *Storage, emulator, path, content, baseHash string) *model.Conflict {
	t.Helper()
	hash := sha256hex(content)
	meta := model.FileEntry{
		SHA256:    hash,
		Size:      int64(len(content)),
		Timestamp: time.Now().UTC(),
		DeviceID:  "test-device",
	}
	conflict, err := s.WriteFile(emulator, path, strings.NewReader(content), meta, baseHash, hash)
	if err != nil {
		t.Fatal(err)
	}
	return conflict
}

func readAll(t *testing.T, s *Storage, emulator, path string) string {
	t.Helper()
	rc, _, err := s.ReadFile(emulator, path)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// --- tests ---

func TestGetManifest_Empty(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	m, err := s.GetManifest("snes")
	if err != nil {
		t.Fatal(err)
	}
	if m.Emulator != "snes" {
		t.Fatalf("expected emulator %q, got %q", "snes", m.Emulator)
	}
	if len(m.Files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(m.Files))
	}
}

func TestGetManifest_AfterWrite(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "saves/game.sav", "save-data-1", "")

	m, err := s.GetManifest("gba")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(m.Files))
	}
	entry, ok := m.Files["saves/game.sav"]
	if !ok {
		t.Fatal("expected saves/game.sav in manifest")
	}
	if entry.SHA256 != sha256hex("save-data-1") {
		t.Fatalf("SHA256 mismatch: got %s", entry.SHA256)
	}
}

func TestGetManifest_MultipleFiles(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "saves/a.sav", "aaa", "")
	writeFile(t, s, "gba", "saves/b.sav", "bbb", "")
	writeFile(t, s, "gba", "roms/c.sav", "ccc", "")

	m, err := s.GetManifest("gba")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(m.Files))
	}
	for _, key := range []string{"saves/a.sav", "saves/b.sav", "roms/c.sav"} {
		if _, ok := m.Files[key]; !ok {
			t.Fatalf("missing %s in manifest", key)
		}
	}
}

func TestReadFile_Exists(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)
	content := "hello world"

	writeFile(t, s, "nes", "save.sav", content, "")

	rc, entry, err := s.ReadFile("nes", "save.sav")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch: got %q", string(data))
	}
	if entry.SHA256 != sha256hex(content) {
		t.Fatalf("metadata SHA256 mismatch")
	}
	if entry.Size != int64(len(content)) {
		t.Fatalf("metadata size mismatch: got %d", entry.Size)
	}
	if entry.DeviceID != "test-device" {
		t.Fatalf("metadata device ID mismatch: got %q", entry.DeviceID)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	_, _, err := s.ReadFile("nes", "no-such-file.sav")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteFile_FirstWrite(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, 5)

	conflict := writeFile(t, s, "gba", "saves/game.sav", "first-write", "")
	if conflict != nil {
		t.Fatal("expected no conflict on first write")
	}

	got := readAll(t, s, "gba", "saves/game.sav")
	if got != "first-write" {
		t.Fatalf("expected %q, got %q", "first-write", got)
	}

	// Verify no .tmp files remain
	var tmpFiles []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".tmp") {
			tmpFiles = append(tmpFiles, path)
		}
		return nil
	})
	if len(tmpFiles) > 0 {
		t.Fatalf("orphaned .tmp files found: %v", tmpFiles)
	}
}

func TestWriteFile_UpdateNoConflict(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, 5)

	writeFile(t, s, "gba", "game.sav", "version-1", "")
	baseHash := sha256hex("version-1")

	conflict := writeFile(t, s, "gba", "game.sav", "version-2", baseHash)
	if conflict != nil {
		t.Fatal("expected no conflict with correct baseHash")
	}

	got := readAll(t, s, "gba", "game.sav")
	if got != "version-2" {
		t.Fatalf("expected %q, got %q", "version-2", got)
	}

	// Verify a backup was created
	backupDir := filepath.Join(dir, "backups", "gba", "game.sav.versions")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("reading backup dir: %v", err)
	}
	bakCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			bakCount++
		}
	}
	if bakCount != 1 {
		t.Fatalf("expected 1 backup, got %d", bakCount)
	}
}

func TestWriteFile_ConflictDetected(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "server-version", "")

	wrongBase := sha256hex("client-thinks-this-was-the-original")
	conflict := writeFile(t, s, "gba", "game.sav", "client-version", wrongBase)
	if conflict == nil {
		t.Fatal("expected conflict with wrong baseHash")
	}

	if conflict.Emulator != "gba" {
		t.Fatalf("conflict emulator mismatch: got %q", conflict.Emulator)
	}
	if conflict.Path != "game.sav" {
		t.Fatalf("conflict path mismatch: got %q", conflict.Path)
	}

	// Canonical should remain unchanged
	got := readAll(t, s, "gba", "game.sav")
	if got != "server-version" {
		t.Fatalf("canonical should be unchanged, got %q", got)
	}
}

func TestWriteFile_EmptyBaseHashNeverConflicts(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "version-1", "")

	// Write with empty baseHash even though the hash differs
	conflict := writeFile(t, s, "gba", "game.sav", "version-2-force", "")
	if conflict != nil {
		t.Fatal("expected no conflict with empty baseHash")
	}

	got := readAll(t, s, "gba", "game.sav")
	if got != "version-2-force" {
		t.Fatalf("expected %q, got %q", "version-2-force", got)
	}
}

func TestBackupRotation(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, 3)

	// First write (no backup created)
	writeFile(t, s, "gba", "game.sav", "v0", "")

	// Writes 1-4 each create a backup of the previous version.
	// We need distinct timestamps for the backup filenames.
	for i := 1; i <= 4; i++ {
		prev := readAll(t, s, "gba", "game.sav")
		baseHash := sha256hex(prev)
		time.Sleep(1100 * time.Millisecond) // backups use second-resolution timestamps
		content := string(rune('A'+i)) + "-content"
		writeFile(t, s, "gba", "game.sav", content, baseHash)
	}

	// Count .bak files
	backupDir := filepath.Join(dir, "backups", "gba", "game.sav.versions")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	bakCount := 0
	jsonCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			bakCount++
		}
		if strings.HasSuffix(e.Name(), ".json") {
			jsonCount++
		}
	}
	if bakCount != 3 {
		t.Fatalf("expected 3 .bak files after rotation, got %d", bakCount)
	}
	if jsonCount != 3 {
		t.Fatalf("expected 3 .json files after rotation, got %d", jsonCount)
	}
}

func TestListConflicts_Empty(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	conflicts, err := s.ListConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d", len(conflicts))
	}
}

func TestListConflicts_WithConflict(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "server-v", "")
	writeFile(t, s, "gba", "game.sav", "client-v", sha256hex("old-hash"))

	conflicts, err := s.ListConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].Emulator != "gba" {
		t.Fatalf("conflict emulator: got %q", conflicts[0].Emulator)
	}
	if conflicts[0].Resolved {
		t.Fatal("conflict should not be resolved")
	}
}

func TestListConflicts_SkipsResolved(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "server-v", "")
	conflict := writeFile(t, s, "gba", "game.sav", "client-v", sha256hex("old-hash"))
	if conflict == nil {
		t.Fatal("expected conflict")
	}

	if err := s.ResolveConflict(conflict.ID, "remote"); err != nil {
		t.Fatal(err)
	}

	conflicts, err := s.ListConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 unresolved conflicts, got %d", len(conflicts))
	}
}

func TestResolveConflict_Local(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "server-version", "")
	conflict := writeFile(t, s, "gba", "game.sav", "incoming-version", sha256hex("wrong-base"))
	if conflict == nil {
		t.Fatal("expected conflict")
	}

	if err := s.ResolveConflict(conflict.ID, "local"); err != nil {
		t.Fatal(err)
	}

	// Incoming should now be canonical
	got := readAll(t, s, "gba", "game.sav")
	if got != "incoming-version" {
		t.Fatalf("expected canonical to be %q, got %q", "incoming-version", got)
	}

	// Old canonical should be backed up
	_, entry, err := s.ReadFile("gba", "game.sav")
	if err != nil {
		t.Fatal(err)
	}
	if entry.SHA256 != sha256hex("incoming-version") {
		t.Fatalf("metadata should reflect incoming version")
	}
}

func TestResolveConflict_Remote(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "server-version", "")
	conflict := writeFile(t, s, "gba", "game.sav", "incoming-version", sha256hex("wrong-base"))
	if conflict == nil {
		t.Fatal("expected conflict")
	}

	if err := s.ResolveConflict(conflict.ID, "remote"); err != nil {
		t.Fatal(err)
	}

	// Canonical stays unchanged
	got := readAll(t, s, "gba", "game.sav")
	if got != "server-version" {
		t.Fatalf("expected canonical to remain %q, got %q", "server-version", got)
	}
}

func TestResolveConflict_KeepBoth(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, 5)

	writeFile(t, s, "gba", "game.sav", "server-version", "")
	conflict := writeFile(t, s, "gba", "game.sav", "incoming-version", sha256hex("wrong-base"))
	if conflict == nil {
		t.Fatal("expected conflict")
	}

	if err := s.ResolveConflict(conflict.ID, "keep-both"); err != nil {
		t.Fatal(err)
	}

	// Original canonical remains
	got := readAll(t, s, "gba", "game.sav")
	if got != "server-version" {
		t.Fatalf("expected canonical %q, got %q", "server-version", got)
	}

	// The conflict file should be saved with device-id suffix: game.test-device.sav
	keepBothPath := filepath.Join(dir, "canonical", "gba", "game.test-device.sav")
	data, err := os.ReadFile(keepBothPath)
	if err != nil {
		t.Fatalf("expected keep-both file at %s: %v", keepBothPath, err)
	}
	if string(data) != "incoming-version" {
		t.Fatalf("keep-both content mismatch: got %q", string(data))
	}
}

func TestResolveConflict_InvalidChoice(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "server-version", "")
	conflict := writeFile(t, s, "gba", "game.sav", "incoming-version", sha256hex("wrong-base"))
	if conflict == nil {
		t.Fatal("expected conflict")
	}

	err := s.ResolveConflict(conflict.ID, "bad-choice")
	if err == nil {
		t.Fatal("expected error for invalid choice")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Fatalf("expected 'invalid choice' in error, got: %v", err)
	}
}

func TestResolveConflict_NotFound(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	err := s.ResolveConflict("nonexistent-id", "local")
	if err == nil {
		t.Fatal("expected error for nonexistent conflict")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

func TestGetHistory_SingleVersion(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "only-version", "")

	history, err := s.GetHistory("gba", "game.sav")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 version, got %d", len(history))
	}
	if history[0].SHA256 != sha256hex("only-version") {
		t.Fatalf("hash mismatch")
	}
}

func TestGetHistory_MultipleVersions(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	writeFile(t, s, "gba", "game.sav", "v1", "")
	time.Sleep(1100 * time.Millisecond) // backups use second-resolution timestamps
	writeFile(t, s, "gba", "game.sav", "v2", sha256hex("v1"))
	time.Sleep(1100 * time.Millisecond)
	writeFile(t, s, "gba", "game.sav", "v3", sha256hex("v2"))

	history, err := s.GetHistory("gba", "game.sav")
	if err != nil {
		t.Fatal(err)
	}

	// Current version + 2 backups = 3 entries
	if len(history) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(history))
	}

	// Should be sorted descending by timestamp
	for i := 0; i < len(history)-1; i++ {
		if history[i].Timestamp.Before(history[i+1].Timestamp) {
			t.Fatalf("history not sorted descending: index %d (%v) before index %d (%v)",
				i, history[i].Timestamp, i+1, history[i+1].Timestamp)
		}
	}

	// Most recent should be v3
	if history[0].SHA256 != sha256hex("v3") {
		t.Fatalf("most recent version should be v3, got hash %s", history[0].SHA256)
	}
}

func TestStorage_PathTraversalPrevented(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	traversalPaths := []string{
		"../other-emulator/game.sav",
		"../../canonical/other-emulator/game.sav",
		"subdir/../../other-emulator/game.sav",
	}

	for _, path := range traversalPaths {
		t.Run(path, func(t *testing.T) {
			_, _, err := s.ReadFile("retroarch", path)
			if err == nil {
				t.Fatalf("expected error for path traversal attempt %q", path)
			}
		})
	}
}

func TestStorage_HashMismatchRejected(t *testing.T) {
	s := NewStorage(t.TempDir(), 5)

	hash := sha256hex("actual-content")
	meta := model.FileEntry{
		SHA256:    hash,
		Size:      int64(len("actual-content")),
		Timestamp: time.Now().UTC(),
		DeviceID:  "test-device",
	}

	// Declare a hash that does not match the content
	wrongHash := sha256hex("different-content")
	_, err := s.WriteFile("gba", "game.sav", strings.NewReader("actual-content"), meta, "", wrongHash)
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("expected ErrHashMismatch, got: %v", err)
	}

	// Canonical file must not have been created
	_, _, readErr := s.ReadFile("gba", "game.sav")
	if readErr == nil {
		t.Fatal("canonical file should not exist after hash mismatch")
	}
}

func TestStorage_ConcurrentWrites(t *testing.T) {
	s := NewStorage(t.TempDir(), 10)

	var wg sync.WaitGroup
	errs := make(chan error, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			content := strings.Repeat("x", n+1)
			hash := sha256hex(content)
			meta := model.FileEntry{
				SHA256:    hash,
				Size:      int64(len(content)),
				Timestamp: time.Now().UTC(),
				DeviceID:  "device",
			}
			// Each goroutine writes to its own file to avoid logical conflicts
			// but exercises the shared mutex.
			path := "save" + string(rune('0'+n)) + ".sav"
			_, err := s.WriteFile("emu", path, strings.NewReader(content), meta, "", hash)
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent write error: %v", err)
	}

	// Verify all 8 files are in the manifest
	m, err := s.GetManifest("emu")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Files) != 8 {
		t.Fatalf("expected 8 files after concurrent writes, got %d", len(m.Files))
	}
}
