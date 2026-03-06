// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/gofrs/uuid/v5"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	installedNodesDir = "installed_nodes"
	nic0FmtInstall    = "user,id=usernet0,ipv6=off,model=virtio"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &InstalledNode{}
	_ resource.ResourceWithImportState = &InstalledNode{}
)

func NewInstalledNode() resource.Resource {
	return &InstalledNode{}
}

// InstalledNode defines the resource implementation.
type InstalledNode struct {
	providerConf *ZedAmigoProviderConfig
}

// InstalledNodeModel describes the resource data model.
type InstalledNodeModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	SerialNo         types.String `tfsdk:"serial_no"`
	InstallerISO     types.String `tfsdk:"installer_iso"`
	InstallerRaw     types.String `tfsdk:"installer_raw"`
	DiskImgBase      types.String `tfsdk:"disk_image_base"`
	Disk1ImgBase     types.String `tfsdk:"disk_1_image_base"`
	SwTPMSock        types.String `tfsdk:"swtpm_socket"`
	DiskImg          types.String `tfsdk:"disk_image"`
	Disk1Img         types.String `tfsdk:"disk_1_image"`
	SerialConsoleLog types.String `tfsdk:"serial_console_log"`
	OvmfVars         types.String `tfsdk:"ovmf_vars"`
	Success          types.Bool   `tfsdk:"success"`
}

func (r *InstalledNode) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, installedNodesDir, id)
}

func (r *InstalledNode) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_installed_edge_node"
}

func (r *InstalledNode) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Installed Edge Node",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Installed Edge Node",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Installed Edge Node identifier",
				MarkdownDescription: "Installed Edge Node identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "Edge Node name",
				MarkdownDescription: "Edge Node name",
				Optional:            true,
				Required:            false,
			},
			"serial_no": schema.StringAttribute{
				Description:         "Installed Edge Node serial number",
				MarkdownDescription: "Installed Edge Node serial number",
				Optional:            false,
				Required:            true,
			},
			"installer_iso": schema.StringAttribute{
				Description:         "Installed Edge Node EVE-OS Installer ISO file",
				MarkdownDescription: "Installed Edge Node EVE-OS Installer ISO file",
				Optional:            true,
				Required:            false,
			},
			"installer_raw": schema.StringAttribute{
				Description:         "Installed Edge Node EVE-OS Installer RAW file (mutually exclusive with installer_iso)",
				MarkdownDescription: "Installed Edge Node EVE-OS Installer RAW file (mutually exclusive with installer_iso)",
				Optional:            true,
				Required:            false,
			},
			"disk_image_base": schema.StringAttribute{
				Description:         "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				MarkdownDescription: "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				Optional:            false,
				Required:            true,
			},
			"disk_1_image_base": schema.StringAttribute{
				Description:         "Disk image base from which the 2nd disk actual disk image used for this node will be created (qemu-img backing file)",
				MarkdownDescription: "Disk image base from which the 2nd disk actual disk image used for this node will be created (qemu-img backing file)",
				Optional:            true,
				Required:            false,
			},
			"swtpm_socket": schema.StringAttribute{
				Description:         "swtpm process unix socket",
				MarkdownDescription: "swtpm process unix socket",
				Optional:            true,
				Required:            false,
			},
			"disk_image": schema.StringAttribute{
				Description:         "Installed Edge Node disk image",
				MarkdownDescription: "Installed Edge Node disk image",
				Computed:            true,
			},
			"disk_1_image": schema.StringAttribute{
				Description:         "Installed Edge Node 2nd disk disk image",
				MarkdownDescription: "Installed Edge Node 2nd disk disk image",
				Computed:            true,
			},
			"serial_console_log": schema.StringAttribute{
				Description:         "Edge Node log file of serial console output",
				MarkdownDescription: "Edge Node log file of serial console output",
				Computed:            true,
			},
			"ovmf_vars": schema.StringAttribute{
				Description:         "UEFI OVMF vars file specific for this installed edge node",
				MarkdownDescription: "UEFI OVMF vars file specific for this installed edge node",
				Computed:            true,
			},
			"success": schema.BoolAttribute{
				Description:         "Whether the EVE-OS install finished succesfully",
				MarkdownDescription: "Whether the EVE-OS install finished succesfully",
				Computed:            true,
			},
		},
	}
}

func (r *InstalledNode) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	conf, ok := req.ProviderData.(*ZedAmigoProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected string, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.providerConf = conf
}

func (r *InstalledNode) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data InstalledNodeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate installer_iso and installer_raw mutual exclusion
	isISO := !data.InstallerISO.IsNull() && len(data.InstallerISO.ValueString()) > 0
	isRaw := !data.InstallerRaw.IsNull() && len(data.InstallerRaw.ValueString()) > 0

	if !isISO && !isRaw {
		resp.Diagnostics.AddAttributeError(
			path.Root("installer_iso"),
			"Missing Required Attribute",
			"Either 'installer_iso' or 'installer_raw' must be set. Exactly one is required.",
		)
		return
	}

	if isISO && isRaw {
		resp.Diagnostics.AddAttributeError(
			path.Root("installer_iso"),
			"Invalid Attribute Combination",
			"Cannot set both 'installer_iso' and 'installer_raw' at the same time. Please set only one.",
		)
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	vmConf := hypervisor.VMConfig{
		Name:           data.Name.ValueString(),
		ID:             data.ID.ValueString(),
		SerialNo:       data.SerialNo.ValueString(),
		ResourceDir:    d,
		DiskImageBase:  data.DiskImgBase.ValueString(),
		Disk1ImageBase: data.Disk1ImgBase.ValueString(),
		Nic0:           nic0FmtInstall,
		SwTPMSocket:    data.SwTPMSock.ValueString(),
		SerialToFile:   filepath.Join(d, "serial_console_install.log"),
		IsInstallation: true,
	}

	if isISO {
		vmConf.InstallerISO = data.InstallerISO.ValueString()
	} else {
		vmConf.InstallerRaw = data.InstallerRaw.ValueString()
	}

	// Prepare disks.
	paths, err := r.providerConf.Hypervisor.PrepareDisks(ctx, vmConf)
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			fmt.Sprintf("Failed to prepare disks: %v", err))
		return
	}

	data.OvmfVars = types.StringValue(paths.OVMFVars)
	data.SerialConsoleLog = types.StringValue(vmConf.SerialToFile)

	// Start VM (runs synchronously for installation).
	if err := r.providerConf.Hypervisor.Start(ctx, vmConf, paths); err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			fmt.Sprintf("Failed to run installation VM: %v", err))
		return
	}

	tflog.Trace(ctx, "Installed Edge Node Resource created succesfully")

	data.DiskImg = types.StringValue(paths.DiskImage)
	data.Disk1Img = types.StringValue(paths.Disk1Image)

	success, err := readInstalledNode(r.providerConf, d)
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Read Error",
			fmt.Sprintf("Can't read EVE-OS install log: %v", err))
	}
	data.Success = types.BoolValue(success)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readInstalledNode(_ *ZedAmigoProviderConfig, path string) (bool, error) {
	x, err := os.ReadFile(filepath.Join(path, "serial_console_install.log"))
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}

	return bytes.Contains(x, []byte("EVE-OS installation completed")), nil
}

func (r *InstalledNode) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data InstalledNodeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	x, err := readInstalledNode(r.providerConf, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Read Error",
			fmt.Sprintf("Can't read EVE-OS install log: %v", err))
		return
	}
	data.Success = types.BoolValue(x)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InstalledNode) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data InstalledNodeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError("Installed Edge Node Resource Update Error", "Update is not supported.")
}

func (r *InstalledNode) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data InstalledNodeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Delete Error",
			fmt.Sprintf("Can't delete resource directory: %v", err))
		return
	}
}

func (r *InstalledNode) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
