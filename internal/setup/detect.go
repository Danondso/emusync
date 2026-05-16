package setup

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dublin/emusync/internal/config"
)

// RootCandidate is a filesystem directory that may contain emulator saves.
type RootCandidate struct {
	Path   string
	Reason string
	Score  int
}

var (
	emulatorRootMarkersOnce sync.Once
	emulatorRootMarkersList []string
)

// emulatorRootMarkers returns sorted emulator directory names used when scoring
// a save root. Only immediate child directories whose lowercased name equals a
// marker (exact match) contribute to the score; names like "retroarch-saves" do not.
func emulatorRootMarkers() []string {
	emulatorRootMarkersOnce.Do(func() {
		seen := map[string]struct{}{}
		for _, e := range config.DefaultEmulators() {
			name := strings.ToLower(strings.TrimSpace(e.Name))
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			emulatorRootMarkersList = append(emulatorRootMarkersList, name)
		}
		sort.Strings(emulatorRootMarkersList)
	})
	return emulatorRootMarkersList
}

// FindSaveRoots returns likely save roots under home, highest scoring first.
func FindSaveRoots(home string) []RootCandidate {
	var out []RootCandidate
	if home == "" {
		return out
	}

	try := func(abs, reason string, baseScore int) {
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return
		}
		score := baseScore + scoreChildMarkers(abs)
		out = append(out, RootCandidate{
			Path:   abs,
			Reason: reason,
			Score:  score,
		})
	}

	try(filepath.Join(home, "Emulation", "saves"), "EmuDeck-style ~/Emulation/saves", 40)

	entries, err := os.ReadDir(home)
	if err != nil {
		sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
		return dedupeRoots(out)
	}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		direct := filepath.Join(home, name)
		try(direct, "directory under home", 5)
		if strings.EqualFold(name, "Games") {
			try(filepath.Join(direct, "Emulation", "saves"), "nested Games/Emulation/saves", 35)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Path < out[j].Path
	})
	return dedupeRoots(out)
}

// ShortenHome returns path as ~/rel when abs is under home; otherwise abs.
func ShortenHome(abs, home string) string {
	abs = filepath.Clean(abs)
	home = filepath.Clean(home)
	if abs == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(abs, prefix) {
		return filepath.Join("~", strings.TrimPrefix(abs, prefix))
	}
	return abs
}

func scoreChildMarkers(dir string) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	score := 0
	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}
		base := strings.ToLower(ent.Name())
		if markerMatch(base) {
			score += 12
		}
	}
	return score
}

func markerMatch(lowerName string) bool {
	markers := emulatorRootMarkers()
	i := sort.SearchStrings(markers, lowerName)
	return i < len(markers) && markers[i] == lowerName
}

func dedupeRoots(in []RootCandidate) []RootCandidate {
	seen := map[string]struct{}{}
	var out []RootCandidate
	for _, r := range in {
		if _, ok := seen[r.Path]; ok {
			continue
		}
		seen[r.Path] = struct{}{}
		out = append(out, r)
	}
	return out
}
