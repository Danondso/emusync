package authtoken

import "strings"

// Normalize trims whitespace and strips one pair of surrounding ASCII quotes.
// That avoids sync failures when tokens are copied from shell/.env examples that
// include quotes, or when accidental outer quotes end up in environment values.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	return s
}
