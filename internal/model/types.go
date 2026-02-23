package model

import "time"

// Manifest is returned by GET /api/v1/manifest/{emulator}.
type Manifest struct {
	Emulator  string               `json:"emulator"`
	Files     map[string]FileEntry `json:"files"`
	UpdatedAt time.Time            `json:"updated_at"`
}

// FileEntry describes a single save file.
type FileEntry struct {
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
	DeviceID  string    `json:"device_id"`
}

// Conflict represents a detected sync conflict.
type Conflict struct {
	ID         string    `json:"id"`
	Emulator   string    `json:"emulator"`
	Path       string    `json:"path"`
	Local      FileEntry `json:"local"`
	Remote     FileEntry `json:"remote"`
	DetectedAt time.Time `json:"detected_at"`
	Resolved   bool      `json:"resolved"`
}

// SyncResult summarizes a sync operation.
type SyncResult struct {
	Emulator   string
	Uploaded   []string
	Downloaded []string
	Conflicts  []Conflict
	Errors     []error
}

// VersionEntry is one item in a file's version history.
type VersionEntry struct {
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
	DeviceID  string    `json:"device_id"`
}
