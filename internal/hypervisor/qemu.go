// SPDX-License-Identifier: MPL-2.0

package hypervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/qmp"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// QEMUHypervisor implements Hypervisor using QEMU/KVM.
type QEMUHypervisor struct {
	QemuPath     string
	QemuImgPath  string
	BaseOVMFCode string
	BaseOVMFVars string
	TasksetPath  string
	UseSudo      bool
	SudoPath     string

	// Exec runs all commands, filesystem and socket operations on the target
	// (localhost or, in remote mode, the SSH host).
	Exec exec.Executor
}

// qmpDialer returns a qmp.DialFunc that dials through the executor with the
// given timeout (local net.Dialer or SSH streamlocal forwarding).
func (h *QEMUHypervisor) qmpDialer(timeout time.Duration) qmp.DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return h.Exec.Dial(ctx, network, addr, timeout)
	}
}

const (
	qemuGvproxyGuestIP      = "192.168.127.2"
	qemuGvproxyMAC          = "5a:94:ef:e4:0c:ee"
	qemuGvproxyPollInterval = 100 * time.Millisecond
	qemuGvproxyPollTimeout  = 3 * time.Second
)

// startGvproxy launches gvproxy (via self-invocation) as a detached process
// with port forwarding and returns the unix socket path for QEMU to connect to.
func (h *QEMUHypervisor) startGvproxy(ctx context.Context, conf VMConfig) (string, error) {
	d := conf.ResourceDir
	socketPath := filepath.Join(d, "gvproxy-qemu.sock")
	pidFile := filepath.Join(d, "gvproxy.pid")

	// Build comma-separated forwards string from the shared definition.
	forwardStr := GvproxyForwards(conf.SSHPort, qemuGvproxyGuestIP)

	args := []string{
		"-pid-file", pidFile,
		"-gvproxy",
		"-gp.listen-qemu", fmt.Sprintf("unix://%s", socketPath),
		"-gp.forwards", forwardStr,
	}

	tflog.Debug(ctx, "Starting gvproxy for QEMU (self-invoke)", map[string]any{"args": args})

	res, err := h.Exec.RunDetached(ctx, d, h.Exec.SelfPath(), args...)
	if err != nil {
		return "", fmt.Errorf("failed to start gvproxy: %w; %s", err, res.Stderr)
	}

	// Write PID file in case gvproxy hasn't written it yet.
	if err := h.Exec.WriteFile(ctx, pidFile, []byte(strconv.Itoa(res.PID)), 0o600); err != nil {
		return "", fmt.Errorf("failed to write gvproxy PID file: %w", err)
	}

	// Poll for the socket file to appear.
	deadline := time.Now().Add(qemuGvproxyPollTimeout)
	for time.Now().Before(deadline) {
		if _, err := h.Exec.Stat(ctx, socketPath); err == nil {
			tflog.Debug(ctx, "gvproxy socket ready", map[string]any{"path": socketPath})
			return socketPath, nil
		}
		time.Sleep(qemuGvproxyPollInterval)
	}

	return "", fmt.Errorf("gvproxy socket %s did not appear within %s", socketPath, qemuGvproxyPollTimeout)
}

// stopGvproxy sends SIGTERM to the gvproxy process.
func (h *QEMUHypervisor) stopGvproxy(ctx context.Context, resourceDir string) {
	pidFile := filepath.Join(resourceDir, "gvproxy.pid")
	pidBytes, err := h.Exec.ReadFile(ctx, pidFile)
	if err != nil {
		if !exec.IsNotExist(err) {
			tflog.Debug(ctx, "Failed to read gvproxy PID file", map[string]any{"error": err})
		}
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		tflog.Debug(ctx, "Invalid gvproxy PID", map[string]any{"error": err})
		return
	}

	if err := h.Exec.Kill(ctx, pid, syscall.SIGTERM); err != nil {
		tflog.Debug(ctx, "SIGTERM to gvproxy failed (may already be stopped)", map[string]any{"error": err})
	}
}

func (h *QEMUHypervisor) PrepareDisks(ctx context.Context, conf VMConfig) (VMPaths, error) {
	var paths VMPaths
	d := conf.ResourceDir

	// Prepare disks in slot order (index 0 = disk0, 1 = disk1, ...).
	paths.DiskImages = make([]string, len(conf.Disks))
	for i, disk := range conf.Disks {
		switch disk.Type {
		case DiskDevice, DiskFile:
			// Use the block device / partition / existing file directly; nothing
			// to create. The source lives outside the resource directory and is
			// never modified or removed by the provider.
			paths.DiskImages[i] = disk.Source
		case DiskOverlay, "":
			img := filepath.Join(d, fmt.Sprintf("disk%d.disk_img.qcow2", i))
			qemuImgArgs := []string{
				"create", "-f", "qcow2",
				"-b", disk.Source, "-F", "qcow2",
				img,
			}
			if disk.HasSize {
				qemuImgArgs = append(qemuImgArgs, fmt.Sprintf("%dM", disk.SizeMB))
			}
			res, err := h.Exec.Run(ctx, d, h.QemuImgPath, qemuImgArgs...)
			if err != nil {
				return paths, fmt.Errorf("unable to create disk %d image: %w; %s", i, err, res.Stderr)
			}
			paths.DiskImages[i] = img
		default:
			return paths, fmt.Errorf("unknown disk %d type %q", i, disk.Type)
		}
	}

	// Copy OVMF vars.
	paths.OVMFVars = filepath.Join(d, "UEFI_OVMF_VARS.bin")
	ovSrc := h.BaseOVMFVars
	if conf.OVMFVarsSrc != "" {
		ovSrc = conf.OVMFVarsSrc
	}
	if _, err := h.Exec.CopyFile(ctx, ovSrc, paths.OVMFVars); err != nil {
		return paths, fmt.Errorf("unable to copy UEFI OVMF vars: %w", err)
	}

	// Set standard paths.
	paths.QMPSocket = filepath.Join(d, "qmp.socket")
	paths.PIDFile = filepath.Join(d, "qemu.pid")
	paths.DebugScript = filepath.Join(d, "start_vm.bash")

	return paths, nil
}

func (h *QEMUHypervisor) Start(ctx context.Context, conf VMConfig, paths VMPaths) error {
	d := conf.ResourceDir

	qemuArgs := []string{}

	// VM name.
	name := conf.ID
	if conf.Name != "" {
		name = conf.Name
	}

	if conf.IsInstallation {
		qemuArgs = append(qemuArgs, "--name", fmt.Sprintf("edge_node_install_%s", name))
	} else {
		qemuArgs = append(qemuArgs, "--name", fmt.Sprintf("guest=%s,debug-threads=on", name))
	}

	qemuArgs = append(qemuArgs,
		"--enable-kvm", "-machine", "q35,accel=kvm,kernel-irqchip=split",
		"-nographic",
	)

	if conf.IsInstallation {
		qemuArgs = append(qemuArgs,
			"-m", "4096",
			"-cpu", "host", "-smp", "4,cores=2",
		)
	} else {
		mem := "4G"
		if conf.MemoryMB != "" {
			mem = conf.MemoryMB
		}
		cpus := int64(4)
		if conf.CPUs > 0 {
			cpus = conf.CPUs
		}
		qemuArgs = append(qemuArgs,
			"-m", mem,
			"-cpu", "host", "-smp", fmt.Sprintf("%d", cpus),
		)
	}

	qemuArgs = append(qemuArgs,
		// caching-mode=on is theoretically needed for using SR-IOV inside the VM, however with EVE-OS SR-IOV doesn't work.
		"-device", "intel-iommu,intremap=on,caching-mode=on,device-iotlb=on",
		"-smbios", fmt.Sprintf("type=1,serial=%s,manufacturer=Dell Inc.,product=ProLiant 100 with 2 disks", conf.SerialNo),
	)

	// Serial console.
	if conf.SerialType == "serial" {
		// Emulated ISA serial: guest uses ttyS0.
		if conf.SerialToSocket != "" {
			qemuArgs = append(qemuArgs, "-serial", fmt.Sprintf("unix:%s,server,wait", conf.SerialToSocket))
		} else if conf.SerialToFile != "" {
			qemuArgs = append(qemuArgs, "-serial", fmt.Sprintf("file:%s", conf.SerialToFile))
		}
	} else {
		// Virtio-serial (default): guest uses hvc0.
		if conf.SerialToSocket != "" {
			qemuArgs = append(qemuArgs,
				"-device", "virtio-serial-pci",
				"-chardev", fmt.Sprintf("socket,id=virtconsole0,path=%s,server=on,wait=on", conf.SerialToSocket),
				"-device", "virtconsole,chardev=virtconsole0",
			)
		} else if conf.SerialToFile != "" {
			qemuArgs = append(qemuArgs,
				"-device", "virtio-serial-pci",
				"-chardev", fmt.Sprintf("file,id=virtconsole0,path=%s", conf.SerialToFile),
				"-device", "virtconsole,chardev=virtconsole0",
			)
		}
	}

	// OVMF firmware.
	qemuArgs = append(qemuArgs,
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", h.BaseOVMFCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", paths.OVMFVars),
	)

	// NIC: use gvproxy stream transport when enabled, otherwise SLIRP.
	if conf.UseGvproxy && !conf.IsInstallation && conf.SSHPort != 0 {
		gvSocketPath, gvErr := h.startGvproxy(ctx, conf)
		if gvErr != nil {
			return fmt.Errorf("start gvproxy: %w", gvErr)
		}
		qemuArgs = append(qemuArgs,
			"-netdev", fmt.Sprintf("stream,id=usernet0,addr.type=unix,addr.path=%s", gvSocketPath),
			"-device", fmt.Sprintf("virtio-net-pci,netdev=usernet0,mac=%s", qemuGvproxyMAC),
		)
	} else {
		qemuArgs = append(qemuArgs, "-nic", conf.Nic0)
	}

	// Disk drives (slot order: disk0, disk1, ...).
	for i, disk := range conf.Disks {
		driveParts := []string{
			fmt.Sprintf("file=%s", paths.DiskImages[i]),
			fmt.Sprintf("format=%s", disk.Format),
		}
		if disk.DriveIf != "" {
			driveParts = append(driveParts, fmt.Sprintf("if=%s", disk.DriveIf))
		}
		driveParts = append(driveParts, disk.Options...)
		qemuArgs = append(qemuArgs, "-drive", strings.Join(driveParts, ","))
	}

	// Installer media (installation only).
	if conf.IsInstallation {
		if conf.InstallerISO != "" {
			qemuArgs = append(qemuArgs, "-cdrom", conf.InstallerISO)
		} else if conf.InstallerRaw != "" {
			qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=raw", conf.InstallerRaw))
		}

		qemuArgs = append(qemuArgs,
			"-boot", "once=d",
		)
	}

	// SwTPM.
	if conf.SwTPMSocket != "" {
		qemuArgs = append(qemuArgs,
			"-chardev", fmt.Sprintf("socket,id=chrtpm,path=%s", conf.SwTPMSocket),
			"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
			"-device", "tpm-crb,id=mytpm,tpmdev=tpm0",
		)
	}

	// QMP and PID file.
	qemuArgs = append(qemuArgs,
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", paths.QMPSocket),
		"-pidfile", paths.PIDFile,
	)

	// Extra args (passed verbatim to QEMU for both edge nodes and installations).
	qemuArgs = append(qemuArgs, conf.ExtraArgs...)

	// Write debug script.
	startVMscript := `#!/usr/bin/env bash

set -eu;

#### QEMU ARGS: %v

%s %s
`
	blob := []byte(fmt.Sprintf(startVMscript, qemuArgs, h.QemuPath, strings.Join(qemuArgs, " ")))
	if err := h.Exec.WriteFile(ctx, paths.DebugScript, blob, 0o755); err != nil {
		tflog.Debug(ctx, "Failed to write start VM script", map[string]any{"error": err})
	}

	// Launch QEMU.
	if conf.IsInstallation {
		// Installation runs synchronously (Run, not RunDetached).
		res, err := h.Exec.Run(ctx, d, h.QemuPath, qemuArgs...)
		if err != nil {
			return fmt.Errorf("failed to run QEMU VM for installing EVE-OS: %w; %s", err, res.Stderr)
		}
	} else {
		res, err := h.Exec.RunDetached(ctx, d, h.QemuPath, qemuArgs...)
		if err != nil {
			return fmt.Errorf("failed to start QEMU VM: %w; %s", err, res.Stderr)
		}
	}

	return nil
}

func (h *QEMUHypervisor) ApplyCPUPins(ctx context.Context, conf VMConfig) error {
	if conf.IsInstallation || len(conf.CPUPins) == 0 {
		return nil
	}
	cpus := int64(4)
	if conf.CPUs > 0 {
		cpus = conf.CPUs
	}
	return h.applyCPUPins(ctx, conf.ResourceDir, conf.CPUPins, int(cpus))
}

const (
	cpuPinPIDPollInterval = 200 * time.Millisecond
	cpuPinPIDPollTimeout  = 15 * time.Second
)

func (h *QEMUHypervisor) applyCPUPins(ctx context.Context, resourceDir string, cpuPins []int64, numCPUs int) error {
	pidFile := filepath.Join(resourceDir, "qemu.pid")

	// Poll for the PID file — QEMU writes it asynchronously after launch.
	var pidBytes []byte
	deadline := time.Now().Add(cpuPinPIDPollTimeout)
	for {
		var err error
		pidBytes, err = h.Exec.ReadFile(ctx, pidFile)
		if err == nil && len(strings.TrimSpace(string(pidBytes))) > 0 {
			break
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("QEMU PID file %s did not appear within %s: %w", pidFile, cpuPinPIDPollTimeout, err)
			}
			return fmt.Errorf("QEMU PID file %s was empty after %s", pidFile, cpuPinPIDPollTimeout)
		}
		time.Sleep(cpuPinPIDPollInterval)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	qemuPID := 0
	if _, err := fmt.Sscanf(pidStr, "%d", &qemuPID); err != nil {
		return fmt.Errorf("invalid QEMU PID %q: %w", pidStr, err)
	}

	return pinCPUThreads(ctx, h, qemuPID, cpuPins, numCPUs, resourceDir)
}

func (h *QEMUHypervisor) Status(ctx context.Context, resourceDir string) (bool, error) {
	mon, err := qmp.NewSocketMonitorWithDialer(ctx, "unix", filepath.Join(resourceDir, "qmp.socket"), h.qmpDialer(1*time.Second))
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}
	if err := mon.Connect(); err != nil {
		return false, fmt.Errorf("%w", err)
	}
	defer mon.Disconnect()

	raw, err := mon.Run([]byte(`{ "execute": "query-status" }`))
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}

	type StatusResult struct {
		ID     string `json:"id"`
		Return struct {
			Running    bool   `json:"running"`
			Singlestep bool   `json:"singlestep"`
			Status     string `json:"status"`
		} `json:"return"`
	}

	qs := StatusResult{}
	if err := json.Unmarshal(raw, &qs); err != nil {
		return false, fmt.Errorf("%w", err)
	}

	return qs.Return.Running, nil
}

func (h *QEMUHypervisor) Stop(ctx context.Context, resourceDir string) error {
	mon, err := qmp.NewSocketMonitorWithDialer(ctx, "unix", filepath.Join(resourceDir, "qmp.socket"), h.qmpDialer(2*time.Second))
	if err != nil {
		return fmt.Errorf("can't create QMP monitor: %w", err)
	}
	if err := mon.Connect(); err != nil {
		return fmt.Errorf("can't QMP connect: %w", err)
	}
	defer mon.Disconnect()

	_, err = mon.Run([]byte(`{ "execute": "quit" }`))
	if err != nil {
		// This may happen because QEMU exits before responding.
		tflog.Debug(ctx, "QMP quit command error (may be benign)", map[string]any{"error": err})
	}

	// Stop gvproxy if it was running.
	h.stopGvproxy(ctx, resourceDir)

	return nil
}
