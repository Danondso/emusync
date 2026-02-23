package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestManifestJSONRoundTrip(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	original := Manifest{
		Emulator: "retroarch",
		Files: map[string]FileEntry{
			"saves/game.sav": {
				SHA256:    "abc123",
				Size:      1024,
				Timestamp: ts,
				DeviceID:  "deck",
			},
		},
		UpdatedAt: ts,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Emulator != original.Emulator {
		t.Errorf("Emulator = %q, want %q", decoded.Emulator, original.Emulator)
	}
	if !decoded.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", decoded.UpdatedAt, original.UpdatedAt)
	}
	if len(decoded.Files) != len(original.Files) {
		t.Fatalf("Files length = %d, want %d", len(decoded.Files), len(original.Files))
	}

	entry, ok := decoded.Files["saves/game.sav"]
	if !ok {
		t.Fatal("missing key saves/game.sav")
	}
	if entry.SHA256 != "abc123" {
		t.Errorf("SHA256 = %q, want %q", entry.SHA256, "abc123")
	}
	if entry.Size != 1024 {
		t.Errorf("Size = %d, want %d", entry.Size, 1024)
	}
	if entry.DeviceID != "deck" {
		t.Errorf("DeviceID = %q, want %q", entry.DeviceID, "deck")
	}
}

func TestConflictJSONRoundTrip(t *testing.T) {
	ts := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	original := Conflict{
		ID:       "conflict-001",
		Emulator: "dolphin",
		Path:     "GC/save.gci",
		Local: FileEntry{
			SHA256:    "localsha",
			Size:      512,
			Timestamp: ts,
			DeviceID:  "deck",
		},
		Remote: FileEntry{
			SHA256:    "remotesha",
			Size:      768,
			Timestamp: ts.Add(time.Hour),
			DeviceID:  "desktop",
		},
		DetectedAt: ts,
		Resolved:   false,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Conflict
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Emulator != original.Emulator {
		t.Errorf("Emulator = %q, want %q", decoded.Emulator, original.Emulator)
	}
	if decoded.Path != original.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, original.Path)
	}
	if decoded.Local.SHA256 != original.Local.SHA256 {
		t.Errorf("Local.SHA256 = %q, want %q", decoded.Local.SHA256, original.Local.SHA256)
	}
	if decoded.Remote.DeviceID != original.Remote.DeviceID {
		t.Errorf("Remote.DeviceID = %q, want %q", decoded.Remote.DeviceID, original.Remote.DeviceID)
	}
	if !decoded.DetectedAt.Equal(original.DetectedAt) {
		t.Errorf("DetectedAt = %v, want %v", decoded.DetectedAt, original.DetectedAt)
	}
	if decoded.Resolved != original.Resolved {
		t.Errorf("Resolved = %v, want %v", decoded.Resolved, original.Resolved)
	}
}
