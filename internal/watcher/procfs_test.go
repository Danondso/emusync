package watcher

import (
	"testing"
)

func TestProcessInfo_BinaryName(t *testing.T) {
	tests := []struct {
		name string
		proc ProcessInfo
		want string
	}{
		{
			name: "exe_set",
			proc: ProcessInfo{Exe: "/usr/bin/dolphin-emu"},
			want: "dolphin-emu",
		},
		{
			name: "cmdline_only",
			proc: ProcessInfo{Cmdline: []string{"/usr/bin/dolphin-emu"}},
			want: "dolphin-emu",
		},
		{
			name: "comm_only",
			proc: ProcessInfo{Comm: "dolphin-emu"},
			want: "dolphin-emu",
		},
		{
			name: "exe_priority",
			proc: ProcessInfo{
				Exe:     "/usr/bin/real",
				Cmdline: []string{"/other/fake"},
			},
			want: "real",
		},
		{
			name: "all_empty",
			proc: ProcessInfo{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.proc.BinaryName()
			if got != tt.want {
				t.Errorf("BinaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProcessInfo_AllNames(t *testing.T) {
	tests := []struct {
		name string
		proc ProcessInfo
		want []string
	}{
		{
			name: "all_distinct",
			proc: ProcessInfo{
				Exe:     "/a/x",
				Cmdline: []string{"/b/y"},
				Comm:    "z",
			},
			want: []string{"x", "z", "y"},
		},
		{
			name: "all_same",
			proc: ProcessInfo{
				Exe:     "/a/foo",
				Cmdline: []string{"/b/foo"},
				Comm:    "foo",
			},
			want: []string{"foo"},
		},
		{
			name: "no_exe",
			proc: ProcessInfo{
				Cmdline: []string{"/a/bar"},
				Comm:    "baz",
			},
			want: []string{"bar", "baz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.proc.AllNames()
			if len(got) != len(tt.want) {
				t.Fatalf("AllNames() returned %d names %v, want %d names %v",
					len(got), got, len(tt.want), tt.want)
			}
			for i, name := range got {
				if name != tt.want[i] {
					t.Errorf("AllNames()[%d] = %q, want %q", i, name, tt.want[i])
				}
			}
		})
	}
}
