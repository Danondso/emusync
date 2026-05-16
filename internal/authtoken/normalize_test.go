package authtoken

import (
	"testing"
)

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

func TestWasTransformed(t *testing.T) {
	t.Parallel()
	if !WasTransformed(` "x" `) {
		t.Errorf("expected transformation for quoted value with spaces")
	}
	if WasTransformed("") || WasTransformed("abc") {
		t.Errorf("did not expect transformation for empty or plain token")
	}
}
