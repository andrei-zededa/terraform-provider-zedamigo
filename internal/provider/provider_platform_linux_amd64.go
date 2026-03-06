// SPDX-License-Identifier: MPL-2.0

//go:build linux && amd64

package provider

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Embed OVMF files.
//
//go:embed embedded_ovmf/OVMF_CODE.fd
//go:embed embedded_ovmf/OVMF_VARS.fd
var embeddedOVMF embed.FS

func configurePlatformTools(ctx context.Context, zaConf *ZedAmigoProviderConfig, resp *provider.ConfigureResponse) {
	// Extract OVMF files.
	if err := os.MkdirAll(filepath.Join(zaConf.LibPath, "embedded_ovmf"), 0o700); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("%s", err),
			fmt.Sprintf("Failed to create lib_path/embedded_ovmf directory: %v", err),
		)
		return
	}

	for _, f := range []string{filepath.Join("embedded_ovmf", "OVMF_CODE.fd"), filepath.Join("embedded_ovmf", "OVMF_VARS.fd")} {
		if err := extractFileIfNotExists(f, filepath.Join(zaConf.LibPath, f)); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("%s", err),
				fmt.Sprintf("Failed to extract OVMF file '%s': %v", f, err),
			)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	baseOVMFCode := filepath.Join(zaConf.LibPath, "embedded_ovmf", "OVMF_CODE.fd")
	baseOVMFVars := filepath.Join(zaConf.LibPath, "embedded_ovmf", "OVMF_VARS.fd")

	// Look up QEMU.
	q, err := exec.LookPath("qemu-system-x86_64")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-system-x86_64` executable.",
			fmt.Sprintf("Can't find the `qemu-system-x86_64` executable, got error: %v", err))
		return
	}

	qi, err := exec.LookPath("qemu-img")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-img` executable.",
			fmt.Sprintf("Can't find the `qemu-img` executable, got error: %v", err))
		return
	}

	// ip command.
	ip, err := exec.LookPath("ip")
	if err != nil {
		resp.Diagnostics.AddError("Can't find `ip`.",
			fmt.Sprintf("Can't find `ip`, got error: %v", err))
		return
	}
	zaConf.IP = ip

	zaConf.QemuImg = qi

	// taskset (optional).
	var tasksetPath string
	taskset, err := exec.LookPath("taskset")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `taskset` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the cpu_pins feature. Can't find `taskset`, got error: %v", err))
	} else {
		tasksetPath = taskset
	}
	if resp.Diagnostics.HasError() {
		return
	}

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
	if stAbs, err := filepath.Abs(st); err != nil {
		tflog.Debug(ctx, "filepath.Abs error", map[string]any{"error": err})
	} else {
		zaConf.Swtpm = stAbs
		if stReal, err := filepath.EvalSymlinks(stAbs); err != nil {
			tflog.Debug(ctx, "filepath.EvalSymlinks error", map[string]any{"error": err})
		} else {
			zaConf.Swtpm = stReal
		}
	}

	// genisoimage (optional).
	gencmd, err := exec.LookPath("genisoimage")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `genisoimage` executable (part of the `cdrkit` package or the `genisoimage` package).",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the Cloud Init ISO resource. Can't find `genisoimage`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.GenISOImage = gencmd

	// Create QEMU hypervisor.
	zaConf.Hypervisor = &hypervisor.QEMUHypervisor{
		QemuPath:     q,
		QemuImgPath:  qi,
		BaseOVMFCode: baseOVMFCode,
		BaseOVMFVars: baseOVMFVars,
		TasksetPath:  tasksetPath,
		UseSudo:      zaConf.UseSudo,
		SudoPath:     zaConf.Sudo,
	}
}

// extractFileIfNotExists checks if a file exists at targetPath, and if not,
// extracts it from the embedded filesystem.
func extractFileIfNotExists(embeddedPath, targetPath string) error {
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("error checking if file exists: %w", err)
	}

	data, err := embeddedOVMF.ReadFile(embeddedPath)
	if err != nil {
		return fmt.Errorf("error reading embedded file %s: %w", embeddedPath, err)
	}

	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return fmt.Errorf("error writing file to %s: %w", targetPath, err)
	}

	return nil
}
