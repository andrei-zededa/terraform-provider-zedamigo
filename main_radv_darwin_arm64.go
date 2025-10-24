//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"os"
)

func radvMain() {
	fmt.Fprintf(os.Stderr, "RADV (Router Advertisement) is not supported on macOS (darwin / arm64)\n")
	os.Exit(2)
}
