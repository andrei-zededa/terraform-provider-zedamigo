// SPDX-License-Identifier: MPL-2.0

//go:build !(darwin && arm64)

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

// configureVFKit is a stub on platforms where the vfkit backend is not built.
// The vfkit backend requires the provider to run natively on macOS arm64 (it is
// not executor-based), and a darwin target is only ever reached when it is the
// local host, so this stub is unreachable in practice — it exists so the shared
// hypervisor selection in configurePlatformTools compiles on all platforms.
func configureVFKit(_ context.Context, _ *ZedAmigoProviderConfig, resp *provider.ConfigureResponse) {
	resp.Diagnostics.AddError(
		"macOS (vfkit) backend not available in this build",
		"The vfkit backend is only available when the provider runs natively on macOS (arm64). "+
			"Remote targets are supported for Linux hosts (QEMU) only.",
	)
}
