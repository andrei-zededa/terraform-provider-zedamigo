//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"os"
)

func tapMoverMain() {
	fmt.Fprintf(os.Stderr, "TAP mover is not supported on macOS (darwin / arm64)\n")
	os.Exit(2)
}
