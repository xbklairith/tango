package main

import (
	"os"
)

// version is set via -ldflags at build time.
var version = "dev"

func main() {
	if err := newRootCmd(version).Execute(); err != nil {
		os.Exit(1)
	}
}
