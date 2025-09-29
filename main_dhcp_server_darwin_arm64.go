//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"os"
)

func dhcpServerMain() {
	fmt.Fprintf(os.Stderr, "DHCP Server is not supported on macOS (darwin / arm64)\n")
	os.Exit(2)
}
