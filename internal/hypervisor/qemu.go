// SPDX-License-Identifier: MPL-2.0

package hypervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
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
}

func (h *QEMUHypervisor) PrepareDisks(ctx context.Context, conf VMConfig) (VMPaths, error) {
	var paths VMPaths
	d := conf.ResourceDir

	// Create primary disk image with backing file.
	paths.DiskImage = filepath.Join(d, "disk0.disk_img.qcow2")
	qemuImgArgs := []string{
		"create", "-f", "qcow2",
		"-b", conf.DiskImageBase, "-F", "qcow2",
		paths.DiskImage,
	}
	if conf.HasDiskSize {
		qemuImgArgs = append(qemuImgArgs, fmt.Sprintf("%dM", conf.DiskSizeMB))
	}
	res, err := cmd.Run(d, h.QemuImgPath, qemuImgArgs...)
	if err != nil {
		return paths, fmt.Errorf("unable to create disk image: %w; %s", err, res.Stderr)
	}

	// Create second disk image if configured.
	paths.Disk1Image = ""
	if conf.Disk1ImageBase != "" {
		paths.Disk1Image = filepath.Join(d, "disk1.disk_img.qcow2")
		qemuImgArgs := []string{
			"create", "-f", "qcow2",
			"-b", conf.Disk1ImageBase, "-F", "qcow2",
			paths.Disk1Image,
		}
		if conf.HasDiskSize {
			qemuImgArgs = append(qemuImgArgs, fmt.Sprintf("%dM", conf.DiskSizeMB))
		}
		res, err := cmd.Run(d, h.QemuImgPath, qemuImgArgs...)
		if err != nil {
			return paths, fmt.Errorf("unable to create second disk image: %w; %s", err, res.Stderr)
		}
	}

	// Copy OVMF vars.
	paths.OVMFVars = filepath.Join(d, "UEFI_OVMF_VARS.bin")
	ovSrc := h.BaseOVMFVars
	if conf.OVMFVarsSrc != "" {
		ovSrc = conf.OVMFVarsSrc
	}
	if _, err := cmd.CopyFile(ovSrc, paths.OVMFVars); err != nil {
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
		"-device", "intel-iommu,intremap=on",
		"-smbios", fmt.Sprintf("type=1,serial=%s,manufacturer=Dell Inc.,product=ProLiant 100 with 2 disks", conf.SerialNo),
	)

	// Serial console.
	if conf.SerialToSocket != "" {
		qemuArgs = append(qemuArgs, "-serial", fmt.Sprintf("unix:%s,server,wait", conf.SerialToSocket))
	} else if conf.SerialToFile != "" {
		qemuArgs = append(qemuArgs, "-serial", fmt.Sprintf("file:%s", conf.SerialToFile))
	}

	// OVMF firmware.
	qemuArgs = append(qemuArgs,
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", h.BaseOVMFCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", paths.OVMFVars),
	)

	// NIC.
	qemuArgs = append(qemuArgs, "-nic", conf.Nic0)

	// Disk drives.
	if conf.IsInstallation {
		qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=qcow2", paths.DiskImage))
		if paths.Disk1Image != "" {
			qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=qcow2", paths.Disk1Image))
		}

		// Installer media.
		if conf.InstallerISO != "" {
			qemuArgs = append(qemuArgs, "-cdrom", conf.InstallerISO)
		} else if conf.InstallerRaw != "" {
			qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=raw", conf.InstallerRaw))
		}

		qemuArgs = append(qemuArgs,
			"-boot", "once=d",
		)
	} else {
		if conf.DriveIf != "" {
			qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=qcow2,if=%s", paths.DiskImage, conf.DriveIf))
			if paths.Disk1Image != "" {
				qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=qcow2,if=%s", paths.Disk1Image, conf.DriveIf))
			}
		} else {
			qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=qcow2", paths.DiskImage))
			if paths.Disk1Image != "" {
				qemuArgs = append(qemuArgs, "-drive", fmt.Sprintf("file=%s,format=qcow2", paths.Disk1Image))
			}
		}
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

	// Extra args (edge_node only, not installation).
	if !conf.IsInstallation {
		qemuArgs = append(qemuArgs, conf.ExtraArgs...)
	}

	// Write debug script.
	startVMscript := `#!/usr/bin/env bash

set -eu;

#### QEMU ARGS: %v

%s %s
`
	blob := []byte(fmt.Sprintf(startVMscript, qemuArgs, h.QemuPath, strings.Join(qemuArgs, " ")))
	if err := os.WriteFile(paths.DebugScript, blob, 0o755); err != nil {
		tflog.Debug(ctx, "Failed to write start VM script", map[string]any{"error": err})
	}

	// Launch QEMU.
	if conf.IsInstallation {
		// Installation runs synchronously (Run, not RunDetached).
		res, err := cmd.Run(d, h.QemuPath, qemuArgs...)
		if err != nil {
			return fmt.Errorf("failed to run QEMU VM for installing EVE-OS: %w; %s", err, res.Stderr)
		}
	} else {
		res, err := cmd.RunDetached(d, h.QemuPath, qemuArgs...)
		if err != nil {
			return fmt.Errorf("failed to start QEMU VM: %w; %s", err, res.Stderr)
		}
	}

	// CPU pinning (non-installation only).
	if !conf.IsInstallation && len(conf.CPUPins) > 0 {
		cpus := int64(4)
		if conf.CPUs > 0 {
			cpus = conf.CPUs
		}
		if err := h.applyCPUPins(ctx, d, conf.CPUPins, int(cpus)); err != nil {
			tflog.Warn(ctx, "CPU pinning failed", map[string]any{"error": err})
		}
	}

	return nil
}

func (h *QEMUHypervisor) applyCPUPins(ctx context.Context, resourceDir string, cpuPins []int64, numCPUs int) error {
	pidBytes, err := os.ReadFile(filepath.Join(resourceDir, "qemu.pid"))
	if err != nil {
		return fmt.Errorf("could not read QEMU PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	qemuPID := 0
	if _, err := fmt.Sscanf(pidStr, "%d", &qemuPID); err != nil {
		return fmt.Errorf("invalid QEMU PID: %w", err)
	}

	return pinCPUThreads(ctx, h, qemuPID, cpuPins, numCPUs, resourceDir)
}

func (h *QEMUHypervisor) Status(ctx context.Context, resourceDir string) (bool, error) {
	mon, err := qmp.NewSocketMonitor("unix", filepath.Join(resourceDir, "qmp.socket"), 1*time.Second)
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
	mon, err := qmp.NewSocketMonitor("unix", filepath.Join(resourceDir, "qmp.socket"), 2*time.Second)
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

	return nil
}
