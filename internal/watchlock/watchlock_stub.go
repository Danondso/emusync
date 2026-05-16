//go:build !unix

package watchlock

import "fmt"

// Acquire is unsupported on non-Unix platforms (watch relies on Linux process monitoring).
func Acquire(path string) (release func(), err error) {
	_ = path
	return nil, fmt.Errorf("emusync watch is not supported on this platform")
}
