package watcher

import (
	"testing"
)

func TestIsProtonProcess(t *testing.T) {
	tests := []struct {
		name string
		proc *ProcessInfo
		want bool
	}{
		{
			name: "pressure_vessel",
			proc: &ProcessInfo{Cmdline: []string{"pressure-vessel-wrap", "--", "game"}},
			want: true,
		},
		{
			name: "wine_preloader",
			proc: &ProcessInfo{Cmdline: []string{"/proton/bin/wine-preloader", "game.exe"}},
			want: true,
		},
		{
			name: "wine64",
			proc: &ProcessInfo{Cmdline: []string{"/proton/bin/wine64-preloader", "game.exe"}},
			want: true,
		},
		{
			name: "proton_path",
			proc: &ProcessInfo{
				Cmdline: []string{
					"/home/user/.steam/steamapps/common/Proton 8.0/proton",
					"run",
					"game.exe",
				},
			},
			want: true,
		},
		{
			name: "normal",
			proc: &ProcessInfo{Cmdline: []string{"dolphin-emu"}},
			want: false,
		},
		{
			name: "empty",
			proc: &ProcessInfo{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProtonProcess(tt.proc)
			if got != tt.want {
				t.Errorf("IsProtonProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractProtonExe(t *testing.T) {
	tests := []struct {
		name string
		proc *ProcessInfo
		want string
	}{
		{
			name: "found",
			proc: &ProcessInfo{Cmdline: []string{"wine-preloader", "/path/game.exe"}},
			want: "game.exe",
		},
		{
			name: "uppercase",
			proc: &ProcessInfo{Cmdline: []string{"wine", "GAME.EXE"}},
			want: "GAME.EXE",
		},
		{
			name: "no_exe",
			proc: &ProcessInfo{Cmdline: []string{"proton", "run", "launcher.sh"}},
			want: "",
		},
		{
			name: "empty",
			proc: &ProcessInfo{Cmdline: []string{}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractProtonExe(tt.proc)
			if got != tt.want {
				t.Errorf("ExtractProtonExe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProtonNames(t *testing.T) {
	tests := []struct {
		name      string
		proc      *ProcessInfo
		wantEmpty bool
	}{
		{
			name: "with_exe",
			proc: &ProcessInfo{
				Cmdline: []string{"wine-preloader", "/path/game.exe"},
			},
			wantEmpty: false,
		},
		{
			name: "without_exe",
			proc: &ProcessInfo{
				Cmdline: []string{"proton", "run", "launcher.sh"},
			},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProtonNames(tt.proc)
			if tt.wantEmpty && len(got) != 0 {
				t.Errorf("ProtonNames() = %v, want empty slice", got)
			}
			if !tt.wantEmpty && len(got) == 0 {
				t.Error("ProtonNames() returned empty slice, want non-empty")
			}
		})
	}
}
