// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Embed OVMF files: EVE-OS's own x86_64 OVMF build (lf-edge/eve pkg/uefi,
// built with TPM2 support so a zedamigo_swtpm gets a measured boot). Origin,
// checksums and how to refresh them: embedded_ovmf/README.md.
//
//go:embed embedded_ovmf/OVMF_CODE.fd
//go:embed embedded_ovmf/OVMF_VARS.fd
var embeddedOVMF embed.FS

// configurePlatformTools looks up the tools required for the target platform
// and creates the matching hypervisor: QEMU on linux/amd64, vfkit on
// darwin/arm64.
func configurePlatformTools(ctx context.Context, zaConf *ZedAmigoProviderConfig, resp *provider.ConfigureResponse) {
	switch {
	case zaConf.TargetOS == "linux" && zaConf.TargetArch == "amd64":
		configureQEMUTools(ctx, zaConf, resp)
	case zaConf.TargetOS == "darwin" && zaConf.TargetArch == "arm64":
		configureVFKitTools(ctx, zaConf, resp)
	default:
		resp.Diagnostics.AddError("Unsupported target platform.",
			fmt.Sprintf("Target %q is %s/%s; supported target platforms are linux/amd64 (QEMU) and darwin/arm64 (vfkit).",
				zaConf.Target, zaConf.TargetOS, zaConf.TargetArch))
	}
}

func configureQEMUTools(ctx context.Context, zaConf *ZedAmigoProviderConfig, resp *provider.ConfigureResponse) {
	// UEFI firmware: the ovmf_code/ovmf_vars overrides, or the embedded OVMF
	// extracted onto the target.
	baseOVMFCode, baseOVMFVars, err := resolveOVMF(ctx, zaConf.Exec, zaConf.LibPath, zaConf.OVMFCode, zaConf.OVMFVars)
	if err != nil {
		resp.Diagnostics.AddError("Unable to set up the UEFI (OVMF) firmware.", err.Error())
		return
	}

	// Look up QEMU.
	q, err := zaConf.Exec.LookPath(ctx, "qemu-system-x86_64")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-system-x86_64` executable.",
			fmt.Sprintf("Can't find the `qemu-system-x86_64` executable, got error: %v", err))
		return
	}

	qi, err := zaConf.Exec.LookPath(ctx, "qemu-img")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-img` executable.",
			fmt.Sprintf("Can't find the `qemu-img` executable, got error: %v", err))
		return
	}

	// ip command.
	ip, err := zaConf.Exec.LookPath(ctx, "ip")
	if err != nil {
		resp.Diagnostics.AddError("Can't find `ip`.",
			fmt.Sprintf("Can't find `ip`, got error: %v", err))
		return
	}
	zaConf.IP = ip

	zaConf.QemuImg = qi

	// taskset (optional).
	var tasksetPath string
	taskset, err := zaConf.Exec.LookPath(ctx, "taskset")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `taskset` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the cpu_pins feature. Can't find `taskset`, got error: %v", err))
	} else {
		tasksetPath = taskset
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// flock (optional; required only by the host_reservation resource).
	flock, err := zaConf.Exec.LookPath(ctx, "flock")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `flock` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the host_reservation resource. Can't find `flock`, got error: %v", err))
	} else {
		zaConf.Flock = flock
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// swtpm (optional).
	st, err := zaConf.Exec.LookPath(ctx, "swtpm")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `swtpm` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the SwTPM resource. Can't find `swtpm`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Swtpm = st
	// Canonicalize the swtpm path only for a local target; for a remote target
	// the path lives on the remote filesystem and must not be resolved against
	// the local one.
	if zaConf.Exec.IsLocal() {
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
	}

	// genisoimage (optional).
	gencmd, err := zaConf.Exec.LookPath(ctx, "genisoimage")
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
		Exec:         zaConf.Exec,
	}
}

func configureVFKitTools(ctx context.Context, zaConf *ZedAmigoProviderConfig, resp *provider.ConfigureResponse) {
	// Look up vfkit.
	vfkit, err := zaConf.Exec.LookPath(ctx, "vfkit")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `vfkit` executable.",
			fmt.Sprintf("Can't find the `vfkit` executable. Install via: brew install vfkit. Got error: %v", err))
		return
	}

	// qemu-img is needed for format conversion (qcow2 -> raw).
	qi, err := zaConf.Exec.LookPath(ctx, "qemu-img")
	if err != nil {
		resp.Diagnostics.AddError("Can't find the `qemu-img` executable.",
			fmt.Sprintf("Can't find the `qemu-img` executable. Install via: brew install qemu. Got error: %v", err))
		return
	}

	zaConf.QemuImg = qi

	// swtpm (optional).
	st, err := zaConf.Exec.LookPath(ctx, "swtpm")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `swtpm` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the SwTPM resource. Can't find `swtpm`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.Swtpm = st

	// genisoimage (optional).
	gencmd, err := zaConf.Exec.LookPath(ctx, "genisoimage")
	if err != nil {
		resp.Diagnostics.AddWarning("Can't find the `genisoimage` executable.",
			fmt.Sprintf("This warning can be ignored if you DO NOT use the Cloud Init ISO resource. Can't find `genisoimage`, got error: %v", err))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	zaConf.GenISOImage = gencmd

	// ip command not available on macOS, leave zaConf.IP empty. The networking
	// resources that need `ip` reject non-Linux targets with a clear error via
	// requireLinuxTarget in their Configure methods.

	// Detect nested virtualization support (requires Apple M3+).
	supportsNested, cpuBrand := hypervisor.SupportsNestedVirtualization(ctx, zaConf.Exec, zaConf.LibPath)

	// Create vfkit hypervisor. gvproxy is embedded (self-invoked).
	zaConf.Hypervisor = &hypervisor.VFKitHypervisor{
		VfkitPath:          vfkit,
		QemuImgPath:        qi,
		SupportsNestedVirt: supportsNested,
		Exec:               zaConf.Exec,
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

// extractFileIfNotExists checks if a file exists at targetPath on the target,
// and if not, extracts it from the embedded filesystem (read locally) and
// writes it to the target via the executor.
// embeddedOVMFFiles are the firmware files embedded in the provider binary,
// as paths inside embeddedOVMF.
var embeddedOVMFFiles = []string{"embedded_ovmf/OVMF_CODE.fd", "embedded_ovmf/OVMF_VARS.fd"}

// embeddedOVMFVersion returns a short, stable identifier of the embedded
// firmware: the first 12 hex digits of the SHA-256 over all embedded files.
func embeddedOVMFVersion() (string, error) {
	h := sha256.New()
	for _, f := range embeddedOVMFFiles {
		data, err := embeddedOVMF.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("error reading embedded file %s: %w", f, err)
		}
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

// resolveOVMF returns the firmware code image and variable-store template the
// VMs boot with.
//
// A non-empty codeOverride / varsOverride (provider ovmf_code / ovmf_vars) is
// used as-is after checking it exists on the target. Otherwise the embedded
// file is extracted to lib_path/embedded_ovmf/<version>/, where <version> is
// derived from the embedded content. Versioning the directory matters because
// extraction never overwrites an existing file: with a fixed path, a provider
// upgrade that ships a different firmware would silently keep booting VMs on
// whatever an older release once extracted. Older directories are left in
// place, so VMs (re)started from a saved command line keep finding theirs.
func resolveOVMF(ctx context.Context, ex exec.Executor, libPath, codeOverride, varsOverride string) (string, string, error) {
	for _, o := range []struct{ name, path string }{{"ovmf_code", codeOverride}, {"ovmf_vars", varsOverride}} {
		if o.path == "" {
			continue
		}
		if _, err := ex.Stat(ctx, o.path); err != nil {
			return "", "", fmt.Errorf("%s %q is not usable on the target: %w", o.name, o.path, err)
		}
	}
	if codeOverride != "" && varsOverride != "" {
		return codeOverride, varsOverride, nil
	}

	ver, err := embeddedOVMFVersion()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(libPath, "embedded_ovmf", ver)
	if err := ex.MkdirAll(ctx, dir, 0o700); err != nil {
		return "", "", fmt.Errorf("failed to create %s: %w", dir, err)
	}

	extracted := make([]string, len(embeddedOVMFFiles))
	for i, f := range embeddedOVMFFiles {
		extracted[i] = filepath.Join(dir, filepath.Base(f))
		if err := extractFileIfNotExists(ctx, ex, f, extracted[i]); err != nil {
			return "", "", fmt.Errorf("failed to extract OVMF file %q: %w", f, err)
		}
	}

	code, vars := extracted[0], extracted[1]
	if codeOverride != "" {
		code = codeOverride
	}
	if varsOverride != "" {
		vars = varsOverride
	}
	return code, vars, nil
}

func extractFileIfNotExists(ctx context.Context, ex exec.Executor, embeddedPath, targetPath string) error {
	if _, err := ex.Stat(ctx, targetPath); err == nil {
		return nil
	} else if !exec.IsNotExist(err) {
		return fmt.Errorf("error checking if file exists: %w", err)
	}

	data, err := embeddedOVMF.ReadFile(embeddedPath)
	if err != nil {
		return fmt.Errorf("error reading embedded file %s: %w", embeddedPath, err)
	}

	if err := ex.WriteFile(ctx, targetPath, data, 0o644); err != nil {
		return fmt.Errorf("error writing file to %s: %w", targetPath, err)
	}

	return nil
}
