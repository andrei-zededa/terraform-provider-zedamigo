//go:build linux && amd64
// +build linux,amd64

package provider

import (
	"embed"
)

var (
	// Embed OVMF files.
	//
	//go:embed embedded_ovmf/linux_amd64/OVMF_CODE.fd
	//go:embed embedded_ovmf/linux_amd64/OVMF_VARS.fd
	embeddedOVMF          embed.FS
	embeddedOVMFFiles     = []string{"embedded_ovmf/linux_amd64/OVMF_CODE.fd", "embedded_ovmf/linux_amd64/OVMF_VARS.fd"}
	embeddedOVMFTargetDir = "embedded_ovmf"

	qemuSystemCmd = "qemu-system-x86_64"
	qemuStdArgs   = []string{
		"--enable-kvm",
		"-machine", "q35,accel=kvm,kernel-irqchip=split",
		"-device", "intel-iommu,intremap=on",
		"-cpu", "host",
		"-nographic",
	}
)
