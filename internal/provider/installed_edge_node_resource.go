// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
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
	ovmfCode          = "/nix/store/7a68vhgrwhp3i6lppv570111jmsb2mn2-OVMF-202402-fd/FV/OVMF_CODE.fd"
	ovmfVars          = "/nix/store/7a68vhgrwhp3i6lppv570111jmsb2mn2-OVMF-202402-fd/FV/OVMF_VARS.fd"
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
	DiskImgBase      types.String `tfsdk:"disk_image_base"`
	SwTPMSock        types.String `tfsdk:"swtpm_socket"`
	DiskImg          types.String `tfsdk:"disk_image"`
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
				Optional:            false,
				Required:            true,
			},
			"disk_image_base": schema.StringAttribute{
				Description:         "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				MarkdownDescription: "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				Optional:            false,
				Required:            true,
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
	i := filepath.Join(d, "disk0.disk_img.qcow2")
	res, err := cmd.Run(d, r.providerConf.QemuImg, "create", "-f", "qcow2",
		"-b", data.DiskImgBase.ValueString(), "-F", "qcow2", i)
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			"Unable to create a new disk image")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	varsFile := filepath.Join(d, "UEFI_OVMF_VARS.bin")
	if _, err := cmd.CopyFile(ovmfVars, varsFile); err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			fmt.Sprintf("Unable to copy UEFI OVMF vars: %s", err))
	}
	data.OvmfVars = types.StringValue(varsFile)

	data.SerialConsoleLog = types.StringValue(filepath.Join(d, "serial_console_install.log"))

	qemuArgs := []string{}
	if !data.Name.IsNull() {
		qemuArgs = append(qemuArgs, []string{"--name", fmt.Sprintf("edge_node_install_%s", data.Name.ValueString())}...)
	} else {
		qemuArgs = append(qemuArgs, []string{"--name", fmt.Sprintf("edge_node_install_%s", data.ID.ValueString())}...)
	}

	qemuArgs = append(qemuArgs, []string{
		"--enable-kvm", "-machine", "q35,accel=kvm,kernel-irqchip=split",
		"-nographic",
		"-m", "4096",
		"-cpu", "host", "-smp", "4,cores=2",
		"-device", "intel-iommu,intremap=on",
		"-smbios", fmt.Sprintf("type=1,serial=%s", data.SerialNo.ValueString()),
		"-net", "user", "-net", "nic,model=virtio",
		"-serial", fmt.Sprintf("file:%s", data.SerialConsoleLog.ValueString()),
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", ovmfCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", varsFile),
		"-drive", fmt.Sprintf("file=%s,format=qcow2", i),
		"-cdrom", data.InstallerISO.ValueString(),
		"-boot", "once=d",
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", filepath.Join(d, "qmp.socket")),
		"-pidfile", filepath.Join(d, "qemu.pid"),
	}...)

	swtpmSock := data.SwTPMSock.ValueString()
	if swtpmSock != "" {
		qemuArgs = append(qemuArgs, []string{
			// Define the character device connected to the swtpm socket.
			"-chardev", fmt.Sprintf("socket,id=chrtpm,path=%s", swtpmSock),
			// Define the TPM backend device using the character device.
			"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
			// Add the virtual TPM device to the VM (use tpm-crb for TPM 2.0).
			"-device", "tpm-crb,id=mytpm,tpmdev=tpm0",
			/*
				-chardev socket,id=chrtpm,path=/tmp/mytpm1/swtpm-sock \
				-tpmdev emulator,id=tpm0,chardev=chrtpm \
				-device tpm-tis,tpmdev=tpm0 test.img
			*/
		}...)
	}

	res, err = cmd.Run(d, r.providerConf.Qemu, qemuArgs...)
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Error",
			"Failed to run QEMU VM for installing EVE-OS")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	tflog.Trace(ctx, "Installed Edge Node Resource created succesfully")

	j, err := readDiskImage(r.providerConf, d, "disk0")
	if err != nil {
		resp.Diagnostics.AddError("Installed Edge Node Resource Read Error",
			fmt.Sprintf("Can't read back installed edge node disk: %v", err))
		return
	}
	data.DiskImg = types.StringValue(j.Filename)

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
