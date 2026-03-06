// SPDX-License-Identifier: MPL-2.0

package hypervisor

import "context"

// VMConfig contains all configuration needed to prepare and start a VM.
type VMConfig struct {
	Name       string
	ID         string
	SerialNo   string
	ResourceDir string
	MemoryMB   string // e.g. "4G", "4096", "4096M"
	CPUs       int64
	DiskImageBase  string
	Disk1ImageBase string
	DiskSizeMB     int64
	HasDiskSize    bool
	DriveIf    string // QEMU-specific, ignored by vfkit

	OVMFCode    string
	OVMFVarsSrc string

	Nic0 string

	SSHPort int32

	SwTPMSocket string

	SerialToFile   string // file path for serial output
	SerialToSocket string // socket path for serial server

	ExtraArgs []string
	CPUPins   []int64

	// For installed_edge_node:
	InstallerISO string
	InstallerRaw string
	IsInstallation bool
}

// VMPaths holds the paths to files created during VM preparation and start.
type VMPaths struct {
	DiskImage        string
	Disk1Image       string
	OVMFVars         string
	QMPSocket        string // QEMU-only, empty on vfkit
	PIDFile          string
	SerialConsoleLog string
	SerialPortSocket string
	DebugScript      string
}

// Hypervisor abstracts VM lifecycle operations across different hypervisor backends.
type Hypervisor interface {
	// PrepareDisks creates disk images and UEFI variable files for the VM.
	PrepareDisks(ctx context.Context, conf VMConfig) (VMPaths, error)

	// Start launches the VM process.
	Start(ctx context.Context, conf VMConfig, paths VMPaths) error

	// Status checks whether the VM is currently running.
	Status(ctx context.Context, resourceDir string) (running bool, err error)

	// Stop shuts down a running VM.
	Stop(ctx context.Context, resourceDir string) error
}
