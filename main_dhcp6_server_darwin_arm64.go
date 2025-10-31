//go:build darwin && arm64
// +build darwin,arm64

package main

import (
	"fmt"
	"os"
)

func dhcp6ServerMain() {
	fmt.Fprintf(os.Stderr, "DHCPv6 Server is not supported on macOS (darwin / arm64)\n")
	os.Exit(2)
}
