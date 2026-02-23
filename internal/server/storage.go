package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dublin/emusync/internal/model"
)

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// Storage manages server-side save file persistence.
type Storage struct {
	dataDir    string
	maxBackups int
	mu         sync.RWMutex
}

// NewStorage creates a new storage instance rooted at dataDir.
func NewStorage(dataDir string, maxBackups int) *Storage {
	return &Storage{
		dataDir:    dataDir,
		maxBackups: maxBackups,
	}
}

func (s *Storage) canonicalDir(emulator string) string {
	return filepath.Join(s.dataDir, "canonical", emulator)
}

func (s *Storage) backupDir(emulator string) string {
	return filepath.Join(s.dataDir, "backups", emulator)
}

func (s *Storage) metadataDir(emulator string) string {
	return filepath.Join(s.dataDir, "metadata", emulator)
}

func (s *Storage) conflictDir() string {
	return filepath.Join(s.dataDir, "conflicts")
}

// validatePath checks that resolved is within s.dataDir.
func (s *Storage) validatePath(resolved string) error {
	clean := filepath.Clean(resolved)
	base := filepath.Clean(s.dataDir) + string(filepath.Separator)
	if !strings.HasPrefix(clean+string(filepath.Separator), base) && clean != filepath.Clean(s.dataDir) {
		return fmt.Errorf("invalid path: outside data directory")
	}
	return nil
}

// ValidateName checks that a name component (emulator, conflict ID) is safe.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q: must match [a-zA-Z0-9._-]+", name)
	}
	return nil
}

func (s *Storage) metadataPath(emulator, filePath string) string {
	return filepath.Join(s.metadataDir(emulator), filePath+".json")
}

// GetManifest returns the current file manifest for an emulator.
func (s *Storage) GetManifest(emulator string) (*model.Manifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	manifest := &model.Manifest{
		Emulator:  emulator,
		Files:     make(map[string]model.FileEntry),
		UpdatedAt: time.Time{},
	}

	metaDir := s.metadataDir(emulator)
	if err := s.validatePath(metaDir); err != nil {
		return nil, err
	}
	if _, err := os.Stat(metaDir); os.IsNotExist(err) {
		return manifest, nil
	}

	err := filepath.Walk(metaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		entry, err := s.readMetadata(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(metaDir, path)
		if err != nil {
			return err
		}
		// Strip .json suffix to get the original file path
		filePath := strings.TrimSuffix(rel, ".json")

		manifest.Files[filePath] = *entry
		if entry.Timestamp.After(manifest.UpdatedAt) {
			manifest.UpdatedAt = entry.Timestamp
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking metadata for %s: %w", emulator, err)
	}

	return manifest, nil
}

// ReadFile returns a reader for the canonical version of a file.
func (s *Storage) ReadFile(emulator, filePath string) (io.ReadCloser, *model.FileEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	canonPath := filepath.Join(s.canonicalDir(emulator), filePath)
	if err := s.validatePath(canonPath); err != nil {
		return nil, nil, err
	}
	f, err := os.Open(canonPath)
	if err != nil {
		return nil, nil, err
	}

	metaPath := s.metadataPath(emulator, filePath)
	if err := s.validatePath(metaPath); err != nil {
		f.Close()
		return nil, nil, err
	}
	entry, err := s.readMetadata(metaPath)
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	return f, entry, nil
}

// WriteFile stores a file. If baseHash doesn't match the current canonical hash,
// a conflict is returned and the file is stored as a conflict candidate.
func (s *Storage) WriteFile(emulator, filePath string, r io.Reader, meta model.FileEntry, baseHash string) (*model.Conflict, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate paths before any filesystem access
	canonPath := filepath.Join(s.canonicalDir(emulator), filePath)
	if err := s.validatePath(canonPath); err != nil {
		return nil, err
	}
	metaP := s.metadataPath(emulator, filePath)
	if err := s.validatePath(metaP); err != nil {
		return nil, err
	}

	// Check for conflict
	existing, err := s.readMetadata(metaP)
	if err == nil && baseHash != "" && existing.SHA256 != baseHash {
		// Conflict detected: server has a different version than what client last synced
		conflict := &model.Conflict{
			ID:         fmt.Sprintf("%s-%s-%d", emulator, strings.ReplaceAll(filePath, "/", "-"), time.Now().UnixNano()),
			Emulator:   emulator,
			Path:       filePath,
			Local:      meta,
			Remote:     *existing,
			DetectedAt: time.Now().UTC(),
		}

		// Store the incoming file as a conflict candidate
		if err := s.writeConflictFile(conflict, r); err != nil {
			return nil, fmt.Errorf("storing conflict file: %w", err)
		}

		// Save conflict metadata
		if err := s.saveConflict(conflict); err != nil {
			return nil, fmt.Errorf("saving conflict metadata: %w", err)
		}

		return conflict, nil
	}

	// No conflict -- backup the existing file (if any), then write the new one
	if _, statErr := os.Stat(canonPath); statErr == nil {
		if err := s.createBackup(emulator, filePath, existing); err != nil {
			return nil, fmt.Errorf("creating backup: %w", err)
		}
	}

	// Write new canonical file atomically
	if err := s.atomicWrite(canonPath, r); err != nil {
		return nil, fmt.Errorf("writing canonical file: %w", err)
	}

	// Write metadata
	if err := s.writeMetadata(metaP, &meta); err != nil {
		return nil, fmt.Errorf("writing metadata: %w", err)
	}

	return nil, nil
}

// GetHistory returns version history for a file.
func (s *Storage) GetHistory(emulator, filePath string) ([]model.VersionEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var versions []model.VersionEntry

	metaPath := s.metadataPath(emulator, filePath)
	if err := s.validatePath(metaPath); err != nil {
		return nil, err
	}

	// Add current version
	current, err := s.readMetadata(metaPath)
	if err == nil {
		versions = append(versions, model.VersionEntry{
			SHA256:    current.SHA256,
			Size:      current.Size,
			Timestamp: current.Timestamp,
			DeviceID:  current.DeviceID,
		})
	}

	// Scan backup metadata
	backupMetaDir := filepath.Join(s.backupDir(emulator), filePath+".versions")
	if entries, err := os.ReadDir(backupMetaDir); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			entry, err := s.readMetadata(filepath.Join(backupMetaDir, e.Name()))
			if err != nil {
				continue
			}
			versions = append(versions, model.VersionEntry{
				SHA256:    entry.SHA256,
				Size:      entry.Size,
				Timestamp: entry.Timestamp,
				DeviceID:  entry.DeviceID,
			})
		}
	}

	// Sort by timestamp descending
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Timestamp.After(versions[j].Timestamp)
	})

	return versions, nil
}

// ListConflicts returns all unresolved conflicts.
func (s *Storage) ListConflicts() ([]model.Conflict, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var conflicts []model.Conflict
	conflictMetaDir := filepath.Join(s.conflictDir(), "meta")
	if _, err := os.Stat(conflictMetaDir); os.IsNotExist(err) {
		return conflicts, nil
	}

	entries, err := os.ReadDir(conflictMetaDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(conflictMetaDir, e.Name()))
		if err != nil {
			continue
		}
		var c model.Conflict
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}
		if !c.Resolved {
			conflicts = append(conflicts, c)
		}
	}
	return conflicts, nil
}

// ResolveConflict resolves a conflict by the given choice.
func (s *Storage) ResolveConflict(id string, choice string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load conflict metadata
	metaPath := filepath.Join(s.conflictDir(), "meta", id+".json")
	if err := s.validatePath(metaPath); err != nil {
		return err
	}
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("conflict %s not found: %w", id, err)
	}
	var conflict model.Conflict
	if err := json.Unmarshal(data, &conflict); err != nil {
		return err
	}

	conflictFilePath := filepath.Join(s.conflictDir(), "files", id)

	switch choice {
	case "local":
		// The incoming (conflicting) file becomes canonical
		existing, _ := s.readMetadata(s.metadataPath(conflict.Emulator, conflict.Path))
		if existing != nil {
			if err := s.createBackup(conflict.Emulator, conflict.Path, existing); err != nil {
				return err
			}
		}
		canonPath := filepath.Join(s.canonicalDir(conflict.Emulator), conflict.Path)
		src, err := os.Open(conflictFilePath)
		if err != nil {
			return err
		}
		defer src.Close()
		if err := s.atomicWrite(canonPath, src); err != nil {
			return err
		}
		if err := s.writeMetadata(s.metadataPath(conflict.Emulator, conflict.Path), &conflict.Local); err != nil {
			return err
		}

	case "remote":
		// Current canonical stays, conflict file is just backed up
		// Nothing to do for canonical; just clean up the conflict file

	case "keep-both":
		// Store the conflict file alongside canonical with a device-id suffix
		ext := filepath.Ext(conflict.Path)
		base := strings.TrimSuffix(conflict.Path, ext)
		newPath := fmt.Sprintf("%s.%s%s", base, conflict.Local.DeviceID, ext)
		destPath := filepath.Join(s.canonicalDir(conflict.Emulator), newPath)
		src, err := os.Open(conflictFilePath)
		if err != nil {
			return err
		}
		defer src.Close()
		if err := s.atomicWrite(destPath, src); err != nil {
			return err
		}
		if err := s.writeMetadata(s.metadataPath(conflict.Emulator, newPath), &conflict.Local); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid choice: %s (must be local, remote, or keep-both)", choice)
	}

	// Mark conflict as resolved
	conflict.Resolved = true
	resolvedData, err := json.MarshalIndent(conflict, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling resolved conflict: %w", err)
	}
	if err := atomicWriteBytes(metaPath, resolvedData); err != nil {
		return fmt.Errorf("writing resolved conflict metadata: %w", err)
	}

	// Clean up conflict file
	if err := os.Remove(conflictFilePath); err != nil && !os.IsNotExist(err) {
		slog.Warn("removing conflict file", "path", conflictFilePath, "error", err)
	}

	return nil
}

// --- Internal helpers ---

func (s *Storage) readMetadata(path string) (*model.FileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entry model.FileEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *Storage) writeMetadata(path string, entry *model.FileEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return atomicWriteBytes(path, data)
}

func (s *Storage) atomicWrite(destPath string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	tmp := destPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, destPath)
}

func atomicWriteBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Storage) createBackup(emulator, filePath string, meta *model.FileEntry) error {
	canonPath := filepath.Join(s.canonicalDir(emulator), filePath)
	timestamp := time.Now().UTC().Format("20060102-150405")

	// Copy the file
	backupFile := filepath.Join(s.backupDir(emulator), filePath+".versions", timestamp+".bak")
	if err := os.MkdirAll(filepath.Dir(backupFile), 0755); err != nil {
		return err
	}

	src, err := os.Open(canonPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := s.atomicWrite(backupFile, src); err != nil {
		return err
	}

	// Save backup metadata
	if meta != nil {
		metaFile := filepath.Join(s.backupDir(emulator), filePath+".versions", timestamp+".json")
		if err := s.writeMetadata(metaFile, meta); err != nil {
			return fmt.Errorf("writing backup metadata: %w", err)
		}
	}

	// Rotate old backups
	s.rotateBackups(filepath.Join(s.backupDir(emulator), filePath+".versions"))

	return nil
}

func (s *Storage) rotateBackups(versionDir string) {
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		return
	}

	// Collect .bak files
	var bakFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".bak") {
			bakFiles = append(bakFiles, e.Name())
		}
	}

	sort.Strings(bakFiles)

	// Remove oldest if over limit
	for len(bakFiles) > s.maxBackups {
		oldest := bakFiles[0]
		bakFiles = bakFiles[1:]
		os.Remove(filepath.Join(versionDir, oldest))
		// Also remove corresponding .json
		jsonFile := strings.TrimSuffix(oldest, ".bak") + ".json"
		os.Remove(filepath.Join(versionDir, jsonFile))
	}
}

func (s *Storage) writeConflictFile(conflict *model.Conflict, r io.Reader) error {
	path := filepath.Join(s.conflictDir(), "files", conflict.ID)
	return s.atomicWrite(path, r)
}

func (s *Storage) saveConflict(conflict *model.Conflict) error {
	path := filepath.Join(s.conflictDir(), "meta", conflict.ID+".json")
	data, err := json.MarshalIndent(conflict, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return atomicWriteBytes(path, data)
}
