// SPDX-License-Identifier: MPL-2.0

package hypervisor

import "context"

// DiskType selects how a disk is backed.
type DiskType string

const (
	// DiskOverlay creates a qcow2 overlay image backed by Source (the historic
	// behavior of disk_image_base / disk_1_image_base).
	DiskOverlay DiskType = "overlay"
	// DiskDevice uses Source (a block device or partition, e.g. /dev/sdb) as-is.
	DiskDevice DiskType = "device"
	// DiskFile uses Source (an existing disk image file) as-is, without creating
	// an overlay.
	DiskFile DiskType = "file"
)

// DiskConfig describes a single disk (disk0, disk1, ...) attached to a VM.
type DiskConfig struct {
	Type DiskType
	// Source is the qemu-img backing image for DiskOverlay, or the device/file
	// path used directly for DiskDevice / DiskFile.
	Source string
	// Format is the resolved QEMU "-drive format=" value ("qcow2" or "raw").
	Format string
	// SizeMB resizes the created image (DiskOverlay only). Ignored when HasSize is false.
	SizeMB  int64
	HasSize bool
	// DriveIf is the per-disk QEMU "-drive if=" value (e.g. "virtio"). QEMU-only.
	DriveIf string
	// Options are extra "-drive" key=value options appended verbatim. QEMU-only.
	Options []string
}

// VMConfig contains all configuration needed to prepare and start a VM.
type VMConfig struct {
	Name        string
	ID          string
	SerialNo    string
	ResourceDir string
	MemoryMB    string // e.g. "4G", "4096", "4096M"
	CPUs        int64

	// Disks holds the VM disks in slot order: index 0 = disk0, index 1 = disk1.
	Disks []DiskConfig

	OVMFCode    string
	OVMFVarsSrc string

	Nic0 string

	SSHPort int32

	SwTPMSocket string

	SerialToFile   string // file path for serial output
	SerialToSocket string // socket path for serial server
	SerialType     string // "virtio" (default) or "serial" (emulated ISA)

	ExtraArgs []string
	CPUPins   []int64

	// Use embedded gvproxy instead of QEMU SLIRP for networking.
	UseGvproxy bool

	// For installed_edge_node:
	InstallerISO   string
	InstallerRaw   string
	IsInstallation bool
}

// VMPaths holds the paths to files created during VM preparation and start.
type VMPaths struct {
	// DiskImages holds the resolved path used in the QEMU "-drive file=" (or the
	// vfkit VirtioBlk path) for each disk, aligned by index with VMConfig.Disks:
	// the created overlay image for DiskOverlay, or the device/file source for
	// DiskDevice / DiskFile.
	DiskImages       []string
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

	// ApplyCPUPins pins vCPU threads to host CPUs. Must be called after the VM
	// process is fully started (i.e., after serial socket clients have connected).
	ApplyCPUPins(ctx context.Context, conf VMConfig) error
}
