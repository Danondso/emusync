package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/model"
)

// ProfileDocument is the JSON stored under data/admin/profile.json and returned by the admin API.
type ProfileDocument struct {
	Version   int                    `json:"version"`
	Emulators []model.EmulatorConfig `json:"emulators"`
}

func (s *Storage) profilePath() string {
	return filepath.Join(s.dataDir, "admin", "profile.json")
}

// ReadProfile loads the admin profile or returns defaults when missing.
func (s *Storage) ReadProfile() (*ProfileDocument, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.profilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &ProfileDocument{
				Version:   1,
				Emulators: append([]model.EmulatorConfig(nil), config.DefaultEmulators()...),
			}, nil
		}
		return nil, err
	}
	var doc ProfileDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode profile: %w", err)
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("unsupported profile version %d", doc.Version)
	}
	return &doc, nil
}

// WriteProfile replaces the admin profile atomically.
func (s *Storage) WriteProfile(doc *ProfileDocument) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if doc == nil {
		return fmt.Errorf("profile is nil")
	}
	if doc.Version != 1 {
		return fmt.Errorf("unsupported profile version %d", doc.Version)
	}
	dir := filepath.Dir(s.profilePath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.profilePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.profilePath())
}
