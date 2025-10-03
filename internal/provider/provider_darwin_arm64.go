//go:build darwin && arm64
// +build darwin,arm64

package provider

import (
	"embed"
)

var (
	// Embed OVMF files.
	//
	//go:embed embedded_ovmf/darwin_arm64/edk2-aarch64-code.fd
	//go:embed embedded_ovmf/darwin_arm64/edk2-arm-vars.fd
	embeddedOVMF          embed.FS
	embeddedOVMFFiles     = []string{"embedded_ovmf/darwin_arm64/edk2-aarch64-code.fd", "embedded_ovmf/darwin_arm64/edk2-arm-vars.fd"}
	embeddedOVMFTargetDir = "embedded_ovmf"

	qemuSystemCmd = "qemu-system-aarch64"
	qemuStdArgs   = []string{
		"-machine", "virt,accel=hvf",
		"-cpu", "host",
		"-nographic",
	}
)
