package authtoken

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"  abc  ", "abc"},
		{`"quoted"`, "quoted"},
		{`'quoted'`, "quoted"},
		{`"'inner'"`, "'inner'"},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
