//go:build !linux

package watchlock

import "fmt"

// Acquire is unsupported outside Linux (watch relies on Linux /proc monitoring).
func Acquire(path string) (release func(), err error) {
	_ = path
	return nil, fmt.Errorf("emusync watch is supported only on Linux")
}
