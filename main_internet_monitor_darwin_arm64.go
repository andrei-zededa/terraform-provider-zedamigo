//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"os"
)

func internetMonitorMain() {
	fmt.Fprintf(os.Stderr, "Internet Monitor is not supported on macOS (darwin / arm64)\n")
	os.Exit(2)
}
