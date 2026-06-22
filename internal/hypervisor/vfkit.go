// SPDX-License-Identifier: MPL-2.0

//go:build darwin && arm64

package hypervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/socket"
	vfconfig "github.com/crc-org/vfkit/pkg/config"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// VFKitHypervisor implements Hypervisor using vfkit (Apple Virtualization.framework).
type VFKitHypervisor struct {
	VfkitPath          string
	QemuImgPath        string
	SupportsNestedVirt bool
}

// SupportsNestedVirtualization checks if the CPU is Apple M3 or later
// by parsing `sysctl -n machdep.cpu.brand_string` (e.g. "Apple M3 Pro").
func SupportsNestedVirtualization() (supported bool, cpuBrand string) {
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		return false, "unknown"
	}
	brand := strings.TrimSpace(string(out))
	re := regexp.MustCompile(`Apple M(\d+)`)
	matches := re.FindStringSubmatch(brand)
	if len(matches) < 2 {
		return false, brand
	}
	gen, err := strconv.Atoi(matches[1])
	if err != nil {
		return false, brand
	}
	return gen >= 3, brand
}

const (
	gvproxyGuestIP      = "192.168.127.2"     // gvproxy default DHCP lease
	gvproxyMAC          = "5a:94:ef:e4:0c:ee" // MAC tied to that DHCP lease
	gvproxyPollInterval = 100 * time.Millisecond
	gvproxyPollTimeout  = 3 * time.Second
)

func (h *VFKitHypervisor) PrepareDisks(ctx context.Context, conf VMConfig) (VMPaths, error) {
	var paths VMPaths
	d := conf.ResourceDir

	// Prepare disks in slot order (index 0 = disk0, 1 = disk1, ...).
	paths.DiskImages = make([]string, len(conf.Disks))
	for i, disk := range conf.Disks {
		switch disk.Type {
		case DiskDevice, DiskFile:
			// vfkit attaches raw images / block devices directly; it cannot read
			// qcow2. Reject a direct qcow2 disk with a clear error. The source
			// lives outside the resource directory and is never modified here.
			if disk.Format == "qcow2" {
				return paths, fmt.Errorf("disk %d: vfkit does not support qcow2 direct disks; use a raw image or block device", i)
			}
			paths.DiskImages[i] = disk.Source
		case DiskOverlay, "":
			// Convert the qcow2 base image to raw format for vfkit.
			raw := filepath.Join(d, fmt.Sprintf("disk%d.raw", i))
			if err := h.convertToRaw(ctx, d, disk.Source, raw); err != nil {
				return paths, fmt.Errorf("unable to create disk %d image: %w", i, err)
			}
			if disk.HasSize {
				res, err := cmd.Run(d, h.QemuImgPath, "resize", "-f", "raw", raw, fmt.Sprintf("%dM", disk.SizeMB))
				if err != nil {
					return paths, fmt.Errorf("unable to resize disk %d image: %w; %s", i, err, res.Stderr)
				}
			}
			paths.DiskImages[i] = raw
		default:
			return paths, fmt.Errorf("unknown disk %d type %q", i, disk.Type)
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

// startGvproxy launches gvproxy (via self-invocation) as a detached process
// with port forwarding and returns the unix socket path for vfkit to connect to.
func (h *VFKitHypervisor) startGvproxy(ctx context.Context, conf VMConfig) (string, error) {
	d := conf.ResourceDir
	socketPath := filepath.Join(d, "vfkit.sock")
	pidFile := filepath.Join(d, "gvproxy.pid")

	// Build comma-separated forwards string from the shared definition.
	forwardStr := GvproxyForwards(conf.SSHPort, gvproxyGuestIP)

	args := []string{
		"-pid-file", pidFile,
		"-gvproxy",
		"-gp.listen-vfkit", fmt.Sprintf("unixgram://%s", socketPath),
		"-gp.forwards", forwardStr,
	}

	tflog.Debug(ctx, "Starting gvproxy (self-invoke)", map[string]any{"args": args})

	res, err := cmd.RunDetached(d, os.Args[0], args...)
	if err != nil {
		return "", fmt.Errorf("failed to start gvproxy: %w; %s", err, res.Stderr)
	}

	// Write PID file in case gvproxy hasn't written it yet.
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(res.PID)), 0o600); err != nil {
		return "", fmt.Errorf("failed to write gvproxy PID file: %w", err)
	}

	// Poll for the socket file to appear.
	deadline := time.Now().Add(gvproxyPollTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			tflog.Debug(ctx, "gvproxy socket ready", map[string]any{"path": socketPath})
			return socketPath, nil
		}
		time.Sleep(gvproxyPollInterval)
	}

	return "", fmt.Errorf("gvproxy socket %s did not appear within %s", socketPath, gvproxyPollTimeout)
}

// stopGvproxy sends SIGTERM to the gvproxy process.
func (h *VFKitHypervisor) stopGvproxy(ctx context.Context, resourceDir string) {
	pidFile := filepath.Join(resourceDir, "gvproxy.pid")
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		if !os.IsNotExist(err) {
			tflog.Debug(ctx, "Failed to read gvproxy PID file", map[string]any{"error": err})
		}
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		tflog.Debug(ctx, "Invalid gvproxy PID", map[string]any{"error": err})
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		tflog.Debug(ctx, "SIGTERM to gvproxy failed (may already be stopped)", map[string]any{"error": err})
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
	vm.Nested = h.SupportsNestedVirt

	// Disks (slot order: disk0, disk1, ...). Per-disk drive_if / options are
	// QEMU-only and ignored by vfkit, which always attaches raw VirtioBlk devices.
	var devices []vfconfig.VirtioDevice
	for i, diskPath := range paths.DiskImages {
		blk, err := vfconfig.VirtioBlkNew(diskPath)
		if err != nil {
			return fmt.Errorf("configure disk %d: %w", i, err)
		}
		devices = append(devices, blk)
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

	// Networking: use gvproxy with port forwarding for running VMs,
	// simple NAT for installation VMs.
	var socketPath string
	if !conf.IsInstallation && conf.SSHPort != 0 {
		var gvErr error
		socketPath, gvErr = h.startGvproxy(ctx, conf)
		if gvErr != nil {
			return fmt.Errorf("start gvproxy: %w", gvErr)
		}
		netDev, err := vfconfig.VirtioNetNew(gvproxyMAC)
		if err != nil {
			return fmt.Errorf("configure network: %w", err)
		}
		netDev.SetUnixSocketPath(socketPath)
		devices = append(devices, netDev)
	} else {
		netDev, err := vfconfig.VirtioNetNew("")
		if err != nil {
			return fmt.Errorf("configure network: %w", err)
		}
		devices = append(devices, netDev)
	}

	// Entropy.
	rngDev, err := vfconfig.VirtioRngNew()
	if err != nil {
		return fmt.Errorf("configure rng: %w", err)
	}
	devices = append(devices, rngDev)

	// Serial console: always use PTY mode. A PTY tailer (launched separately
	// or in-process for installations) reads the PTY and writes to the log file.
	serialDev, err := vfconfig.VirtioSerialNewPty()
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
	var debugScript string
	if socketPath != "" {
		debugScript = fmt.Sprintf("#!/usr/bin/env bash\n\nset -eu;\n\n#### GVPROXY (started separately, see gvproxy.pid)\n#### VFKIT ARGS: %v\n\n%s %s\n",
			args, h.VfkitPath, strings.Join(args, " "))
	} else {
		debugScript = fmt.Sprintf("#!/usr/bin/env bash\n\nset -eu;\n\n#### VFKIT ARGS: %v\n\n%s %s\n",
			args, h.VfkitPath, strings.Join(args, " "))
	}
	if err := os.WriteFile(paths.DebugScript, []byte(debugScript), 0o755); err != nil {
		tflog.Debug(ctx, "Failed to write start VM script", map[string]any{"error": err})
	}

	// Launch vfkit.
	if conf.IsInstallation {
		resultChan := cmd.RunBG(d, h.VfkitPath, args...)

		// Poll for PTY path in the vfkit stderr log.
		ptyDevPath, err := waitForPtyPath(d)
		if err != nil {
			tflog.Warn(ctx, "Could not determine serial PTY path for installation", map[string]any{"error": err})
		}

		// Start in-process tailer goroutine if we have both a PTY and an output path.
		tailerCtx, tailerCancel := context.WithCancel(ctx)
		tailerDone := make(chan struct{})
		if err == nil && conf.SerialToFile != "" {
			go func() {
				defer close(tailerDone)
				if tErr := runPtyToFile(tailerCtx, ptyDevPath, conf.SerialToFile); tErr != nil {
					tflog.Warn(ctx, "PTY tailer error during installation", map[string]any{"error": tErr})
				}
			}()
		} else {
			close(tailerDone)
		}

		// Wait for vfkit to complete.
		result := <-resultChan
		tailerCancel()
		<-tailerDone

		if result.Error != nil {
			return fmt.Errorf("failed to run vfkit VM for installing EVE-OS: %w; %s", result.Error, result.Stderr)
		}
	} else {
		res, err := cmd.RunDetached(d, h.VfkitPath, args...)
		if err != nil {
			return fmt.Errorf("failed to start vfkit VM: %w; %s", err, res.Stderr)
		}
		// Write vfkit PID file.
		if err := os.WriteFile(paths.PIDFile, []byte(strconv.Itoa(res.PID)), 0o600); err != nil {
			return fmt.Errorf("failed to write vfkit PID file: %w", err)
		}
		// PTY mode: extract the PTY path from vfkit's stderr log so the tailer can connect.
		if ptyPath, err := parsePtyPath(res.Logs.Stderr); err == nil {
			ptyFile := filepath.Join(d, "serial.pty")
			if err := os.WriteFile(ptyFile, []byte(ptyPath+"\n"), 0o600); err != nil {
				tflog.Warn(ctx, "Failed to write serial PTY path file", map[string]any{"error": err})
			}
			tflog.Info(ctx, "Serial console PTY available", map[string]any{"pty": ptyPath, "connect": fmt.Sprintf("screen %s", ptyPath)})
		} else {
			tflog.Warn(ctx, "Could not determine serial PTY path from vfkit output", map[string]any{"error": err, "stderr_log": res.Logs.Stderr})
		}
	}

	return nil
}

func (h *VFKitHypervisor) ApplyCPUPins(_ context.Context, _ VMConfig) error {
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

	// Stop gvproxy if it was running.
	h.stopGvproxy(ctx, resourceDir)

	return nil
}

// parsePtyPath reads a vfkit stderr log file and extracts the PTY device path
// from the line: level=info msg="Using PTY (pty path: /dev/ttys003)"
func parsePtyPath(logFile string) (string, error) {
	f, err := os.Open(logFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		const marker = "pty path: "
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(marker):]
		// The path is followed by a closing paren.
		if end := strings.Index(rest, ")"); end > 0 {
			return rest[:end], nil
		}
		return strings.TrimSpace(rest), nil
	}
	return "", fmt.Errorf("PTY path not found in log file %s", logFile)
}

// waitForPtyPath polls for a vfkit stderr log file in the resource directory
// and extracts the PTY device path from it.
func waitForPtyPath(resourceDir string) (string, error) {
	const (
		pollInterval = 200 * time.Millisecond
		pollTimeout  = 10 * time.Second
	)

	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(resourceDir, "*_vfkit_stderr.log"))
		for _, logFile := range matches {
			if ptyPath, err := parsePtyPath(logFile); err == nil {
				return ptyPath, nil
			}
		}
		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("PTY path not found in vfkit stderr logs within %s", pollTimeout)
}

// runPtyToFile opens a PTY device and writes timestamped lines to an output file.
func runPtyToFile(ctx context.Context, ptyDevPath, outPath string) error {
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open output file %s: %w", outPath, err)
	}
	defer outFile.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	tailer := socket.NewTailer(outFile, logger)
	defer tailer.Close()

	return tailer.RunPty(ctx, ptyDevPath)
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
