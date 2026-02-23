package main

import (
	"os"

	"github.com/dublin/emusync/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
