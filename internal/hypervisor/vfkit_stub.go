// SPDX-License-Identifier: MPL-2.0

//go:build !(darwin && arm64)

package hypervisor

// VFKitHypervisor is a stub type on non-darwin/arm64 platforms.
// It exists so that the type can be referenced in shared code for documentation
// purposes, but it should never be instantiated on these platforms.
type VFKitHypervisor struct {
	VfkitPath   string
	QemuImgPath string
}
