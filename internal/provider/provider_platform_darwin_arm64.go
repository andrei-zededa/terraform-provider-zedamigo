// SPDX-License-Identifier: MPL-2.0

//go:build darwin && arm64

package provider

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/hashicorp/terraform-plugin-framework/provider"
)

func configurePlatformTools(ctx context.Context, zaConf *ZedAmigoProviderConfig, resp *provider.ConfigureResponse) {
	// Look up vfkit.
	vfkit, err := exec.LookPath("vfkit")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `vfkit` executable.",
			fmt.Sprintf("Can't find the `vfkit` executable. Install via: brew install vfkit. Got error: %v", err))
		return
	}

	// qemu-img is needed for format conversion (qcow2 -> raw).
	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-img` executable.",
			fmt.Sprintf("Can't find the `qemu-img` executable. Install via: brew install qemu. Got error: %v", err))
		return
	}

	zaConf.QemuImg = qi

	// swtpm (optional).
	st, err := exec.LookPath("swtpm")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `swtpm` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the SwTPM resource. Can't find `swtpm`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Swtpm = st

	// genisoimage (optional).
	gencmd, err := exec.LookPath("genisoimage")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `genisoimage` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the Cloud Init ISO resource. Can't find `genisoimage`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.GenISOImage = gencmd

	// ip command not available on macOS, leave zaConf.IP empty.
	// Networking resources that need `ip` will need platform-specific handling.

	// Detect nested virtualization support (requires Apple M3+).
	supportsNested, cpuBrand := hypervisor.SupportsNestedVirtualization()

	// Create vfkit hypervisor. gvproxy is embedded (self-invoked).
	zaConf.Hypervisor = &hypervisor.VFKitHypervisor{
		VfkitPath:          vfkit,
		QemuImgPath:        qi,
		SupportsNestedVirt: supportsNested,
	}

	if !supportsNested {
		resp.Diagnostics.AddWarning(
			"Nested virtualization not available",
			fmt.Sprintf(
				"Detected CPU: %s. Nested virtualization requires Apple M3 or later. "+
					"VMs will run without nested virtualization, which means app instances "+
					"inside EVE-OS will not work. Consider upgrading to an M3+ Mac for full functionality.",
				cpuBrand,
			),
		)
	}
}
