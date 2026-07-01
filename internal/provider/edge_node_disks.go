// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/exec"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DiskBlockModel backs a single `disk` block on the edge node / installed edge
// node resources. The first `disk` block is disk0, the second is disk1.
type DiskBlockModel struct {
	Type    types.String `tfsdk:"type"`
	Source  types.String `tfsdk:"source"`
	Format  types.String `tfsdk:"format"`
	SizeMB  types.Int64  `tfsdk:"size_mb"`
	DriveIf types.String `tfsdk:"drive_if"`
	Options types.List   `tfsdk:"options"`
}

// legacyDiskAttrs holds the flat, pre-`disk`-block disk attributes. For
// resources that don't expose disk_size_mb / drive_if (e.g.
// installed_edge_node) pass null values.
type legacyDiskAttrs struct {
	DiskImageBase  types.String
	Disk1ImageBase types.String
	DiskSizeMB     types.Int64
	DriveIf        types.String
}

// diskSchemaBlock returns the shared `disk` ListNestedBlock so both resources
// stay in sync. The block is capped at 2 entries (disk0, disk1) and changes
// force replacement of the VM.
func diskSchemaBlock() schema.ListNestedBlock {
	return schema.ListNestedBlock{
		Description: "Disk attached to the VM. Repeat the block to add a second disk: the first `disk` " +
			"block is disk0, the second is disk1 (at most two). Mutually exclusive with the legacy " +
			"disk_image_base / disk_1_image_base attributes.",
		MarkdownDescription: undent.Md(`
		Disk attached to the VM. Repeat the block to add a second disk: the first |disk| block is
		disk0, the second is disk1 (at most two). Mutually exclusive with the legacy
		|disk_image_base| / |disk_1_image_base| attributes.

		The |type| selects how the disk is backed:

		- |overlay| (default): a qcow2 overlay image is created backed by |source| (the historic
		  behavior of |disk_image_base|). |size_mb| resizes the created image.
		- |device|: |source| is a block device or partition (e.g. |/dev/sdb|, |/dev/sdb1|) used
		  as-is. The provider never creates, copies, resizes, or deletes it.
		- |file|: |source| is an existing disk image file used directly, without an overlay.

		On macOS (vfkit), |drive_if| and |options| are ignored and a direct |qcow2| disk
		(|type = "device"| or |"file"| with |format = "qcow2"|) is not supported.`),
		Validators: []validator.List{
			listvalidator.SizeAtMost(2),
		},
		PlanModifiers: []planmodifier.List{
			listplanmodifier.RequiresReplace(),
		},
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"type": schema.StringAttribute{
					Description: `How the disk is backed: "overlay" (default; create a qcow2 overlay from source), ` +
						`"device" (use a block device / partition as-is), or "file" (use an existing image file as-is).`,
					Optional: true,
					Validators: []validator.String{
						stringvalidator.OneOf(string(hypervisor.DiskOverlay), string(hypervisor.DiskDevice), string(hypervisor.DiskFile)),
					},
				},
				"source": schema.StringAttribute{
					Description: "For type=overlay, the qemu-img backing image. For type=device, the block device or " +
						"partition path (e.g. /dev/sdb). For type=file, the path of an existing disk image file.",
					Required: true,
				},
				"format": schema.StringAttribute{
					Description: "QEMU `-drive format=` value. Defaults to qcow2 for type=overlay and raw for " +
						"type=device/file. Only qcow2 is allowed for type=overlay.",
					Optional: true,
					Validators: []validator.String{
						stringvalidator.OneOf("raw", "qcow2"),
					},
				},
				"size_mb": schema.Int64Attribute{
					Description: "Disk image size in MB. Only valid for type=overlay (resizes the created image). " +
						"If not specified, the size of the base image is preserved.",
					Optional: true,
				},
				"drive_if": schema.StringAttribute{
					Description: "The value of the interface (if) option for this disk's QEMU `-drive` flag (e.g. " +
						"virtio, ide, scsi). QEMU-only; ignored on macOS (vfkit).",
					Optional: true,
				},
				"options": schema.ListAttribute{
					Description: "Extra `-drive` options for this disk, appended verbatim (comma-joined) to the " +
						"`-drive` argument, e.g. [\"cache=none\", \"aio=native\", \"discard=unmap\"]. QEMU-only.",
					ElementType: types.StringType,
					Optional:    true,
				},
			},
		},
	}
}

// diskImagePath returns the resolved path for disk slot i, or "" if that slot
// is not configured.
func diskImagePath(paths []string, i int) string {
	if i < len(paths) {
		return paths[i]
	}
	return ""
}

// buildDisks translates either the `disk` blocks or the legacy flat attributes
// into the ordered []hypervisor.DiskConfig consumed by the hypervisor layer,
// applying defaults and validation. Index 0 is disk0, index 1 is disk1.
func buildDisks(ctx context.Context, ex exec.Executor, blocks []DiskBlockModel, legacy legacyDiskAttrs) ([]hypervisor.DiskConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	legacyDisk0 := !legacy.DiskImageBase.IsNull() && legacy.DiskImageBase.ValueString() != ""
	legacyDisk1 := !legacy.Disk1ImageBase.IsNull() && legacy.Disk1ImageBase.ValueString() != ""
	legacyExtras := !legacy.DiskSizeMB.IsNull() || (!legacy.DriveIf.IsNull() && legacy.DriveIf.ValueString() != "")
	legacyUsed := legacyDisk0 || legacyDisk1 || legacyExtras
	blocksUsed := len(blocks) > 0

	if blocksUsed && legacyUsed {
		diags.AddError("Conflicting disk configuration",
			"Use either the `disk` blocks or the legacy `disk_image_base` / `disk_1_image_base` "+
				"(and `disk_size_mb` / `drive_if`) attributes to configure disks, not both.")
		return nil, diags
	}

	if blocksUsed {
		disks := make([]hypervisor.DiskConfig, 0, len(blocks))
		for i, b := range blocks {
			dc, dcDiags := diskConfigFromBlock(ctx, ex, i, b)
			diags.Append(dcDiags...)
			disks = append(disks, dc)
		}
		return disks, diags
	}

	// Legacy path: disk0 (required) + optional disk1, both qcow2 overlays. The
	// single disk_size_mb / drive_if apply to both, as before.
	if !legacyDisk0 {
		if legacyDisk1 {
			diags.AddError("Invalid disk configuration",
				"`disk_1_image_base` is set without `disk_image_base`; the first disk (disk0) is required.")
		} else {
			diags.AddError("Missing disk configuration",
				"A disk is required: configure a `disk` block or set `disk_image_base`.")
		}
		return nil, diags
	}

	hasSize := !legacy.DiskSizeMB.IsNull()
	sizeMB := legacy.DiskSizeMB.ValueInt64()
	driveIf := legacy.DriveIf.ValueString()

	disks := []hypervisor.DiskConfig{{
		Type:    hypervisor.DiskOverlay,
		Source:  legacy.DiskImageBase.ValueString(),
		Format:  "qcow2",
		SizeMB:  sizeMB,
		HasSize: hasSize,
		DriveIf: driveIf,
	}}
	if legacyDisk1 {
		disks = append(disks, hypervisor.DiskConfig{
			Type:    hypervisor.DiskOverlay,
			Source:  legacy.Disk1ImageBase.ValueString(),
			Format:  "qcow2",
			SizeMB:  sizeMB,
			HasSize: hasSize,
			DriveIf: driveIf,
		})
	}
	return disks, diags
}

func diskConfigFromBlock(ctx context.Context, ex exec.Executor, idx int, b DiskBlockModel) (hypervisor.DiskConfig, diag.Diagnostics) {
	var diags diag.Diagnostics

	dt := hypervisor.DiskOverlay
	if !b.Type.IsNull() && b.Type.ValueString() != "" {
		dt = hypervisor.DiskType(b.Type.ValueString())
	}

	source := b.Source.ValueString()
	if source == "" {
		diags.AddError("Invalid disk configuration",
			fmt.Sprintf("disk %d: `source` must not be empty.", idx))
		return hypervisor.DiskConfig{}, diags
	}

	format := ""
	if !b.Format.IsNull() {
		format = b.Format.ValueString()
	}
	hasSize := !b.SizeMB.IsNull()

	switch dt {
	case hypervisor.DiskOverlay:
		if format == "" {
			format = "qcow2"
		} else if format != "qcow2" {
			diags.AddError("Invalid disk configuration",
				fmt.Sprintf("disk %d: `type = \"overlay\"` only supports `format = \"qcow2\"`.", idx))
		}
	case hypervisor.DiskDevice, hypervisor.DiskFile:
		if format == "" {
			format = "raw"
		}
		if hasSize {
			diags.AddError("Invalid disk configuration",
				fmt.Sprintf("disk %d: `size_mb` is only valid for `type = \"overlay\"` (cannot resize a %s disk).", idx, dt))
		}
		// The device / file must already exist on the target; fail fast with a
		// clear error.
		if _, err := ex.Stat(ctx, source); err != nil {
			diags.AddError("Invalid disk configuration",
				fmt.Sprintf("disk %d: cannot access source %q: %v", idx, source, err))
		}
	default:
		diags.AddError("Invalid disk configuration",
			fmt.Sprintf("disk %d: unknown type %q (expected overlay, device, or file).", idx, dt))
		return hypervisor.DiskConfig{}, diags
	}

	var options []string
	if !b.Options.IsNull() && !b.Options.IsUnknown() {
		diags.Append(b.Options.ElementsAs(ctx, &options, false)...)
	}

	return hypervisor.DiskConfig{
		Type:    dt,
		Source:  source,
		Format:  format,
		SizeMB:  b.SizeMB.ValueInt64(),
		HasSize: hasSize,
		DriveIf: b.DriveIf.ValueString(),
		Options: options,
	}, diags
}
