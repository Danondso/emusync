package model

import (
	"testing"
)

func TestMatchesProcess(t *testing.T) {
	tests := []struct {
		name         string
		processName  string
		processNames []string
		want         bool
	}{
		{
			name:         "exact_match",
			processName:  "retroarch",
			processNames: []string{"retroarch"},
			want:         true,
		},
		{
			name:         "no_match",
			processName:  "mame",
			processNames: []string{"retroarch"},
			want:         false,
		},
		{
			name:         "case_insensitive",
			processName:  "RetroArch",
			processNames: []string{"retroarch"},
			want:         true,
		},
		{
			name:         "appimage_process",
			processName:  "pcsx2-qt.AppImage",
			processNames: []string{"pcsx2-qt"},
			want:         true,
		},
		{
			name:         "appimage_config",
			processName:  "pcsx2-qt",
			processNames: []string{"pcsx2-Qt.AppImage"},
			want:         true,
		},
		{
			name:         "appimage_case_insensitive",
			processName:  "MGBA.AppImage",
			processNames: []string{"mgba.AppImage"},
			want:         true,
		},
		{
			name:         "exe_suffix",
			processName:  "xenia_canary.exe",
			processNames: []string{"xenia_canary.exe"},
			want:         true,
		},
		{
			name:         "path_prefix",
			processName:  "/usr/bin/retroarch",
			processNames: []string{"retroarch"},
			want:         true,
		},
		{
			name:         "empty_process_names",
			processName:  "retroarch",
			processNames: []string{},
			want:         false,
		},
		{
			name:         "multi_name_second",
			processName:  "ppsspp",
			processNames: []string{"PPSSPPSDL", "ppsspp"},
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &EmulatorConfig{
				ProcessNames: tt.processNames,
			}
			got := e.MatchesProcess(tt.processName)
			if got != tt.want {
				t.Errorf("MatchesProcess(%q) = %v, want %v", tt.processName, got, tt.want)
			}
		})
	}
}
