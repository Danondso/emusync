package setup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dublin/emusync/internal/setup"
)

func TestFindSaveRoots_prefersEmuDeckLayout(t *testing.T) {
	home := t.TempDir()
	em := filepath.Join(home, "Emulation", "saves")
	if err := os.MkdirAll(filepath.Join(em, "retroarch", "saves"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(em, "rpcs3"), 0o755); err != nil {
		t.Fatal(err)
	}

	roots := setup.FindSaveRoots(home)
	if len(roots) == 0 {
		t.Fatal("expected at least one candidate")
	}
	if roots[0].Path != em {
		t.Fatalf("first candidate = %q want %q", roots[0].Path, em)
	}
	if roots[0].Reason == "" {
		t.Fatal("expected reason")
	}
}

func TestFindSaveRoots_scoresKnownEmulatorDirs(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(home, "my-saves")
	if err := os.MkdirAll(filepath.Join(custom, "retroarch", "saves"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(custom, "dolphin", "GC"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(custom, "pcsx2"), 0o755); err != nil {
		t.Fatal(err)
	}

	roots := setup.FindSaveRoots(home)
	found := false
	for _, r := range roots {
		if r.Path == custom && r.Score >= 30 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom layout scored: %+v", roots)
	}
}

func TestFindSaveRoots_sortedByScore(t *testing.T) {
	home := t.TempDir()
	weak := filepath.Join(home, "weak")
	if err := os.MkdirAll(filepath.Join(weak, "retroarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	strong := filepath.Join(home, "Emulation", "saves")
	if err := os.MkdirAll(filepath.Join(strong, "retroarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(strong, "rpcs3"), 0o755); err != nil {
		t.Fatal(err)
	}

	roots := setup.FindSaveRoots(home)
	if len(roots) < 2 {
		t.Fatalf("expected multiple roots, got %+v", roots)
	}
	if roots[0].Score < roots[1].Score {
		t.Fatalf("expected descending score: %+v", roots)
	}
}

func TestShortenHome(t *testing.T) {
	home := filepath.FromSlash("/home/u")
	abs := filepath.FromSlash("/home/u/a/b")
	got := setup.ShortenHome(abs, home)
	want := filepath.Join("~", "a", "b")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	outside := filepath.FromSlash("/other/x")
	if setup.ShortenHome(outside, home) != filepath.Clean(outside) {
		t.Fatal("outside home should stay absolute")
	}
}
