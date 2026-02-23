package watcher

import (
	"testing"
)

func TestIsFlatpakProcess(t *testing.T) {
	tests := []struct {
		name string
		proc *ProcessInfo
		want bool
	}{
		{
			name: "bwrap_comm",
			proc: &ProcessInfo{Comm: "bwrap"},
			want: true,
		},
		{
			name: "flatpak_cmdline",
			proc: &ProcessInfo{Cmdline: []string{"flatpak", "run", "org.app"}},
			want: true,
		},
		{
			name: "bwrap_cmdline",
			proc: &ProcessInfo{Cmdline: []string{"/usr/bin/bwrap", "--args"}},
			want: true,
		},
		{
			name: "normal_process",
			proc: &ProcessInfo{Comm: "dolphin-emu", Cmdline: []string{"dolphin-emu"}},
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
			got := IsFlatpakProcess(tt.proc)
			if got != tt.want {
				t.Errorf("IsFlatpakProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractFlatpakBinary(t *testing.T) {
	tests := []struct {
		name string
		proc *ProcessInfo
		want string
	}{
		{
			name: "with_separator",
			proc: &ProcessInfo{
				Cmdline: []string{"bwrap", "--bind", "/", "/", "--", "/app/bin/dolphin-emu", "--arg"},
			},
			want: "dolphin-emu",
		},
		{
			name: "no_separator_fallback",
			proc: &ProcessInfo{
				Cmdline: []string{"bwrap", "--bind", "/", "/", "retroarch"},
			},
			want: "retroarch",
		},
		{
			name: "empty_cmdline",
			proc: &ProcessInfo{Cmdline: []string{}},
			want: "",
		},
		{
			name: "only_bwrap",
			proc: &ProcessInfo{Cmdline: []string{"bwrap"}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFlatpakBinary(tt.proc)
			if got != tt.want {
				t.Errorf("ExtractFlatpakBinary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFlatpakNames(t *testing.T) {
	tests := []struct {
		name      string
		proc      *ProcessInfo
		wantEmpty bool
	}{
		{
			name: "has_binary",
			proc: &ProcessInfo{
				Cmdline: []string{"bwrap", "--", "/app/bin/dolphin-emu"},
			},
			wantEmpty: false,
		},
		{
			name:      "no_binary",
			proc:      &ProcessInfo{Cmdline: []string{"bwrap"}},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FlatpakNames(tt.proc)
			if tt.wantEmpty && len(got) != 0 {
				t.Errorf("FlatpakNames() = %v, want empty slice", got)
			}
			if !tt.wantEmpty && len(got) == 0 {
				t.Error("FlatpakNames() returned empty slice, want non-empty")
			}
		})
	}
}
