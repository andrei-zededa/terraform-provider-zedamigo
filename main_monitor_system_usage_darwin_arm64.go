//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"os"
)

func monitorSystemUsageMain() {
	fmt.Fprintf(os.Stderr, "Monitor System Usage is not supported on macOS (darwin / arm64)\n")
	os.Exit(2)
}
