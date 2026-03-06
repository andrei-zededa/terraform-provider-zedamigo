// SPDX-License-Identifier: MPL-2.0

//go:build darwin && arm64

package hypervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	vfconfig "github.com/crc-org/vfkit/pkg/config"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// VFKitHypervisor implements Hypervisor using vfkit (Apple Virtualization.framework).
type VFKitHypervisor struct {
	VfkitPath   string
	QemuImgPath string
}

func (h *VFKitHypervisor) PrepareDisks(ctx context.Context, conf VMConfig) (VMPaths, error) {
	var paths VMPaths
	d := conf.ResourceDir

	// Convert qcow2 base images to raw format for vfkit.
	paths.DiskImage = filepath.Join(d, "disk0.raw")
	if err := h.convertToRaw(ctx, d, conf.DiskImageBase, paths.DiskImage); err != nil {
		return paths, fmt.Errorf("unable to create disk image: %w", err)
	}

	// Resize if needed.
	if conf.HasDiskSize {
		res, err := cmd.Run(d, h.QemuImgPath, "resize", "-f", "raw", paths.DiskImage, fmt.Sprintf("%dM", conf.DiskSizeMB))
		if err != nil {
			return paths, fmt.Errorf("unable to resize disk image: %w; %s", err, res.Stderr)
		}
	}

	// Create second disk image if configured.
	paths.Disk1Image = ""
	if conf.Disk1ImageBase != "" {
		paths.Disk1Image = filepath.Join(d, "disk1.raw")
		if err := h.convertToRaw(ctx, d, conf.Disk1ImageBase, paths.Disk1Image); err != nil {
			return paths, fmt.Errorf("unable to create second disk image: %w", err)
		}
		if conf.HasDiskSize {
			res, err := cmd.Run(d, h.QemuImgPath, "resize", "-f", "raw", paths.Disk1Image, fmt.Sprintf("%dM", conf.DiskSizeMB))
			if err != nil {
				return paths, fmt.Errorf("unable to resize second disk image: %w; %s", err, res.Stderr)
			}
		}
	}

	// EFI variable store for vfkit (created automatically by vfkit if it doesn't exist).
	paths.OVMFVars = filepath.Join(d, "efi_variable_store")

	// If an OVMFVarsSrc is provided, copy it as the starting point.
	if conf.OVMFVarsSrc != "" {
		if _, err := cmd.CopyFile(conf.OVMFVarsSrc, paths.OVMFVars); err != nil {
			return paths, fmt.Errorf("unable to copy EFI variable store: %w", err)
		}
	}

	paths.PIDFile = filepath.Join(d, "vfkit.pid")
	paths.DebugScript = filepath.Join(d, "start_vm.bash")

	return paths, nil
}

// imageInfo holds the subset of qemu-img info JSON output we need.
type imageInfo struct {
	Format string `json:"format"`
}

// detectImageFormat returns the disk image format (e.g. "raw", "qcow2") using qemu-img info.
func (h *VFKitHypervisor) detectImageFormat(logDir, src string) (string, error) {
	res, err := cmd.Run(logDir, h.QemuImgPath, "info", "--output=json", src)
	if err != nil {
		return "", fmt.Errorf("qemu-img info failed: %w; %s", err, res.Stderr)
	}
	var info imageInfo
	if err := json.Unmarshal([]byte(res.Stdout), &info); err != nil {
		return "", fmt.Errorf("failed to parse qemu-img info output: %w", err)
	}
	return info.Format, nil
}

// convertToRaw converts a disk image to raw format using qemu-img, or copies it if already raw.
func (h *VFKitHypervisor) convertToRaw(ctx context.Context, logDir, src, dst string) error {
	format, err := h.detectImageFormat(logDir, src)
	if err != nil {
		return err
	}

	switch format {
	case "raw":
		tflog.Debug(ctx, "Source image is already raw, copying instead of converting", map[string]any{"src": src, "dst": dst})
		if _, err := cmd.CopyFile(src, dst); err != nil {
			return fmt.Errorf("failed to copy raw image: %w", err)
		}
		return nil
	case "qcow2":
		res, err := cmd.Run(logDir, h.QemuImgPath, "convert", "-f", "qcow2", "-O", "raw", src, dst)
		if err != nil {
			return fmt.Errorf("qemu-img convert failed: %w; %s", err, res.Stderr)
		}
		return nil
	default:
		return fmt.Errorf("unsupported disk image format %q; expected raw or qcow2", format)
	}
}

func (h *VFKitHypervisor) Start(ctx context.Context, conf VMConfig, paths VMPaths) error {
	d := conf.ResourceDir

	cpus := uint(4)
	if conf.CPUs > 0 {
		cpus = uint(conf.CPUs)
	}
	memMiB := uint64(4096)
	if conf.MemoryMB != "" {
		parsed, err := parseMemoryToMiB(conf.MemoryMB)
		if err != nil {
			tflog.Warn(ctx, "Failed to parse memory, using default 4096MiB", map[string]any{"error": err})
		} else {
			memMiB = parsed
		}
	}

	// UEFI boot.
	bootloader := vfconfig.NewEFIBootloader(paths.OVMFVars, true)
	vm := vfconfig.NewVirtualMachine(cpus, memMiB, bootloader)
	vm.Nested = true

	// Root disk.
	rootDisk, err := vfconfig.VirtioBlkNew(paths.DiskImage)
	if err != nil {
		return fmt.Errorf("configure root disk: %w", err)
	}
	devices := []vfconfig.VirtioDevice{rootDisk}

	// Second disk.
	if paths.Disk1Image != "" {
		disk1, err := vfconfig.VirtioBlkNew(paths.Disk1Image)
		if err != nil {
			return fmt.Errorf("configure second disk: %w", err)
		}
		devices = append(devices, disk1)
	}

	// Installer disk for installation mode.
	if conf.IsInstallation {
		installerPath := conf.InstallerISO
		if conf.InstallerRaw != "" {
			installerPath = conf.InstallerRaw
		}
		if installerPath != "" {
			instDisk, err := vfconfig.VirtioBlkNew(installerPath)
			if err != nil {
				return fmt.Errorf("configure installer disk: %w", err)
			}
			devices = append(devices, instDisk)
		}
	}

	// Default NAT networking.
	netDev, err := vfconfig.VirtioNetNew("")
	if err != nil {
		return fmt.Errorf("configure network: %w", err)
	}
	devices = append(devices, netDev)

	// Entropy.
	rngDev, err := vfconfig.VirtioRngNew()
	if err != nil {
		return fmt.Errorf("configure rng: %w", err)
	}
	devices = append(devices, rngDev)

	// Serial: log to file.
	serialLogPath := paths.SerialConsoleLog
	if serialLogPath == "" {
		if conf.IsInstallation {
			serialLogPath = filepath.Join(d, "serial_console_install.log")
		} else {
			serialLogPath = filepath.Join(d, "serial_console_run.log")
		}
		paths.SerialConsoleLog = serialLogPath
	}
	serialDev, err := vfconfig.VirtioSerialNew(serialLogPath)
	if err != nil {
		return fmt.Errorf("configure serial: %w", err)
	}
	devices = append(devices, serialDev)

	if err := vm.AddDevices(devices...); err != nil {
		return fmt.Errorf("add devices: %w", err)
	}

	args, err := vm.ToCmdLine()
	if err != nil {
		return fmt.Errorf("build vfkit command line: %w", err)
	}

	// Write debug script.
	blob := []byte(fmt.Sprintf("#!/usr/bin/env bash\n\nset -eu;\n\n#### VFKIT ARGS: %v\n\n%s %s\n",
		args, h.VfkitPath, strings.Join(args, " ")))
	if err := os.WriteFile(paths.DebugScript, blob, 0o755); err != nil {
		tflog.Debug(ctx, "Failed to write start VM script", map[string]any{"error": err})
	}

	// Launch vfkit.
	if conf.IsInstallation {
		res, err := cmd.Run(d, h.VfkitPath, args...)
		if err != nil {
			return fmt.Errorf("failed to run vfkit VM for installing EVE-OS: %w; %s", err, res.Stderr)
		}
	} else {
		res, err := cmd.RunDetached(d, h.VfkitPath, args...)
		if err != nil {
			return fmt.Errorf("failed to start vfkit VM: %w; %s", err, res.Stderr)
		}
	}

	// Write PID file (RunDetached doesn't produce one for vfkit, read from process).
	// For detached mode, we need to find the PID. RunDetached uses cmd.Start(),
	// so the PID file may need to be populated differently. For now, find the vfkit process.
	// Note: In practice, we rely on the PID file being written or on process discovery.

	return nil
}

func (h *VFKitHypervisor) Status(ctx context.Context, resourceDir string) (bool, error) {
	pidFile := filepath.Join(resourceDir, "vfkit.pid")
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("can't read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false, fmt.Errorf("invalid PID in file: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}

	// Signal 0 checks if process exists without sending a signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false, nil
	}

	return true, nil
}

func (h *VFKitHypervisor) Stop(ctx context.Context, resourceDir string) error {
	pidFile := filepath.Join(resourceDir, "vfkit.pid")
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("can't read PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		tflog.Debug(ctx, "SIGTERM to vfkit failed (may already be stopped)", map[string]any{"error": err})
	}

	// Give it time to shut down.
	time.Sleep(2 * time.Second)

	return nil
}

// parseMemoryToMiB converts memory strings like "4G", "4096M", "4096" to MiB.
func parseMemoryToMiB(mem string) (uint64, error) {
	mem = strings.TrimSpace(mem)
	if mem == "" {
		return 4096, nil
	}

	if strings.HasSuffix(mem, "G") {
		val, err := strconv.ParseUint(strings.TrimSuffix(mem, "G"), 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1024, nil
	}

	if strings.HasSuffix(mem, "M") {
		val, err := strconv.ParseUint(strings.TrimSuffix(mem, "M"), 10, 64)
		if err != nil {
			return 0, err
		}
		return val, nil
	}

	// Plain number: assume MiB.
	val, err := strconv.ParseUint(mem, 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}
