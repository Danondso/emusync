//go:build linux

package watcher

import (
	"os"
	"testing"
)

func TestListProcesses(t *testing.T) {
	processes, err := ListProcesses()
	if err != nil {
		t.Fatalf("ListProcesses() error: %v", err)
	}

	if len(processes) == 0 {
		t.Fatal("ListProcesses() returned no processes")
	}

	myPID := os.Getpid()
	var found bool
	for _, proc := range processes {
		if proc.PID == myPID {
			found = true
			if proc.BinaryName() == "" {
				t.Errorf("current process (PID %d) has empty BinaryName", myPID)
			}
			break
		}
	}

	if !found {
		t.Errorf("current process (PID %d) not found in ListProcesses results", myPID)
	}
}
