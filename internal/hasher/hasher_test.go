package hasher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile_KnownContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := HashFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestHashFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := HashFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestHashFile_MissingFile(t *testing.T) {
	_, err := HashFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestHashDirectory_Empty(t *testing.T) {
	dir := t.TempDir()

	result, err := HashDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestHashDirectory_NestedFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a/b.txt with "hello\n"
	subdir := filepath.Join(dir, "a")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "b.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create c.txt with empty content
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := HashDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}

	wantHello := "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	wantEmpty := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	key := filepath.Join("a", "b.txt")
	if got, ok := result[key]; !ok {
		t.Errorf("missing key %q", key)
	} else if got != wantHello {
		t.Errorf("hash for %s: got %s, want %s", key, got, wantHello)
	}

	if got, ok := result["c.txt"]; !ok {
		t.Errorf("missing key %q", "c.txt")
	} else if got != wantEmpty {
		t.Errorf("hash for c.txt: got %s, want %s", got, wantEmpty)
	}
}

func TestHashDirectory_NonexistentDir(t *testing.T) {
	_, err := HashDirectory("/nonexistent/directory/path")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}
