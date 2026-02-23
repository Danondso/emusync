package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/hasher"
	"github.com/dublin/emusync/internal/model"
)

// Syncer orchestrates save file syncing between client and server.
type Syncer struct {
	cfg    *config.Config
	client *APIClient
	state  *SyncState
}

// SyncState tracks last-synced hashes per file.
type SyncState struct {
	path  string
	Files map[string]map[string]string `json:"files"` // emulator -> relPath -> sha256
}

// NewSyncer creates a new syncer.
func NewSyncer(cfg *config.Config) (*Syncer, error) {
	statePath, err := config.DefaultStatePath()
	if err != nil {
		return nil, fmt.Errorf("determining state path: %w", err)
	}
	return NewSyncerWithStatePath(cfg, statePath), nil
}

// NewSyncerWithStatePath creates a new syncer with a custom state file path.
func NewSyncerWithStatePath(cfg *config.Config, statePath string) *Syncer {
	client := NewAPIClient(cfg.Server.BaseURL(), cfg.Server.AuthToken)
	state := loadState(statePath)
	return &Syncer{
		cfg:    cfg,
		client: client,
		state:  state,
	}
}

// SyncAfterExit syncs saves after an emulator exits (upload changed files).
func (s *Syncer) SyncAfterExit(ctx context.Context, emu *model.EmulatorConfig) (*model.SyncResult, error) {
	slog.Info("sync after exit", "emulator", emu.Name)
	result := &model.SyncResult{Emulator: emu.Name}

	for _, savePath := range emu.SavePaths {
		fullPath := s.cfg.ResolveSavePath(savePath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			slog.Debug("save path does not exist, skipping", "path", fullPath)
			continue
		}

		// Hash local files
		localHashes, err := hasher.HashDirectory(fullPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("hashing %s: %w", fullPath, err))
			continue
		}

		// Get server manifest
		manifest, err := s.client.GetManifest(ctx, emu.Name)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("getting manifest for %s: %w", emu.Name, err))
			continue
		}

		// Upload files that have changed locally
		for relPath, localHash := range localHashes {
			serverKey := filepath.Join(savePath, relPath)
			serverEntry, existsOnServer := manifest.Files[serverKey]

			if existsOnServer && serverEntry.SHA256 == localHash {
				continue // File unchanged
			}

			// Get the base hash (what we last synced)
			baseHash := s.getBaseHash(emu.Name, serverKey)

			absPath := filepath.Join(fullPath, relPath)
			info, err := os.Stat(absPath)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}

			f, err := os.Open(absPath)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}

			meta := model.FileEntry{
				SHA256:    localHash,
				Size:      info.Size(),
				Timestamp: info.ModTime().UTC(),
				DeviceID:  s.cfg.Client.DeviceID,
			}

			conflict, err := s.client.UploadFile(ctx, emu.Name, serverKey, f, meta, baseHash)
			f.Close()

			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("uploading %s: %w", serverKey, err))
				continue
			}

			if conflict != nil {
				slog.Warn("conflict detected", "emulator", emu.Name, "path", serverKey)
				result.Conflicts = append(result.Conflicts, *conflict)
			} else {
				slog.Info("uploaded", "emulator", emu.Name, "path", serverKey)
				result.Uploaded = append(result.Uploaded, serverKey)
				s.setBaseHash(emu.Name, serverKey, localHash)
			}
		}
	}

	s.saveState()
	return result, nil
}

// SyncBeforeLaunch pulls latest saves from server before an emulator starts.
func (s *Syncer) SyncBeforeLaunch(ctx context.Context, emu *model.EmulatorConfig) (*model.SyncResult, error) {
	slog.Info("sync before launch", "emulator", emu.Name)
	result := &model.SyncResult{Emulator: emu.Name}

	// Get server manifest
	manifest, err := s.client.GetManifest(ctx, emu.Name)
	if err != nil {
		return result, fmt.Errorf("getting manifest for %s: %w", emu.Name, err)
	}

	for _, savePath := range emu.SavePaths {
		fullPath := s.cfg.ResolveSavePath(savePath)

		// Hash local files (if directory exists)
		localHashes := make(map[string]string)
		if _, err := os.Stat(fullPath); err == nil {
			localHashes, err = hasher.HashDirectory(fullPath)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("hashing %s: %w", fullPath, err))
				continue
			}
		}

		// Download files that are newer on server
		for serverKey, serverEntry := range manifest.Files {
			// Only process files belonging to this save path
			rel, err := filepath.Rel(savePath, serverKey)
			if err != nil || rel == ".." || len(rel) > 0 && rel[0] == '.' {
				continue
			}

			// Validate resolved path stays within save directory
			destPath := filepath.Clean(filepath.Join(fullPath, rel))
			cleanBase := filepath.Clean(fullPath) + string(os.PathSeparator)
			if !strings.HasPrefix(destPath+string(os.PathSeparator), cleanBase) {
				slog.Warn("path traversal attempt in download, skipping", "path", rel)
				continue
			}

			localHash, existsLocally := localHashes[rel]
			if existsLocally && localHash == serverEntry.SHA256 {
				continue // File unchanged
			}

			// Download from server
			reader, _, err := s.client.DownloadFile(ctx, emu.Name, serverKey)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("downloading %s: %w", serverKey, err))
				continue
			}

			if err := atomicWriteFile(destPath, reader); err != nil {
				reader.Close()
				result.Errors = append(result.Errors, fmt.Errorf("writing %s: %w", destPath, err))
				continue
			}
			reader.Close()

			slog.Info("downloaded", "emulator", emu.Name, "path", serverKey)
			result.Downloaded = append(result.Downloaded, serverKey)
			s.setBaseHash(emu.Name, serverKey, serverEntry.SHA256)
		}
	}

	s.saveState()
	return result, nil
}

// PushAll uploads all saves for the given emulators.
func (s *Syncer) PushAll(ctx context.Context, emulators []model.EmulatorConfig) (*model.SyncResult, error) {
	combined := &model.SyncResult{}
	for i := range emulators {
		result, err := s.SyncAfterExit(ctx, &emulators[i])
		if err != nil {
			combined.Errors = append(combined.Errors, err)
			continue
		}
		combined.Uploaded = append(combined.Uploaded, result.Uploaded...)
		combined.Conflicts = append(combined.Conflicts, result.Conflicts...)
		combined.Errors = append(combined.Errors, result.Errors...)
	}
	return combined, nil
}

// PullAll downloads latest saves for the given emulators.
func (s *Syncer) PullAll(ctx context.Context, emulators []model.EmulatorConfig) (*model.SyncResult, error) {
	combined := &model.SyncResult{}
	for i := range emulators {
		result, err := s.SyncBeforeLaunch(ctx, &emulators[i])
		if err != nil {
			combined.Errors = append(combined.Errors, err)
			continue
		}
		combined.Downloaded = append(combined.Downloaded, result.Downloaded...)
		combined.Conflicts = append(combined.Conflicts, result.Conflicts...)
		combined.Errors = append(combined.Errors, result.Errors...)
	}
	return combined, nil
}

// GetClient returns the underlying API client (for direct use by CLI commands).
func (s *Syncer) GetClient() *APIClient {
	return s.client
}

// --- State management ---

func (s *Syncer) getBaseHash(emulator, path string) string {
	if s.state.Files == nil {
		return ""
	}
	if m, ok := s.state.Files[emulator]; ok {
		return m[path]
	}
	return ""
}

func (s *Syncer) setBaseHash(emulator, path, hash string) {
	if s.state.Files == nil {
		s.state.Files = make(map[string]map[string]string)
	}
	if s.state.Files[emulator] == nil {
		s.state.Files[emulator] = make(map[string]string)
	}
	s.state.Files[emulator][path] = hash
}

func (s *Syncer) saveState() {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		slog.Error("marshaling state", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.state.path), 0755); err != nil {
		slog.Error("creating state directory", "error", err)
		return
	}
	tmp := s.state.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		slog.Error("writing state", "error", err)
		return
	}
	if err := os.Rename(tmp, s.state.path); err != nil {
		slog.Error("renaming state file", "error", err)
	}
}

func loadState(path string) *SyncState {
	state := &SyncState{
		path:  path,
		Files: make(map[string]map[string]string),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, state); err != nil {
		slog.Warn("parsing state file, starting fresh", "path", path, "error", err)
	}
	state.path = path
	return state
}

// --- Helpers ---

func atomicWriteFile(destPath string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	tmp := destPath + ".emusync.tmp"
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

// Status returns the sync status for an emulator.
func (s *Syncer) Status(ctx context.Context, emu *model.EmulatorConfig) (changed []string, conflicts []model.Conflict, err error) {
	manifest, err := s.client.GetManifest(ctx, emu.Name)
	if err != nil {
		return nil, nil, err
	}

	for _, savePath := range emu.SavePaths {
		fullPath := s.cfg.ResolveSavePath(savePath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			continue
		}

		localHashes, err := hasher.HashDirectory(fullPath)
		if err != nil {
			continue
		}

		for relPath, localHash := range localHashes {
			serverKey := filepath.Join(savePath, relPath)
			if serverEntry, ok := manifest.Files[serverKey]; ok {
				if serverEntry.SHA256 != localHash {
					changed = append(changed, serverKey)
				}
			} else {
				changed = append(changed, serverKey+" (new)")
			}
		}

		// Check for files on server but not local
		for serverKey := range manifest.Files {
			rel, err := filepath.Rel(savePath, serverKey)
			if err != nil || rel == ".." || len(rel) > 0 && rel[0] == '.' {
				continue
			}
			destPath := filepath.Clean(filepath.Join(fullPath, rel))
			cleanBase := filepath.Clean(fullPath) + string(os.PathSeparator)
			if !strings.HasPrefix(destPath+string(os.PathSeparator), cleanBase) {
				continue
			}
			if _, ok := localHashes[rel]; !ok {
				changed = append(changed, serverKey+" (remote only)")
			}
		}
	}

	// Also check for unresolved conflicts
	serverConflicts, _ := s.client.GetConflicts(ctx)
	for _, c := range serverConflicts {
		if c.Emulator == emu.Name {
			conflicts = append(conflicts, c)
		}
	}

	return changed, conflicts, nil
}
